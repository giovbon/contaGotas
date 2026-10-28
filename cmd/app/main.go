// Command app sobe o servidor HTTP: store SQLite, autenticação de sessão,
// importador de arquivos, fila de extração e as telas de gestão.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/giovani/readingdrip/internal/auth"
	"github.com/giovani/readingdrip/internal/ingest"
	"github.com/giovani/readingdrip/internal/library"
	"github.com/giovani/readingdrip/internal/links"
	"github.com/giovani/readingdrip/internal/platform"
	"github.com/giovani/readingdrip/internal/schedule"
	"github.com/giovani/readingdrip/internal/store"
	"github.com/giovani/readingdrip/internal/web"
)

const (
	leituraCabecalho = 10 * time.Second
	leituraMaxima    = 10 * time.Minute
	escritaMaxima    = 10 * time.Minute
	idleMaximo       = 120 * time.Second
	shutdownMaximo   = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("readingdrip: falha", "erro", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := platform.LoadEnv(os.Getenv)
	if err != nil {
		return err
	}
	logger := platform.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	app, err := montar(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()

	servidor := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: leituraCabecalho,
		ReadTimeout:       leituraMaxima,
		WriteTimeout:      escritaMaxima,
		IdleTimeout:       idleMaximo,
	}

	erroServidor := make(chan error, 1)
	go func() { erroServidor <- servidor.ListenAndServe() }()
	logger.Info("servidor iniciado", "addr", cfg.Addr, "data_dir", cfg.DataDir)

	select {
	case err := <-erroServidor:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("servidor HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("encerrando por sinal")
	}

	desligar, cancelar := context.WithTimeout(context.Background(), shutdownMaximo)
	defer cancelar()
	if err := servidor.Shutdown(desligar); err != nil {
		logger.Error("shutdown do servidor", "erro", err)
		_ = servidor.Close()
	}
	return nil
}

// aplicacao agrega os componentes de longa duração. Close encerra o agendador,
// drena a fila e depois fecha o banco.
type aplicacao struct {
	cfg      platform.Config
	logger   *slog.Logger
	store    *store.Store
	importer *library.Importer
	fila     *ingest.Queue
	agenda   *schedule.Runner
	web      *web.Server
}

func montar(ctx context.Context, cfg platform.Config, logger *slog.Logger) (*aplicacao, error) {
	if logger == nil {
		logger = platform.NewLogger(cfg.LogLevel)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sessoes, err := auth.New(auth.Config{
		Password:         cfg.Password,
		Secret:           cfg.Secret,
		BehindCloudflare: cfg.BehindCloudflare,
	})
	if err != nil {
		return nil, err
	}

	assinador, err := links.New(links.Config{Secret: cfg.Secret, BaseURL: cfg.BaseURL})
	if err != nil {
		return nil, err
	}

	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, err
	}

	imp := &library.Importer{
		Store:    st,
		Root:     cfg.DataDir,
		MaxBytes: cfg.MaxUploadBytes,
	}
	if err := imp.CleanupTemp(); err != nil {
		logger.Warn("limpar temporários de upload", "erro", err)
	}

	fila := ingest.NewQueue(&ingest.Service{Store: st}, 0)
	imp.Queue = fila
	fila.OnResult = func(documentID int64, res *ingest.Result, err error) {
		if err != nil {
			logger.Warn("extração falhou", "documento", documentID, "erro", err)
			return
		}
		logger.Info("documento pronto",
			"documento", documentID, "blocos", res.Blocks, "chunks", res.Chunks)
	}
	if err := fila.Start(context.Background()); err != nil {
		st.Close()
		return nil, err
	}

	docs, err := st.Documents().List(ctx)
	if err != nil {
		fila.Close()
		st.Close()
		return nil, err
	}
	pendentes := documentosPendentes(docs)
	for _, id := range pendentes {
		if err := fila.Enqueue(ctx, id); err != nil {
			logger.Warn("reenfileirar documento", "documento", id, "erro", err)
		}
	}
	if len(pendentes) > 0 {
		logger.Info("documentos em processing reenfileirados", "quantidade", len(pendentes))
	}

	agenda := &schedule.Runner{
		Store:  st,
		Logger: logger,
		Links: func(_ store.Document, chunk store.Chunk, leitorTTL, lidoTTL time.Duration) (string, string) {
			return assinador.ReadURL(chunk.ID, leitorTTL), assinador.DoneURL(chunk.ID, lidoTTL)
		},
	}
	agenda.Start(context.Background())

	return &aplicacao{
		cfg:      cfg,
		logger:   logger,
		store:    st,
		importer: imp,
		fila:     fila,
		agenda:   agenda,
		web: &web.Server{
			Store:    st,
			Importer: imp,
			Queue:    fila,
			Sessions: sessoes,
			Sender:   agenda,
			Links:    assinador,
			Logger:   logger,
		},
	}, nil
}

// Handler expõe o mux HTTP já configurado.
func (a *aplicacao) Handler() http.Handler { return a.web.Handler() }

// Close encerra o agendador, drena a fila antes de fechar o banco, para não
// deixar o worker usando um store fechado.
func (a *aplicacao) Close() {
	if a == nil {
		return
	}
	if a.agenda != nil {
		a.agenda.Stop()
	}
	if a.fila != nil {
		a.fila.Close()
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			a.logger.Error("fechar banco", "erro", err)
		}
	}
}

// documentosPendentes devolve os ids que ficaram em processing (crash ou
// shutdown durante a extração) para reenfileirar no start.
func documentosPendentes(docs []store.Document) []int64 {
	out := make([]int64, 0)
	for _, d := range docs {
		if d.Status == store.StatusProcessing {
			out = append(out, d.ID)
		}
	}
	return out
}
