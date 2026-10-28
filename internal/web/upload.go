package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/giovani/readingdrip/internal/library"
	"github.com/giovani/readingdrip/internal/store"
)

// handleUpload recebe um ou vários arquivos em multipart, importa cada um e
// redireciona para o destino adequado. O parse nunca acontece na request: o
// importador grava o original e enfileira a extração.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if s.Importer == nil {
		http.Error(w, "importação indisponível", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.limiteCorpo())
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "envie multipart/form-data", http.StatusBadRequest)
		return
	}

	var (
		importados []*store.Document
		duplicados []*library.DuplicateError
	)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.recusar(w, r, partErro(err), err)
			return
		}
		if part.FileName() == "" {
			part.Close()
			continue
		}

		doc, err := s.Importer.Import(r.Context(), library.Entrada{
			Name:   part.FileName(),
			Reader: part,
		})
		part.Close()

		if err != nil {
			var dup *library.DuplicateError
			if errors.As(err, &dup) {
				duplicados = append(duplicados, dup)
				continue
			}
			status, msg := statusDoErro(err)
			s.recusar(w, r, status, fmt.Errorf("%s: %w", msg, err))
			return
		}
		importados = append(importados, doc)
	}

	if len(importados)+len(duplicados) == 0 {
		http.Error(w, "nenhum arquivo enviado", http.StatusUnprocessableEntity)
		return
	}

	s.logger().Info("upload concluído",
		"importados", len(importados), "duplicados", len(duplicados))
	s.redirecionar(w, r, importados, duplicados)
}

func (s *Server) redirecionar(w http.ResponseWriter, r *http.Request, importados []*store.Document, duplicados []*library.DuplicateError) {
	if len(importados) == 1 && len(duplicados) == 0 {
		http.Redirect(w, r, fmt.Sprintf("/documents/%d", importados[0].ID), http.StatusSeeOther)
		return
	}
	if len(importados) == 0 && len(duplicados) == 1 && duplicados[0].Existing != nil {
		http.Redirect(w, r,
			fmt.Sprintf("/documents/%d?duplicado=1", duplicados[0].Existing.ID),
			http.StatusSeeOther)
		return
	}

	q := url.Values{}
	q.Set("importados", strconv.Itoa(len(importados)))
	q.Set("duplicados", strconv.Itoa(len(duplicados)))
	http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
}

// recusar registra a causa (sem conteúdo de arquivo) e devolve a mensagem
// pública.
func (s *Server) recusar(w http.ResponseWriter, r *http.Request, status int, causa error) {
	if r.Context().Err() != nil {
		return
	}
	s.logger().Warn("upload recusado", "status", status, "erro", causa)
	http.Error(w, mensagemDoStatus(status), status)
}

func mensagemDoStatus(status int) string {
	switch status {
	case http.StatusRequestEntityTooLarge:
		return "arquivo acima do limite"
	case http.StatusUnprocessableEntity:
		return "arquivo inválido: use .md, .epub ou .pdf não vazio"
	case http.StatusBadRequest:
		return "requisição multipart inválida"
	default:
		return "não foi possível importar o arquivo"
	}
}

// statusDoErro traduz as sentinelas da importação para HTTP (docs/web-ui.md).
func statusDoErro(err error) (int, string) {
	var grande *library.TooLargeError
	var corpo *http.MaxBytesError
	switch {
	case errors.As(err, &grande), errors.As(err, &corpo):
		return http.StatusRequestEntityTooLarge, "arquivo acima do limite"
	case errors.Is(err, library.ErrUnsupportedFormat), errors.Is(err, library.ErrEmptyFile):
		return http.StatusUnprocessableEntity, "arquivo inválido"
	default:
		return http.StatusInternalServerError, "falha ao importar"
	}
}

func partErro(err error) int {
	var corpo *http.MaxBytesError
	if errors.As(err, &corpo) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// limiteCorpo dá folga de 1 MiB para o envelope multipart; o limite exato do
// arquivo é aplicado pelo importador.
func (s *Server) limiteCorpo() int64 {
	if s.MaxBodyBytes > 0 {
		return s.MaxBodyBytes
	}
	limite := library.DefaultMaxBytes
	if s.Importer != nil && s.Importer.MaxBytes > 0 {
		limite = s.Importer.MaxBytes
	}
	return limite + (1 << 20)
}
