package web_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/delivery"
	"github.com/giovani/readingdrip/internal/schedule"
)

// remetenteFake registra as chamadas de envio manual e de teste.
type remetenteFake struct {
	enviados   []int64
	resultado  schedule.SendNowResult
	erroEnvio  error
	teste      delivery.SendResult
	erroTeste  error
	chamouTest bool
}

func (r *remetenteFake) SendNow(_ context.Context, id int64) (schedule.SendNowResult, error) {
	r.enviados = append(r.enviados, id)
	if r.erroEnvio != nil {
		return schedule.SendNowResult{}, r.erroEnvio
	}
	res := r.resultado
	res.DocumentID = id
	if res.RunID == 0 {
		res.RunID = 7
	}
	if res.ChunkID == 0 {
		res.ChunkID = 11
	}
	return res, nil
}

func (r *remetenteFake) SendTest(context.Context) (delivery.SendResult, error) {
	r.chamouTest = true
	return r.teste, r.erroTeste
}

func formConfig(extra url.Values) url.Values {
	v := url.Values{
		"dias":            {"mon", "tue", "wed", "thu", "fri"},
		"horario":         {"07:00"},
		"fuso":            {"America/Sao_Paulo"},
		"catchup":         {"1"},
		"max_docs":        {"5"},
		"ntfy_url":        {"http://ntfy:8080"},
		"ntfy_topico":     {"leitura"},
		"ntfy_prioridade": {"3"},
		"modo_corpo":      {"full"},
		"max_bytes":       {"60000"},
	}
	for k, vs := range extra {
		v[k] = vs
	}
	return v
}

func TestConfiguracoesSemSessao(t *testing.T) {
	a, _ := ambienteComLogin(t)

	casos := []struct {
		metodo  string
		caminho string
	}{
		{http.MethodGet, "/settings"},
		{http.MethodPost, "/settings"},
		{http.MethodPost, "/settings/test"},
		{http.MethodPost, "/documents/1/send-now"},
	}
	for _, caso := range casos {
		var resp *http.Response
		if caso.metodo == http.MethodGet {
			resp = getComCookies(t, a, caso.caminho)
		} else {
			resp = postForm(t, a, caso.caminho, url.Values{})
		}
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("%s %s: status = %d, quer 303", caso.metodo, caso.caminho, resp.StatusCode)
			continue
		}
		if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/login") {
			t.Errorf("%s %s: Location = %q", caso.metodo, caso.caminho, loc)
		}
	}
}

func TestConfiguracoesMostraPadroes(t *testing.T) {
	a, _ := ambienteComLogin(t)
	ctx := context.Background()
	if err := a.store.Settings().Set(ctx, schedule.KeyNtfyToken, "tk_segredo", agora); err != nil {
		t.Fatal(err)
	}
	cookie := entrar(t, a, "/settings")

	resp := getComCookies(t, a, "/settings", cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)

	for _, trecho := range []string{
		`action="/settings"`,
		`name="horario"`,
		`value="07:00"`,
		`name="fuso"`,
		`value="America/Sao_Paulo"`,
		`name="ntfy_token"`,
		`name="dias"`,
		`value="mon"`,
		`action="/settings/test"`,
		`token salvo`,
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página sem %q", trecho)
		}
	}
	if strings.Contains(corpo, "tk_segredo") {
		t.Error("token não pode ser renderizado")
	}
	if !strings.Contains(corpo, `checked`) {
		t.Error("dias/agendador deveriam vir marcados nos padrões")
	}
}

func TestConfiguracoesSalva(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/settings")

	form := formConfig(url.Values{
		"agenda_ligada": {"on"},
		"horario":       {"21:45"},
		"catchup":       {"3"},
		"max_docs":      {"2"},
		"ntfy_topico":   {"leituras"},
		"ntfy_token":    {"tk_novo"},
		"dias":          {"mon", "wed", "sun"},
	})
	resp := postForm(t, a, "/settings", form, cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/settings?salvo=1" {
		t.Errorf("Location = %q", loc)
	}

	st, err := schedule.LoadSettings(context.Background(), a.store.Settings())
	if err != nil {
		t.Fatalf("carregar configurações: %v", err)
	}
	if !st.Schedule.Enabled || st.Schedule.Time != "21:45" || st.Schedule.CatchupDays != 3 || st.Schedule.MaxDocsPerRun != 2 {
		t.Errorf("agenda = %+v", st.Schedule)
	}
	if len(st.Schedule.Days) != 3 || st.Schedule.Days[0] != time.Monday || st.Schedule.Days[2] != time.Sunday {
		t.Errorf("dias = %v", st.Schedule.Days)
	}
	if st.Ntfy.Topic != "leituras" || st.Ntfy.Token != "tk_novo" || st.Ntfy.Priority != 3 {
		t.Errorf("ntfy = %+v", st.Ntfy)
	}

	// GET depois do POST mostra o sucesso e os valores salvos.
	pagina := lerCorpo(t, getComCookies(t, a, "/settings?salvo=1", cookie))
	if !strings.Contains(pagina, "Configurações salvas") || !strings.Contains(pagina, `value="21:45"`) {
		t.Error("página não refletiu o salvamento")
	}
	if strings.Contains(pagina, "tk_novo") {
		t.Error("token não pode ser renderizado depois de salvar")
	}
	if !strings.Contains(pagina, "Remover o token salvo") {
		t.Error("checkbox de remoção deveria aparecer com token definido")
	}
}

