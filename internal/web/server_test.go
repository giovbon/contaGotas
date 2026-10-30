package web_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/auth"
	"github.com/giovani/readingdrip/internal/ingest"
	"github.com/giovani/readingdrip/internal/library"
	"github.com/giovani/readingdrip/internal/store"
	"github.com/giovani/readingdrip/internal/web"
)

var agora = time.Date(2026, 10, 27, 18, 0, 0, 0, time.FixedZone("BRT", -3*3600))

type filaFalsa struct {
	mu   sync.Mutex
	ids  []int64
	erro error
}

func (f *filaFalsa) Enqueue(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.erro != nil {
		return f.erro
	}
	f.ids = append(f.ids, id)
	return nil
}

func (f *filaFalsa) enfileirados() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.ids...)
}

type ambiente struct {
	srv    *web.Server
	store  *store.Store
	imp    *library.Importer
	fila   *filaFalsa
	worker *ingest.Service
	http   *httptest.Server
	root   string
}

func novoAmbiente(t *testing.T, auth web.Middleware) *ambiente {
	t.Helper()
	return novoAmbienteConfig(t, auth, nil)
}

func novoAmbienteConfig(t *testing.T, auth web.Middleware, sessions *auth.Authenticator) *ambiente {
	t.Helper()
	root := t.TempDir()
	s, err := store.Open(context.Background(), filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})
	fila := &filaFalsa{}
	imp := &library.Importer{
		Store: s,
		Root:  filepath.Join(root, "data"),
		Queue: fila,
		Now:   func() time.Time { return agora },
	}
	srv := &web.Server{Store: s, Importer: imp, Queue: fila, Auth: auth, Sessions: sessions}
	worker := &ingest.Service{Store: s, Now: func() time.Time { return agora }}
	srv.Replanner = worker
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &ambiente{srv: srv, store: s, imp: imp, fila: fila, worker: worker, http: ts, root: filepath.Join(root, "data")}
}

func permitirTudo(h http.Handler) http.Handler { return h }

var semRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}}

type arquivoTeste struct {
	nome     string
	conteudo string
}

func corpoMultipart(t *testing.T, arquivos ...arquivoTeste) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for _, a := range arquivos {
		fw, err := mw.CreateFormFile("files", a.nome)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(fw, a.conteudo); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &b, mw.FormDataContentType()
}

func enviar(t *testing.T, a *ambiente, arquivos ...arquivoTeste) *http.Response {
	t.Helper()
	body, ct := corpoMultipart(t, arquivos...)
	resp, err := semRedirect.Post(a.http.URL+"/documents", ct, body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func documentos(t *testing.T, a *ambiente) []store.Document {
	t.Helper()
	docs, err := a.store.Documents().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

func TestUploadUmArquivo(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)
	conteudo := "# Livro\n\nPrimeiro parágrafo.\n"

	resp := enviar(t, a, arquivoTeste{nome: "Livro.md", conteudo: conteudo})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}

	docs := documentos(t, a)
	if len(docs) != 1 {
		t.Fatalf("documentos = %d, quer 1", len(docs))
	}
	doc := docs[0]
	if resp.Header.Get("Location") != "/documents/"+itoa(doc.ID) {
		t.Errorf("location = %q", resp.Header.Get("Location"))
	}
	if doc.Status != store.StatusProcessing {
		t.Errorf("status = %q, quer processing", doc.Status)
	}

	origem := filepath.Join(a.root, "documents", itoa(doc.ID), "source.md")
	lido, err := os.ReadFile(origem)
	if err != nil {
		t.Fatalf("ler original: %v", err)
	}
	if string(lido) != conteudo {
		t.Errorf("original = %q", lido)
	}
	if ids := a.fila.enfileirados(); len(ids) != 1 || ids[0] != doc.ID {
		t.Errorf("fila = %v, quer [%d]", ids, doc.ID)
	}

	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", resp.Header.Get("X-Content-Type-Options"))
	}
}

func TestUploadVariosArquivos(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)

	resp := enviar(t, a,
		arquivoTeste{nome: "a.md", conteudo: "# A\n\nTexto A.\n"},
		arquivoTeste{nome: "b.md", conteudo: "# B\n\nTexto B.\n"},
		arquivoTeste{nome: "c.md", conteudo: "# C\n\nTexto C.\n"},
	)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}

	destino := localizar(t, resp)
	if destino.Path != "/" {
		t.Errorf("path = %q, quer /", destino.Path)
	}
	if destino.Query().Get("importados") != "3" || destino.Query().Get("duplicados") != "0" {
		t.Errorf("query = %q", destino.RawQuery)
	}
	if docs := documentos(t, a); len(docs) != 3 {
		t.Errorf("documentos = %d, quer 3", len(docs))
	}
	if ids := a.fila.enfileirados(); len(ids) != 3 {
		t.Errorf("fila = %v, quer 3 ids", ids)
	}
}

