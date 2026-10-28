package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/platform"
	"github.com/giovani/readingdrip/internal/store"
)

func configTeste(t *testing.T) platform.Config {
	t.Helper()
	return platform.Config{
		Addr:           "127.0.0.1:0",
		DataDir:        t.TempDir(),
		BaseURL:        "http://localhost:8080",
		Password:       "senha-de-teste",
		Secret:         []byte(strings.Repeat("k", 32)),
		MaxUploadBytes: platform.DefaultMaxUploadBytes,
		LogLevel:       slog.LevelError,
	}
}

var semRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}}

func get(t *testing.T, base, caminho string) *http.Response {
	t.Helper()
	resp, err := semRedirect.Get(base + caminho)
	if err != nil {
		t.Fatalf("GET %s: %v", caminho, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestMontarServeTelas(t *testing.T) {
	app, err := montar(context.Background(), configTeste(t), platform.NewLogger(slog.LevelError))
	if err != nil {
		t.Fatalf("montar: %v", err)
	}
	defer app.Close()

	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	if resp := get(t, ts.URL, "/healthz"); resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz = %d", resp.StatusCode)
	}
	if resp := get(t, ts.URL, "/readyz"); resp.StatusCode != http.StatusOK {
		t.Errorf("/readyz = %d", resp.StatusCode)
	}
	if resp := get(t, ts.URL, "/login"); resp.StatusCode != http.StatusOK {
		t.Errorf("/login = %d", resp.StatusCode)
	}
	resp := get(t, ts.URL, "/")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("/ sem sessão = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location = %q", loc)
	}
}

func TestMontarRecuperaProcessing(t *testing.T) {
	ctx := context.Background()
	cfg := configTeste(t)

	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	doc := store.Document{
		Title:        "Recuperado",
		Format:       "md",
		SourceName:   "recuperado.md",
		SourcePath:   filepath.Join(cfg.DataDir, "documents", "pendente", "source.md"),
		SourceSHA256: strings.Repeat("a", 64),
		Status:       store.StatusProcessing,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	id, err := st.Documents().Create(ctx, doc)
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	doc.SourcePath = filepath.Join(cfg.DataDir, "documents", strconv.FormatInt(id, 10), "source.md")
	if err := os.MkdirAll(filepath.Dir(doc.SourcePath), 0o750); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := os.WriteFile(doc.SourcePath, []byte("# Recuperado\n\nConteúdo depois do restart.\n"), 0o600); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.Documents().UpdateSource(ctx, id, doc.SourceName, doc.SourcePath, time.Now()); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	app, err := montar(ctx, cfg, platform.NewLogger(slog.LevelError))
	if err != nil {
		t.Fatalf("montar: %v", err)
	}
	defer app.Close()

	limite := time.Now().Add(3 * time.Second)
	for {
		atual, err := app.store.Documents().Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if atual != nil && atual.Status == store.StatusReady {
			plano, err := app.store.Plans().Active(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if plano == nil || len(plano.Chunks) == 0 {
				t.Fatal("documento ficou ready sem plano")
			}
			break
		}
		if time.Now().After(limite) {
			status := "nil"
			if atual != nil {
				status = string(atual.Status)
			}
			t.Fatalf("documento não ficou ready; status = %s", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMontarProcessaUpload(t *testing.T) {
	app, err := montar(context.Background(), configTeste(t), platform.NewLogger(slog.LevelError))
	if err != nil {
		t.Fatalf("montar: %v", err)
	}
	defer app.Close()

	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	resp, err := semRedirect.PostForm(ts.URL+"/login", map[string][]string{"senha": {"senha-de-teste"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login = %d", resp.StatusCode)
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d", len(cookies))
	}

	var corpo bytes.Buffer
	mw := multipart.NewWriter(&corpo)
	parte, err := mw.CreateFormFile("files", "Upload.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parte.Write([]byte("# Upload\n\nConteúdo enviado pelo entrypoint.\n")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/documents", &corpo)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(cookies[0])
	resp, err = semRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("upload = %d", resp.StatusCode)
	}
	destino := resp.Header.Get("Location")
	if !strings.HasPrefix(destino, "/documents/") {
		t.Fatalf("Location = %q", destino)
	}

	id, err := strconv.ParseInt(strings.TrimPrefix(destino, "/documents/"), 10, 64)
	if err != nil {
		t.Fatalf("id em %q: %v", destino, err)
	}

	limite := time.Now().Add(3 * time.Second)
	for {
		doc, err := app.store.Documents().Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if doc != nil && doc.Status == store.StatusReady {
			break
		}
		if time.Now().After(limite) {
			status := "nil"
			if doc != nil {
				status = string(doc.Status)
			}
			t.Fatalf("documento não ficou ready; status = %s", status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	req, err = http.NewRequest(http.MethodGet, ts.URL+destino, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookies[0])
	resp, err = semRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	corpoPagina, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(corpoPagina), "Plano de leitura") {
		t.Error("detalhe sem plano depois do upload")
	}
}

func TestDocumentosPendentes(t *testing.T) {
	docs := []store.Document{
		{ID: 1, Status: store.StatusReady},
		{ID: 2, Status: store.StatusProcessing},
		{ID: 3, Status: store.StatusFailed},
		{ID: 4, Status: store.StatusProcessing},
	}
	ids := documentosPendentes(docs)
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 4 {
		t.Errorf("ids = %v, quer [2 4]", ids)
	}
	if got := documentosPendentes(nil); len(got) != 0 {
		t.Errorf("vazio = %v", got)
	}
}