func TestConfiguracoesValorInvalidoNaoGrava(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/settings")

	resp := postForm(t, a, "/settings", formConfig(url.Values{"horario": {"25:00"}}), cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quer 422", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)
	if !strings.Contains(corpo, "schedule.time") {
		t.Errorf("erro deveria citar a chave: %s", corpo)
	}
	if !strings.Contains(corpo, `value="25:00"`) {
		t.Error("formulário deveria preservar o valor digitado")
	}

	if _, ok, err := a.store.Settings().Get(context.Background(), schedule.KeyTime); err != nil || ok {
		t.Errorf("valor inválido não pode ser gravado: ok=%v err=%v", ok, err)
	}
}

func TestConfiguracoesToken(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/settings")
	ctx := context.Background()
	if err := a.store.Settings().Set(ctx, schedule.KeyNtfyToken, "tk_antigo", agora); err != nil {
		t.Fatal(err)
	}

	// Em branco mantém o token salvo.
	resp := postForm(t, a, "/settings", formConfig(nil), cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if v, _, _ := a.store.Settings().Get(ctx, schedule.KeyNtfyToken); v != "tk_antigo" {
		t.Errorf("token = %q, quer manter o anterior", v)
	}

	// Remover grava vazio.
	resp = postForm(t, a, "/settings", formConfig(url.Values{"ntfy_remover_token": {"on"}}), cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if v, ok, _ := a.store.Settings().Get(ctx, schedule.KeyNtfyToken); !ok || v != "" {
		t.Errorf("token = %q (ok=%v), quer vazio", v, ok)
	}
}

func TestConfiguracoesTesteOK(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/settings")
	fake := &remetenteFake{teste: delivery.SendResult{Status: 200, Bytes: 512, Attempts: 1, Latency: 42 * time.Millisecond}}
	a.srv.Sender = fake

	resp := postForm(t, a, "/settings/test", url.Values{}, cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	if !fake.chamouTest {
		t.Error("SendTest não foi chamado")
	}
	loc := resp.Header.Get("Location")
	for _, trecho := range []string{"teste=ok", "http=200", "ms=42", "bytes=512"} {
		if !strings.Contains(loc, trecho) {
			t.Errorf("Location = %q sem %q", loc, trecho)
		}
	}

	pagina := lerCorpo(t, getComCookies(t, a, loc, cookie))
	if !strings.Contains(pagina, "Teste enviado: HTTP 200 · 42 ms · 512 bytes.") {
		t.Errorf("página sem o resumo do teste: %s", pagina)
	}
}

func TestConfiguracoesTesteErro(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/settings")
	a.srv.Sender = &remetenteFake{erroTeste: schedule.ErrNtfyNaoConfigurado}

	resp := postForm(t, a, "/settings/test", url.Values{}, cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "teste=erro") {
		t.Fatalf("Location = %q", loc)
	}
	pagina := lerCorpo(t, getComCookies(t, a, loc, cookie))
	if !strings.Contains(pagina, "Teste falhou: configure a URL e o tópico do ntfy") {
		t.Errorf("página sem o motivo: %s", pagina)
	}

	a.srv.Sender = &remetenteFake{erroTeste: errors.New("ntfy: resposta HTTP 401")}
	resp = postForm(t, a, "/settings/test", url.Values{}, cookie)
	pagina = lerCorpo(t, getComCookies(t, a, resp.Header.Get("Location"), cookie))
	if !strings.Contains(pagina, "HTTP 401") {
		t.Errorf("página sem o erro do ntfy: %s", pagina)
	}
}

func TestConfiguracoesTesteSemRemetente(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/settings")
	a.srv.Sender = nil

	if resp := postForm(t, a, "/settings/test", url.Values{}, cookie); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, quer 503", resp.StatusCode)
	}
	if resp := postForm(t, a, "/documents/1/send-now", url.Values{}, cookie); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, quer 503", resp.StatusCode)
	}
}

func TestEnviarAgora(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/")
	doc := importarDoc(t, a, "manual.md", "# Capítulo\n\n"+strings.Repeat("Frase de teste para envio manual. ", 120))
	processar(t, a, doc.ID)

	fake := &remetenteFake{}
	a.srv.Sender = fake

	resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/send-now", url.Values{}, cookie)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/documents/"+itoa(doc.ID)+"?enviado=1" {
		t.Errorf("Location = %q", loc)
	}
	if len(fake.enviados) != 1 || fake.enviados[0] != doc.ID {
		t.Errorf("enviados = %v", fake.enviados)
	}

	pagina := lerCorpo(t, getComCookies(t, a, "/documents/"+itoa(doc.ID)+"?enviado=1", cookie))
	if !strings.Contains(pagina, "Trecho enviado para o ntfy.") {
		t.Error("detalhe não mostrou o sucesso do envio")
	}
}

func TestEnviarAgoraAvisos(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/")
	doc := importarDoc(t, a, "avisos.md", "# Capítulo\n\n"+strings.Repeat("Texto para o envio. ", 100))
	processar(t, a, doc.ID)

	casos := []struct {
		nome     string
		erro     error
		aviso    string
		mensagem string
	}{
		{"sem trechos", schedule.ErrSemTrechos, "sem-trechos", "Não há trechos pendentes"},
		{"já enviado", schedule.ErrJaEnviadoHoje, "ja-enviado", "A run de hoje já entregou"},
		{"sem ntfy", schedule.ErrNtfyNaoConfigurado, "ntfy", "Configure a URL e o tópico"},
		{"ocupado", schedule.ErrEnvioEmAndamento, "ocupado", "Uma entrega está em andamento"},
		{"não pronto", schedule.ErrDocumentoNaoPronto, "indisponivel", "precisa estar pronto"},
		{"tempo", context.DeadlineExceeded, "tempo", "não respondeu a tempo"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			a.srv.Sender = &remetenteFake{erroEnvio: caso.erro}
			resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/send-now", url.Values{}, cookie)
			loc := resp.Header.Get("Location")
			if !strings.Contains(loc, "aviso="+caso.aviso) {
				t.Fatalf("Location = %q, quer aviso=%s", loc, caso.aviso)
			}
			pagina := lerCorpo(t, getComCookies(t, a, loc, cookie))
			if !strings.Contains(pagina, caso.mensagem) {
				t.Errorf("detalhe sem %q", caso.mensagem)
			}
		})
	}

	t.Run("falha de publicação", func(t *testing.T) {
		a.srv.Sender = &remetenteFake{erroEnvio: errors.New("ntfy: resposta HTTP 500")}
		resp := postForm(t, a, "/documents/"+itoa(doc.ID)+"/send-now", url.Values{}, cookie)
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "aviso=falha") || !strings.Contains(loc, "motivo=") {
			t.Fatalf("Location = %q", loc)
		}
		pagina := lerCorpo(t, getComCookies(t, a, loc, cookie))
		if !strings.Contains(pagina, "Não foi possível enviar: ntfy: resposta HTTP 500") {
			t.Errorf("detalhe sem o erro: %s", pagina)
		}
	})
}

