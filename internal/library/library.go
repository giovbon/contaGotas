// Package library importa arquivos para a biblioteca: valida o formato,
// calcula o sha256, evita duplicatas, grava o original em
// `{root}/documents/{id}/source.*` e enfileira a extração.
package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/giovani/readingdrip/internal/docs"
	"github.com/giovani/readingdrip/internal/store"
)

// DefaultMaxBytes é o limite de upload de 95 MiB (margem do limite de 100 MiB
// do plano free da Cloudflare, ver docs/operations.md).
const DefaultMaxBytes int64 = 95 * 1024 * 1024

var (
	// ErrUnsupportedFormat indica extensão fora de md/epub/pdf.
	ErrUnsupportedFormat = errors.New("library: formato não suportado")
	// ErrEmptyFile indica arquivo sem conteúdo.
	ErrEmptyFile = errors.New("library: arquivo vazio")
	// ErrDuplicate indica conteúdo já importado (mesmo sha256).
	ErrDuplicate = errors.New("library: documento duplicado")
	// ErrTooLarge indica arquivo acima do limite configurado.
	ErrTooLarge = errors.New("library: arquivo acima do limite")
)

// Enqueuer recebe o id do documento recém-criado. Implementado por
// ingest.Queue; a interface evita que a importação dependa do worker.
type Enqueuer interface {
	Enqueue(ctx context.Context, documentID int64) error
}

// Entrada é um arquivo a importar, já aberto pelo chamador (multipart ou
// teste). Name é o nome original; Reader é lido até o limite.
type Entrada struct {
	Name   string
	Reader io.Reader
}

// Importer grava originais e cria documentos em `processing`.
type Importer struct {
	Store    *store.Store
	Root     string   // diretório de dados, ex.: /data
	MaxBytes int64    // 0 → DefaultMaxBytes
	Queue    Enqueuer // opcional; sem fila o documento fica em processing
	Now      func() time.Time
}

// DuplicateError carrega o documento existente para o aviso da UI (RF-007).
type DuplicateError struct {
	Existing *store.Document
}

func (e *DuplicateError) Error() string {
	if e.Existing == nil {
		return ErrDuplicate.Error()
	}
	return fmt.Sprintf("%s (id %d: %q)", ErrDuplicate, e.Existing.ID, e.Existing.Title)
}

func (e *DuplicateError) Unwrap() error { return ErrDuplicate }

// TooLargeError informa o limite e quantos bytes foram lidos.
type TooLargeError struct {
	Limit int64
	Size  int64
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("%s: %d bytes (limite %d)", ErrTooLarge, e.Size, e.Limit)
}

func (e *TooLargeError) Unwrap() error { return ErrTooLarge }

