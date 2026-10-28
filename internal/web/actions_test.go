package web_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/store"
)

func docPronto(t *testing.T, a *ambiente, nome, conteudo string) *store.Document {
	t.Helper()
	doc := importarDoc(t, a, nome, conteudo)
	processar(t, a, doc.ID)
	pronto, err := a.store.Documents().Get(context.Background(), doc.ID)
	if err != nil || pronto == nil {
		t.Fatalf("buscar documento pronto: %+v, %v", pronto, err)
	}
	if pronto.Status != store.StatusReady {
		t.Fatalf("status = %q, quer ready", pronto.Status)
	}
	return pronto
}

func statusDe(t *testing.T, a *ambiente, id int64) store.DocumentStatus {
	t.Helper()
	doc, err := a.store.Documents().Get(context.Background(), id)
	if err != nil || doc == nil {
		t.Fatalf("buscar documento %d: %+v, %v", id, doc, err)
	}
	return doc.Status
}

func requisitar(t *testing.T, a *ambiente, metodo, caminho string, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(metodo, a.http.URL+caminho, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := semRedirect.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", metodo, caminho, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func ultimoEnfileirado(f *filaFalsa) (int64, bool) {
	ids := f.enfileirados()
	if len(ids) == 0 {
		return 0, false
	}
	return ids[len(ids)-1], true
}

func TestAcoesSemSessao(t *testing.T) {
	a, _ := ambienteComLogin(t)
	casos := []struct {
		metodo  string
		caminho string
	}{
		{http.MethodPost, "/documents/1/pause"},
		{http.MethodPost, "/documents/1/resume"},
		{http.MethodPost, "/documents/1/reprocess"},
		{http.MethodGet, "/documents/1/delete"},
		{http.MethodPost, "/documents/1/delete"},
		{http.MethodDelete, "/documents/1"},
		{http.MethodGet, "/history"},
	}
	for _, caso := range casos {
		resp := requisitar(t, a, caso.metodo, caso.caminho)
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("%s %s: status = %d, quer 303", caso.metodo, caso.caminho, resp.StatusCode)
			continue
		}
		if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/login") {
			t.Errorf("%s %s: Location = %q", caso.metodo, caso.caminho, loc)
		}
	}
}

func TestPausarRetomarDocumento(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := docPronto(t, a, "Pausa.md", "# Pausa\n\nConteúdo.\n")
	cookie := entrar(t, a, "/")

	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/pause", url.Values{}, cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("pausar: status = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?acao=pausado" {
		t.Errorf("Location = %q", loc)
	}
	if s := statusDe(t, a, doc.ID); s != store.StatusPaused {
		t.Fatalf("status = %q, quer paused", s)
	}

	// Pausar de novo é inofensivo e não muda o estado.
	resp = postForm(t, a, "/documents/"+itoa(doc.ID)+"/pause", url.Values{}, cookie)
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?acao=pausado" {
		t.Errorf("segunda pausa: Location = %q", loc)
	}
	if s := statusDe(t, a, doc.ID); s != store.StatusPaused {
		t.Errorf("status após segunda pausa = %q", s)
	}

	resp = postForm(t, a, "/documents/"+itoa(doc.ID)+"/resume", url.Values{}, cookie)
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?acao=retomado" {
		t.Errorf("retomar: Location = %q", loc)
	}
	if s := statusDe(t, a, doc.ID); s != store.StatusReady {
		t.Fatalf("status = %q, quer ready", s)
	}

	resp = postForm(t, a, "/documents/"+itoa(doc.ID)+"/resume", url.Values{}, cookie)
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?acao=retomado" {
		t.Errorf("segundo retomar: Location = %q", loc)
	}
}

func TestPausarDocumentoEmProcessamento(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := importarDoc(t, a, "Novo.md", "# Novo\n\nConteúdo.\n")
	cookie := entrar(t, a, "/")

	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/pause", url.Values{}, cookie)
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?aviso=indisponivel" {
		t.Errorf("Location = %q", loc)
	}
	if s := statusDe(t, a, doc.ID); s != store.StatusProcessing {
		t.Errorf("status = %q, quer processing", s)
	}
}

