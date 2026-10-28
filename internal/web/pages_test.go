package web_test

import (
	"context"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/ingest"
	"github.com/giovani/readingdrip/internal/library"
	"github.com/giovani/readingdrip/internal/store"
)

func importarDoc(t *testing.T, a *ambiente, nome, conteudo string) *store.Document {
	t.Helper()
	doc, err := a.imp.Import(context.Background(), library.Entrada{
		Name:   nome,
		Reader: strings.NewReader(conteudo),
	})
	if err != nil {
		t.Fatalf("importar %s: %v", nome, err)
	}
	return doc
}

func processar(t *testing.T, a *ambiente, id int64) {
	t.Helper()
	servico := &ingest.Service{
		Store: a.store,
		Now:   func() time.Time { return agora },
	}
	if _, err := servico.Process(context.Background(), id); err != nil {
		t.Fatalf("processar %d: %v", id, err)
	}
}

func TestBibliotecaSemSessao(t *testing.T) {
	a, _ := ambienteComLogin(t)
	resp := getComCookies(t, a, "/")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location = %q", loc)
	}
}

func TestBibliotecaVazia(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/")

	resp := getComCookies(t, a, "/", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)
	for _, trecho := range []string{
		"<h1>Biblioteca</h1>",
		`action="/documents"`,
		`href="/static/app.css"`,
		`src="/static/app.js"`,
		"Nenhum documento ainda.",
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página sem %q", trecho)
		}
	}
}

func TestBibliotecaListaDocumentos(t *testing.T) {
	a, _ := ambienteComLogin(t)
	pronto := importarDoc(t, a, "Pronto.md", "# Pronto\n\nJá processado.\n")
	processar(t, a, pronto.ID)
	pendente := importarDoc(t, a, "Pendente.md", "# Pendente\n\nAinda na fila.\n")
	cookie := entrar(t, a, "/")

	resp := getComCookies(t, a, "/?importados=2&duplicados=1", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)
	for _, trecho := range []string{
		"Pronto",
		"Pendente",
		"2 arquivos importados.",
		"1 duplicado ignorado.",
		`href="/documents/` + itoa(pronto.ID) + `"`,
		`href="/documents/` + itoa(pendente.ID) + `"`,
		`data-status="ready"`,
		`data-status="processing"`,
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página sem %q", trecho)
		}
	}
}

func TestDetalheProcessando(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := importarDoc(t, a, "Novo.md", "# Novo\n\nConteúdo.\n")
	cookie := entrar(t, a, "/")

	resp := getComCookies(t, a, "/documents/"+itoa(doc.ID), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)
	for _, trecho := range []string{
		`data-status="processing"`,
		`data-status-url="/documents/` + itoa(doc.ID) + `"`,
		"Extraindo o conteúdo",
		"Baixar original",
		`href="/documents/` + itoa(doc.ID) + `/download"`,
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página sem %q", trecho)
		}
	}
	if strings.Contains(corpo, "Plano de leitura") {
		t.Error("documento em processing não deveria exibir plano")
	}
}

func TestDetalheProntoComPlano(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := importarDoc(t, a, "Livro.md", "# Capítulo\n\nPrimeiro parágrafo.\n\n## Seção\n\nSegundo parágrafo.\n")
	processar(t, a, doc.ID)
	cookie := entrar(t, a, "/")

	resp := getComCookies(t, a, "/documents/"+itoa(doc.ID), cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)
	for _, trecho := range []string{
		`data-status="ready"`,
		"Plano de leitura",
		"Ver plano completo",
		`href="/documents/` + itoa(doc.ID) + `/plan"`,
		"Capítulo",
		"Prévia sem datas",
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página sem %q", trecho)
		}
	}
}

func TestDetalheDuplicado(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := importarDoc(t, a, "Igual.md", "# Igual\n\nMesmo conteúdo.\n")
	cookie := entrar(t, a, "/")

	resp := getComCookies(t, a, "/documents/"+itoa(doc.ID)+"?duplicado=1", cookie)
	corpo := lerCorpo(t, resp)
	if !strings.Contains(corpo, "já estava na biblioteca") {
		t.Error("aviso de duplicado ausente")
	}
}