func TestEnviarAgoraIdInvalido(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/")
	a.srv.Sender = &remetenteFake{}

	resp := postForm(t, a, "/documents/abc/send-now", url.Values{}, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, quer 404", resp.StatusCode)
	}
}

func TestDetalheMostraBotaoEnviarAgora(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/")
	doc := importarDoc(t, a, "botao.md", "# Capítulo\n\n"+strings.Repeat("Texto do botão. ", 100))
	processar(t, a, doc.ID)

	pagina := lerCorpo(t, getComCookies(t, a, "/documents/"+itoa(doc.ID), cookie))
	if !strings.Contains(pagina, `action="/documents/`+itoa(doc.ID)+`/send-now"`) {
		t.Error("detalhe de documento pronto deveria ter o botão de envio")
	}
}

func TestDocumentoNaoMostraEnvioSemPlano(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/")
	doc := importarDoc(t, a, "sem-plano.md", "# Capítulo\n\n"+strings.Repeat("Texto. ", 100))

	// Ainda em processing, sem plano: botão não aparece.
	pagina := lerCorpo(t, getComCookies(t, a, "/documents/"+itoa(doc.ID), cookie))
	if strings.Contains(pagina, "/send-now") {
		t.Error("documento sem plano não deveria oferecer envio manual")
	}
}