func TestDetalheMostraAcoes(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := docPronto(t, a, "Ações.md", "# Ações\n\nConteúdo.\n")
	cookie := entrar(t, a, "/")

	corpo := lerCorpo(t, getComCookies(t, a, "/documents/"+itoa(doc.ID), cookie))
	for _, trecho := range []string{
		`action="/documents/` + itoa(doc.ID) + `/pause"`,
		`action="/documents/` + itoa(doc.ID) + `/reprocess"`,
		`href="/documents/` + itoa(doc.ID) + `/delete"`,
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("detalhe ready sem %q", trecho)
		}
	}

	if _, err := a.store.Documents().TransitionStatus(context.Background(), doc.ID,
		[]store.DocumentStatus{store.StatusReady}, store.StatusPaused, agora); err != nil {
		t.Fatal(err)
	}
	corpo = lerCorpo(t, getComCookies(t, a, "/documents/"+itoa(doc.ID), cookie))
	for _, trecho := range []string{
		`action="/documents/` + itoa(doc.ID) + `/resume"`,
		"Documento pausado",
		`href="/documents/` + itoa(doc.ID) + `/delete"`,
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("detalhe paused sem %q", trecho)
		}
	}
	if strings.Contains(corpo, `/pause"`) {
		t.Error("detalhe paused não deve oferecer pausar de novo")
	}
}

func TestReprocessarDocumento(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := docPronto(t, a, "Reprocessar.md", "# Reprocessar\n\nConteúdo.\n")
	cookie := entrar(t, a, "/")

	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/reprocess", url.Values{}, cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?acao=reprocessando" {
		t.Errorf("Location = %q", loc)
	}
	if s := statusDe(t, a, doc.ID); s != store.StatusProcessing {
		t.Fatalf("status = %q, quer processing", s)
	}
	if id, ok := ultimoEnfileirado(a.fila); !ok || id != doc.ID {
		t.Errorf("último enfileirado = %d, %v; quer %d", id, ok, doc.ID)
	}

	// Com a extração em andamento, um segundo clique não reenfileira.
	resp = postForm(t, a, "/documents/"+itoa(doc.ID)+"/reprocess", url.Values{}, cookie)
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?aviso=processando" {
		t.Errorf("reprocessar em andamento: Location = %q", loc)
	}
}

func TestReprocessarComHistorico(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := docPronto(t, a, "Histórico.md", "# Histórico\n\nConteúdo.\n")
	ctx := context.Background()
	plano, err := a.store.Plans().Active(ctx, doc.ID)
	if err != nil || plano == nil || len(plano.Chunks) == 0 {
		t.Fatalf("plano ativo: %+v, %v", plano, err)
	}
	if err := a.store.Plans().MarkSent(ctx, plano.Chunks[0].ID, agora); err != nil {
		t.Fatal(err)
	}
	cookie := entrar(t, a, "/")

	antes := len(a.fila.enfileirados())
	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/reprocess", url.Values{}, cookie)
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?aviso=historico" {
		t.Errorf("Location = %q", loc)
	}
	if s := statusDe(t, a, doc.ID); s != store.StatusReady {
		t.Errorf("status = %q, quer ready", s)
	}
	if depois := len(a.fila.enfileirados()); depois != antes {
		t.Errorf("fila = %v, quer sem novo enfileiramento", a.fila.enfileirados())
	}
}

func TestReprocessarComFilaIndisponivel(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := docPronto(t, a, "Fila.md", "# Fila\n\nConteúdo.\n")
	cookie := entrar(t, a, "/")

	a.fila.erro = errors.New("fila cheia")
	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/reprocess", url.Values{}, cookie)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, quer 500", resp.StatusCode)
	}
	if s := statusDe(t, a, doc.ID); s != store.StatusReady {
		t.Errorf("status após falha da fila = %q, quer ready", s)
	}
}