func TestDetalheNaoEncontrado(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/")

	for _, caminho := range []string{"/documents/9999", "/documents/abc", "/documents/-1"} {
		resp := getComCookies(t, a, caminho, cookie)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d, quer 404", caminho, resp.StatusCode)
			continue
		}
		corpo := lerCorpo(t, resp)
		if !strings.Contains(corpo, "Documento não encontrado") {
			t.Errorf("%s: página sem mensagem de 404", caminho)
		}
	}
}

func TestPlanoDoDocumento(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := importarDoc(t, a, "Plano.md", "# Um\n\nPrimeiro.\n\n# Dois\n\nSegundo.\n")
	processar(t, a, doc.ID)
	cookie := entrar(t, a, "/")

	resp := getComCookies(t, a, "/documents/"+itoa(doc.ID)+"/plan", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)
	for _, trecho := range []string{
		"Plano de leitura",
		"versão 1",
		"trechos",
		`class="tabela-plano"`,
		"Um",
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página sem %q", trecho)
		}
	}

	semPlano := importarDoc(t, a, "SemPlano.md", "# Sem plano\n\nTexto.\n")
	resp = getComCookies(t, a, "/documents/"+itoa(semPlano.ID)+"/plan", cookie)
	corpo = lerCorpo(t, resp)
	if !strings.Contains(corpo, "ainda não tem um plano ativo") {
		t.Error("aviso de plano ausente não exibido")
	}
}

func TestDownloadOriginal(t *testing.T) {
	a, _ := ambienteComLogin(t)
	conteudo := "# Baixável\n\nConteúdo original.\n"
	doc := importarDoc(t, a, "Baixável.md", conteudo)
	cookie := entrar(t, a, "/")

	resp := getComCookies(t, a, "/documents/"+itoa(doc.ID)+"/download", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	if corpo := lerCorpo(t, resp); corpo != conteudo {
		t.Errorf("corpo = %q, quer %q", corpo, conteudo)
	}
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err != nil {
		t.Fatalf("Content-Disposition inválido: %v", err)
	}
	if params["filename"] != "Baixável.md" {
		t.Errorf("filename = %q", params["filename"])
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestDownloadSemArquivo(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := importarDoc(t, a, "Some.md", "# Some\n\nSerá apagado.\n")
	cookie := entrar(t, a, "/")

	if err := os.Remove(doc.SourcePath); err != nil {
		t.Fatal(err)
	}
	resp := getComCookies(t, a, "/documents/"+itoa(doc.ID)+"/download", cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, quer 404", resp.StatusCode)
	}
}

func TestDownloadForaDoDiretorio(t *testing.T) {
	a, _ := ambienteComLogin(t)
	fora := filepath.Join(t.TempDir(), "fora.md")
	if err := os.WriteFile(fora, []byte("# Fora\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := a.store.Documents().Create(context.Background(), store.Document{
		Title:        "Fora",
		Format:       "md",
		SourceName:   "fora.md",
		SourcePath:   fora,
		SourceSHA256: strings.Repeat("a", 64),
		CreatedAt:    agora,
		UpdatedAt:    agora,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := entrar(t, a, "/")

	resp := getComCookies(t, a, "/documents/"+itoa(id)+"/download", cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, quer 404", resp.StatusCode)
	}
}

func TestAssetsEstaticos(t *testing.T) {
	a := novoAmbiente(t, permitirTudo)

	casos := []struct {
		caminho string
		ctype   string
	}{
		{"/static/app.css", "text/css"},
		{"/static/app.js", "text/javascript"},
	}
	for _, c := range casos {
		resp := getComCookies(t, a, c.caminho)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d, quer 200", c.caminho, resp.StatusCode)
			continue
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, c.ctype) {
			t.Errorf("%s: Content-Type = %q", c.caminho, ct)
		}
		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age=") {
			t.Errorf("%s: Cache-Control = %q", c.caminho, cc)
		}
		if corpo := lerCorpo(t, resp); len(corpo) == 0 {
			t.Errorf("%s: corpo vazio", c.caminho)
		}
	}

	resp := getComCookies(t, a, "/static/app.js")
	js := lerCorpo(t, resp)
	for _, trecho := range []string{"processing", "setInterval", "location.reload"} {
		if !strings.Contains(js, trecho) {
			t.Errorf("app.js sem %q", trecho)
		}
	}

	resp = getComCookies(t, a, "/static/nao-existe.js")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("arquivo inexistente: status = %d, quer 404", resp.StatusCode)
	}
}