// Import valida, grava e enfileira um arquivo. Em qualquer falha depois da
// criação do documento, o registro e os arquivos são removidos: ou a
// importação inteira conclui, ou nada fica para trás.
func (i *Importer) Import(ctx context.Context, e Entrada) (*store.Document, error) {
	if i.Store == nil {
		return nil, errors.New("library: importador sem store")
	}
	if i.Root == "" {
		return nil, errors.New("library: importador sem diretório de dados")
	}
	if e.Reader == nil {
		return nil, errors.New("library: entrada sem conteúdo")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	nome := sanitizarNome(e.Name)
	format, ok := docs.Detect(nome)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, e.Name)
	}

	limite := i.MaxBytes
	if limite <= 0 {
		limite = DefaultMaxBytes
	}
	docsDir := filepath.Join(i.Root, "documents")
	if err := os.MkdirAll(docsDir, 0o750); err != nil {
		return nil, fmt.Errorf("library: criar diretório de documentos: %w", err)
	}

	tmp, err := os.CreateTemp(docsDir, ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("library: criar temporário de upload: %w", err)
	}
	tmpPath := tmp.Name()
	movido := false
	defer func() {
		if !movido {
			_ = os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	lidos, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(e.Reader, limite+1))
	if err != nil {
		tmp.Close()
		return nil, fmt.Errorf("library: gravar upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("library: fechar upload: %w", err)
	}
	if lidos == 0 {
		return nil, ErrEmptyFile
	}
	if lidos > limite {
		return nil, &TooLargeError{Limit: limite, Size: lidos}
	}
	sum := hex.EncodeToString(hasher.Sum(nil))

	existente, err := i.Store.Documents().BySHA256(ctx, sum)
	if err != nil {
		return nil, err
	}
	if existente != nil {
		return nil, &DuplicateError{Existing: existente}
	}

	agora := i.agora()
	titulo := strings.TrimSuffix(nome, filepath.Ext(nome))
	if titulo == "" {
		titulo = "Documento"
	}
	id, err := i.Store.Documents().Create(ctx, store.Document{
		Title:        titulo,
		Format:       string(format),
		SourceName:   nome,
		SourcePath:   tmpPath,
		SourceSHA256: sum,
		Status:       store.StatusProcessing,
		CreatedAt:    agora,
		UpdatedAt:    agora,
	})
	if err != nil {
		// Corrida entre dois uploads do mesmo conteúdo: o UNIQUE decide e o
		// perdedor devolve o documento do vencedor.
		if existente, buscaErr := i.Store.Documents().BySHA256(ctx, sum); buscaErr == nil && existente != nil {
			return nil, &DuplicateError{Existing: existente}
		}
		return nil, err
	}

	finalDir := filepath.Join(docsDir, strconv.FormatInt(id, 10))
	if err := os.MkdirAll(finalDir, 0o750); err != nil {
		return nil, i.descartar(ctx, id, finalDir,
			fmt.Errorf("library: criar diretório do documento %d: %w", id, err))
	}
	finalPath := filepath.Join(finalDir, "source"+strings.ToLower(filepath.Ext(nome)))
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, i.descartar(ctx, id, finalDir,
			fmt.Errorf("library: mover original do documento %d: %w", id, err))
	}
	movido = true

	if err := i.Store.Documents().UpdateSource(ctx, id, nome, finalPath, agora); err != nil {
		return nil, i.descartar(ctx, id, finalDir, err)
	}

	doc, err := i.Store.Documents().Get(ctx, id)
	if err != nil {
		return nil, i.descartar(ctx, id, finalDir, err)
	}
	if doc == nil {
		return nil, i.descartar(ctx, id, finalDir,
			fmt.Errorf("library: documento %d sumiu após a importação", id))
	}

	if i.Queue != nil {
		if err := i.Queue.Enqueue(ctx, id); err != nil {
			return nil, i.descartar(ctx, id, finalDir,
				fmt.Errorf("library: enfileirar documento %d: %w", id, err))
		}
	}
	return doc, nil
}

// descartar desfaz uma importação parcial: remove o documento (e derivados) e
// o diretório do original. Erros são agregados à causa original.
func (i *Importer) descartar(ctx context.Context, id int64, dir string, causa error) error {
	erroDoc := i.Store.Documents().Delete(ctx, id)
	erroArq := os.RemoveAll(dir)
	return errors.Join(causa, erroDoc, erroArq)
}

// RemoveOriginal apaga o diretório `{root}/documents/{id}`, que guarda o
// arquivo original e a capa. Diretório ausente não é erro; o chamador já
// removeu o registro do documento. A camada web decide quando chamar.
func (i *Importer) RemoveOriginal(id int64) error {
	if i.Root == "" {
		return errors.New("library: importador sem diretório de dados")
	}
	if id <= 0 {
		return fmt.Errorf("library: id de documento inválido para remoção: %d", id)
	}
	dir := filepath.Join(i.Root, "documents", strconv.FormatInt(id, 10))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("library: remover diretório do documento %d: %w", id, err)
	}
	return nil
}

// CleanupTemp remove temporários de uploads interrompidos por crash. Deve ser
// chamado no start, antes de aceitar novos uploads.
func (i *Importer) CleanupTemp() error {
	if i.Root == "" {
		return errors.New("library: importador sem diretório de dados")
	}
	temps, err := filepath.Glob(filepath.Join(i.Root, "documents", ".upload-*"))
	if err != nil {
		return fmt.Errorf("library: listar temporários: %w", err)
	}
	var erros []error
	for _, path := range temps {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			erros = append(erros, fmt.Errorf("library: remover temporário %s: %w", filepath.Base(path), err))
		}
	}
	return errors.Join(erros...)
}

func (i *Importer) agora() time.Time {
	if i.Now != nil {
		return i.Now()
	}
	return time.Now()
}

// sanitizarNome usa apenas o nome base, aceitando separadores de Windows.
// Nomes vazios ou com caminho embutido não escapam do diretório do documento.
func sanitizarNome(nome string) string {
	nome = strings.ReplaceAll(nome, "\\", "/")
	nome = filepath.Base(strings.TrimSpace(nome))
	if nome == "." || nome == "/" {
		return ""
	}
	return nome
}