func TestExcluirDocumento(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := docPronto(t, a, "Excluir.md", "# Excluir\n\nConteúdo.\n")
	cookie := entrar(t, a, "/")
	caminho := "/documents/" + itoa(doc.ID)

	resp := getComCookies(t, a, caminho+"/delete", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirmação: status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)
	for _, trecho := range []string{
		"Excluir documento?",
		`action="` + caminho + `/delete"`,
		"Excluir definitivamente",
		"Cancelar",
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("confirmação sem %q", trecho)
		}
	}

	resp = postForm(t, a, caminho+"/delete", url.Values{}, cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("excluir: status = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/?excluido=1" {
		t.Errorf("Location = %q", loc)
	}
	if d, err := a.store.Documents().Get(context.Background(), doc.ID); err != nil || d != nil {
		t.Errorf("documento após exclusão = %+v, %v", d, err)
	}
	if _, err := os.Stat(filepath.Join(a.root, "documents", itoa(doc.ID))); !os.IsNotExist(err) {
		t.Errorf("diretório do documento ainda existe: %v", err)
	}

	corpo = lerCorpo(t, getComCookies(t, a, "/?excluido=1", cookie))
	if !strings.Contains(corpo, "1 documento excluído.") {
		t.Error("biblioteca sem o aviso de exclusão")
	}

	// Repetir responde 404.
	resp = postForm(t, a, caminho+"/delete", url.Values{}, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("segunda exclusão: status = %d, quer 404", resp.StatusCode)
	}
}

func TestExcluirViaDELETE(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc := docPronto(t, a, "Api.md", "# Api\n\nConteúdo.\n")
	cookie := entrar(t, a, "/")

	resp := requisitar(t, a, http.MethodDelete, "/documents/"+itoa(doc.ID), cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	if d, err := a.store.Documents().Get(context.Background(), doc.ID); err != nil || d != nil {
		t.Errorf("documento após DELETE = %+v, %v", d, err)
	}
}

func TestHistorico(t *testing.T) {
	a, _ := ambienteComLogin(t)
	ctx := context.Background()
	doc := docPronto(t, a, "Histórico.md", "# Histórico\n\nConteúdo.\n")
	plano, err := a.store.Plans().Active(ctx, doc.ID)
	if err != nil || plano == nil || len(plano.Chunks) == 0 {
		t.Fatalf("plano ativo: %+v, %v", plano, err)
	}

	run, _, err := a.store.Runs().Create(ctx, "2026-10-27", agora, agora, false)
	if err != nil {
		t.Fatal(err)
	}
	item, err := a.store.Runs().AddItem(ctx, run.ID, doc.ID, plano.Chunks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Runs().MarkItemSent(ctx, item.ID, plano.Chunks[0].ID, 1200, 2, agora); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Runs().FinishRun(ctx, run.ID, store.RunSent, "", agora); err != nil {
		t.Fatal(err)
	}

	cookie := entrar(t, a, "/history")
	resp := getComCookies(t, a, "/history", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)
	for _, trecho := range []string{
		"<h1>Histórico</h1>",
		"2026-10-27",
		"Enviada",
		"Histórico",
		"Enviado",
		"1.2 KiB",
		`name="documento"`,
		`href="/documents/` + itoa(doc.ID) + `"`,
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("histórico sem %q", trecho)
		}
	}

	// Filtro por outro documento não traz a run.
	resp = getComCookies(t, a, "/history?documento=9999", cookie)
	corpo = lerCorpo(t, resp)
	if !strings.Contains(corpo, "Nenhuma entrega registrada ainda.") {
		t.Error("filtro sem resultado deveria mostrar o vazio")
	}

	// Filtro do próprio documento e filtro inválido continuam mostrando a run.
	for _, caminho := range []string{
		"/history?documento=" + itoa(doc.ID),
		"/history?documento=abc",
	} {
		corpo = lerCorpo(t, getComCookies(t, a, caminho, cookie))
		if !strings.Contains(corpo, "2026-10-27") {
			t.Errorf("%s: run esperada não apareceu", caminho)
		}
	}
}

func TestHistoricoMostraFalha(t *testing.T) {
	a, _ := ambienteComLogin(t)
	ctx := context.Background()
	doc := docPronto(t, a, "Falha.md", "# Falha\n\nConteúdo.\n")
	plano, err := a.store.Plans().Active(ctx, doc.ID)
	if err != nil || plano == nil || len(plano.Chunks) == 0 {
		t.Fatalf("plano ativo: %+v, %v", plano, err)
	}

	run, _, err := a.store.Runs().Create(ctx, "2026-10-28", agora, agora, true)
	if err != nil {
		t.Fatal(err)
	}
	item, err := a.store.Runs().AddItem(ctx, run.ID, doc.ID, plano.Chunks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Runs().MarkItemFailed(ctx, item.ID, 3, 0, "ntfy: 500 Internal Server Error"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Runs().FinishRun(ctx, run.ID, store.RunFailed, "todos falharam", agora); err != nil {
		t.Fatal(err)
	}

	cookie := entrar(t, a, "/history")
	corpo := lerCorpo(t, getComCookies(t, a, "/history", cookie))
	for _, trecho := range []string{
		"recuperação",
		"Falhou",
		"todos falharam",
		"ntfy: 500 Internal Server Error",
		"3",
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("histórico de falha sem %q", trecho)
		}
	}
}
