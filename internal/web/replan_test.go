package web_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/plan"
	"github.com/giovani/readingdrip/internal/store"
)

func textoWebLongo() string {
	var b strings.Builder
	b.WriteString("# Livro Longo\n\n")
	for i := 0; i < 250; i++ {
		fmt.Fprintf(&b, "Parágrafo %d com algumas palavras para dar corpo ao texto e permitir um plano de leitura razoável.\n\n", i)
	}
	return b.String()
}

func prepararPlanoLongo(t *testing.T, a *ambiente) (*store.Document, *store.Plan) {
	t.Helper()
	doc := docPronto(t, a, "Longo.md", textoWebLongo())
	plano, err := a.store.Plans().Active(context.Background(), doc.ID)
	if err != nil || plano == nil {
		t.Fatalf("plano ativo: %+v, %v", plano, err)
	}
	if len(plano.Chunks) < 2 {
		t.Fatalf("chunks = %d, o teste precisa de pelo menos 2", len(plano.Chunks))
	}
	return doc, plano
}

func TestRegenerarPlanoAPartirDoCursor(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc, plano := prepararPlanoLongo(t, a)
	if err := a.store.Plans().MarkSent(context.Background(), plano.Chunks[0].ID, agora); err != nil {
		t.Fatalf("marcar primeiro chunk enviado: %v", err)
	}
	cursor := plano.Chunks[1]
	cookie := entrar(t, a, "/")

	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/plan/regenerate", url.Values{
		"modo":    {"finish_by_days"},
		"dias":    {"5"},
		"minutos": {"10"},
	}, cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?acao=regenerado" {
		t.Errorf("Location = %q", loc)
	}

	ativo, err := a.store.Plans().Active(context.Background(), doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ativo == nil || ativo.Version != 2 {
		t.Fatalf("plano ativo = %+v, quer versão 2", ativo)
	}
	if got := ativo.Chunks[0].FirstBlockID; got != cursor.FirstBlockID {
		t.Errorf("primeiro bloco = %d, quer %d (cursor)", got, cursor.FirstBlockID)
	}
	if ativo.Params.Mode != plan.ModeFinishByDays || ativo.Params.Days != 5 {
		t.Errorf("params = %+v", ativo.Params)
	}

	atualizado, err := a.store.Documents().Get(context.Background(), doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if atualizado.PacingMode != string(plan.ModeFinishByDays) || atualizado.PacingDays == nil || *atualizado.PacingDays != 5 {
		t.Errorf("ritmo = %q/%v", atualizado.PacingMode, atualizado.PacingDays)
	}

	corpo := lerCorpo(t, getComCookies(t, a, "/documents/"+itoa(doc.ID)+"?acao=regenerado", cookie))
	if !strings.Contains(corpo, "Plano regenerado a partir do trecho pendente") {
		t.Error("detalhe não mostra o aviso de plano regenerado")
	}
}

func TestRegenerarPlanoRitmoInvalido(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc, _ := prepararPlanoLongo(t, a)
	cookie := entrar(t, a, "/")

	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/plan/regenerate", url.Values{
		"modo":    {"minutes_per_day"},
		"minutos": {"0"},
	}, cookie)
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?aviso=ritmo" {
		t.Errorf("Location = %q", loc)
	}

	ativo, err := a.store.Plans().Active(context.Background(), doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ativo.Version != 1 {
		t.Errorf("versão = %d, quer 1 (nada deveria mudar)", ativo.Version)
	}
}

func TestRegenerarPlanoSemTrechosPendentes(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc, plano := prepararPlanoLongo(t, a)
	for _, c := range plano.Chunks {
		if err := a.store.Plans().MarkSent(context.Background(), c.ID, agora); err != nil {
			t.Fatal(err)
		}
	}
	cookie := entrar(t, a, "/")

	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/plan/regenerate", url.Values{
		"modo":    {"minutes_per_day"},
		"minutos": {"10"},
	}, cookie)
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?aviso=sem-trechos" {
		t.Errorf("Location = %q", loc)
	}

	corpo := lerCorpo(t, getComCookies(t, a, "/documents/"+itoa(doc.ID), cookie))
	if strings.Contains(corpo, `/plan/regenerate"`) {
		t.Error("detalhe não deveria oferecer regeneração sem trecho pendente")
	}
}

func TestDetalheMostraFormularioDeRitmo(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc, _ := prepararPlanoLongo(t, a)
	cookie := entrar(t, a, "/")

	corpo := lerCorpo(t, getComCookies(t, a, "/documents/"+itoa(doc.ID), cookie))
	for _, trecho := range []string{
		`action="/documents/` + itoa(doc.ID) + `/plan/regenerate"`,
		`name="modo"`,
		`name="minutos"`,
		`name="dias"`,
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("detalhe sem %q", trecho)
		}
	}
}