func TestUploadDuplicado(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)
	conteudo := "# Igual\n\nMesmo conteúdo.\n"

	if resp := enviar(t, a, arquivoTeste{nome: "a.md", conteudo: conteudo}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("primeiro upload = %d", resp.StatusCode)
	}
	docs := documentos(t, a)
	if len(docs) != 1 {
		t.Fatalf("documentos = %d", len(docs))
	}

	resp := enviar(t, a, arquivoTeste{nome: "b.md", conteudo: conteudo})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	destino := localizar(t, resp)
	if destino.Path != "/documents/"+itoa(docs[0].ID) || destino.Query().Get("duplicado") != "1" {
		t.Errorf("location = %q", resp.Header.Get("Location"))
	}
	if docs := documentos(t, a); len(docs) != 1 {
		t.Errorf("documentos = %d, quer 1", len(docs))
	}
	if ids := a.fila.enfileirados(); len(ids) != 1 {
		t.Errorf("fila = %v, quer só o primeiro", ids)
	}
}

func TestUploadLoteComDuplicado(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)
	igual := "# Igual\n\nMesmo conteúdo.\n"

	enviar(t, a, arquivoTeste{nome: "a.md", conteudo: igual})

	resp := enviar(t, a,
		arquivoTeste{nome: "b.md", conteudo: igual},
		arquivoTeste{nome: "novo.md", conteudo: "# Novo\n\nConteúdo novo.\n"},
	)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	destino := localizar(t, resp)
	if destino.Path != "/" || destino.Query().Get("importados") != "1" || destino.Query().Get("duplicados") != "1" {
		t.Errorf("location = %q", resp.Header.Get("Location"))
	}
	if docs := documentos(t, a); len(docs) != 2 {
		t.Errorf("documentos = %d, quer 2", len(docs))
	}
}

func TestUploadArquivoInvalido(t *testing.T) {
	casos := []struct {
		nome    string
		arquivo arquivoTeste
		status  int
	}{
		{"formato", arquivoTeste{nome: "nota.txt", conteudo: "texto"}, http.StatusUnprocessableEntity},
		{"vazio", arquivoTeste{nome: "vazio.md", conteudo: ""}, http.StatusUnprocessableEntity},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			a := novoAmbiente(t, permitirTudo)
			resp := enviar(t, a, c.arquivo)
			if resp.StatusCode != c.status {
				t.Fatalf("status = %d, quer %d", resp.StatusCode, c.status)
			}
			if docs := documentos(t, a); len(docs) != 0 {
				t.Errorf("documentos = %d, quer 0", len(docs))
			}
		})
	}
}

func TestUploadAcimaDoLimite(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)
	a.imp.MaxBytes = 32

	resp := enviar(t, a, arquivoTeste{nome: "grande.md", conteudo: strings.Repeat("a", 33)})
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, quer 413", resp.StatusCode)
	}
	if docs := documentos(t, a); len(docs) != 0 {
		t.Errorf("documentos = %d, quer 0", len(docs))
	}
	if nomes, err := os.ReadDir(filepath.Join(a.root, "documents")); err == nil && len(nomes) != 0 {
		t.Errorf("arquivos deixados: %v", nomes)
	}
}

func TestUploadCorpoAcimaDoLimite(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)
	a.srv.MaxBodyBytes = 512

	resp := enviar(t, a, arquivoTeste{nome: "grande.md", conteudo: strings.Repeat("a", 1024)})
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, quer 413", resp.StatusCode)
	}
	if docs := documentos(t, a); len(docs) != 0 {
		t.Errorf("documentos = %d, quer 0", len(docs))
	}
}

func TestUploadSemArquivo(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	if err := mw.WriteField("nota", "sem arquivo"); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	resp, err := semRedirect.Post(a.http.URL+"/documents", mw.FormDataContentType(), &b)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quer 422", resp.StatusCode)
	}
}

func TestUploadSemAuth(t *testing.T) {
	a := novoAmbiente(t, nil)

	resp := enviar(t, a, arquivoTeste{nome: "livro.md", conteudo: "# Livro\n\nTexto.\n"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, quer 401", resp.StatusCode)
	}
	if docs := documentos(t, a); len(docs) != 0 {
		t.Errorf("documentos = %d, quer 0", len(docs))
	}

	saude, err := http.Get(a.http.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	saude.Body.Close()
	if saude.StatusCode != http.StatusOK {
		t.Errorf("healthz = %d", saude.StatusCode)
	}
}

func TestUploadNaoMultipart(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)
	resp, err := semRedirect.Post(a.http.URL+"/documents", "text/plain", strings.NewReader("oi"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, quer 400", resp.StatusCode)
	}
}

func TestRotas(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)

	resp, err := http.Get(a.http.URL + "/documents")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /documents = %d, quer 405", resp.StatusCode)
	}

	saude, err := http.Get(a.http.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	corpo, _ := io.ReadAll(saude.Body)
	saude.Body.Close()
	if saude.StatusCode != http.StatusOK || string(corpo) != "ok\n" {
		t.Errorf("healthz = %d %q", saude.StatusCode, corpo)
	}

	pronto, err := http.Get(a.http.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	pronto.Body.Close()
	if pronto.StatusCode != http.StatusOK {
		t.Errorf("readyz = %d", pronto.StatusCode)
	}

	semBanco := &web.Server{}
	ts := httptest.NewServer(semBanco.Handler())
	defer ts.Close()
	indisponivel, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	indisponivel.Body.Close()
	if indisponivel.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("readyz sem banco = %d, quer 503", indisponivel.StatusCode)
	}
}

func localizar(t *testing.T, resp *http.Response) *url.URL {
	t.Helper()
	destino, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("location inválida %q: %v", resp.Header.Get("Location"), err)
	}
	return destino
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }
