package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/giovani/readingdrip/internal/delivery"
	"github.com/giovani/readingdrip/internal/planconfig"
	"github.com/giovani/readingdrip/internal/schedule"
	"github.com/giovani/readingdrip/internal/web/views"
)

const (
	// configBodyLimit limita o formulário de configurações (só texto).
	configBodyLimit int64 = 64 << 10
	// testeTimeout limita o envio de teste; o cliente usa uma tentativa só.
	testeTimeout = 20 * time.Second
	// envioTimeout limita o envio manual, também de uma tentativa só.
	envioTimeout = 30 * time.Second
	// motivoMax limita o texto de erro mostrado na UI e na URL.
	motivoMax = 200
)

// Sender entrega pelo ntfy. *schedule.Runner implementa.
type Sender interface {
	SendNow(ctx context.Context, documentID int64) (schedule.SendNowResult, error)
	SendTest(ctx context.Context) (delivery.SendResult, error)
}

// handleConfiguracoes mostra o formulário com os valores salvos (ou os padrões
// quando a chave não existe).
func (s *Server) handleConfiguracoes(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}

	valores, err := s.Store.Settings().All(r.Context())
	if err != nil {
		s.logger().Error("ler configurações", "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar as configurações")
		return
	}

	props := configDosValores(valores)
	props.Salvo = queryBool(r, "salvo")
	props.Teste = testeDaQuery(r)
	// Configuração salva inválida (editada no banco ou por versão antiga) não
	// pode derrubar a página: mostra o aviso e deixa corrigir.
	if _, err := schedule.LoadSettings(r.Context(), s.Store.Settings()); err != nil {
		props.Erro = "A configuração salva tem valores inválidos: " + truncarMotivo(err.Error(), motivoMax)
	}

	s.render(w, r, http.StatusOK, views.Configuracoes(props))
}

// handleSalvarConfiguracoes valida o formulário inteiro com schedule.Parse e
// grava tudo em uma transação. Valor inválido não é persistido.
func (s *Server) handleSalvarConfiguracoes(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, configBodyLimit)
	if err := r.ParseForm(); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			http.Error(w, "formulário acima do limite", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}

	props, valores := configDoFormulario(r)
	// A senha em branco mantém a salva; o Parse valida o par usuário/senha
	// completo, então o valor salvo entra na validação sem ser reescrito pela UI.
	if _, ok := valores[schedule.KeyNtfyPassword]; !ok {
		salva, existe, err := s.Store.Settings().Get(r.Context(), schedule.KeyNtfyPassword)
		if err != nil {
			s.logger().Error("ler senha do ntfy", "erro", err)
			s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar as configurações")
			return
		}
		if existe {
			valores[schedule.KeyNtfyPassword] = salva
			props.SenhaDefinida = true
		}
	}
	if _, err := schedule.Parse(valores); err != nil {
		props.Erro = truncarMotivo(err.Error(), motivoMax)
		s.render(w, r, http.StatusUnprocessableEntity, views.Configuracoes(props))
		return
	}

	if err := s.Store.Settings().SetMany(r.Context(), valores, time.Now()); err != nil {
		s.logger().Error("gravar configurações", "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível salvar as configurações")
		return
	}
	s.logger().Info("configurações salvas", "chaves", len(valores))
	http.Redirect(w, r, "/settings?salvo=1", http.StatusSeeOther)
}

// handleTestarNtfy publica a mensagem de teste com a configuração salva e volta
// para a página com status, latência e bytes (ou o motivo da falha).
func (s *Server) handleTestarNtfy(w http.ResponseWriter, r *http.Request) {
	if s.Sender == nil {
		http.Error(w, "entrega indisponível", http.StatusServiceUnavailable)
		return
	}

	ctx, cancelar := context.WithTimeout(r.Context(), testeTimeout)
	defer cancelar()
	envio, err := s.Sender.SendTest(ctx)
	if err != nil {
		s.logger().Warn("teste do ntfy falhou", "erro", err)
		direcionarTeste(w, r, envio, err)
		return
	}
	s.logger().Info("teste do ntfy enviado",
		"status", envio.Status, "bytes", envio.Bytes, "latencia_ms", envio.Latency.Milliseconds())
	direcionarTeste(w, r, envio, nil)
}

// handleEnviarAgora entrega o próximo trecho do documento fora do horário. O
// envio é síncrono e de uma tentativa; o resultado volta na própria página.
func (s *Server) handleEnviarAgora(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return
	}
	if s.Sender == nil {
		http.Error(w, "entrega indisponível", http.StatusServiceUnavailable)
		return
	}

	ctx, cancelar := context.WithTimeout(r.Context(), envioTimeout)
	defer cancelar()
	resultado, err := s.Sender.SendNow(ctx, id)
	if err == nil {
		s.logger().Info("envio manual concluído",
			"documento", id, "run", resultado.RunID, "chunk", resultado.ChunkID,
			"bytes", resultado.Bytes, "latencia_ms", resultado.Latency.Milliseconds())
		http.Redirect(w, r, destinoEnvio(id, "enviado", "1", ""), http.StatusSeeOther)
		return
	}

	aviso := avisoDoEnvio(err)
	s.logger().Warn("envio manual não concluído", "documento", id, "erro", err)
	http.Redirect(w, r, destinoEnvio(id, "aviso", aviso, motivoDoEnvio(err)), http.StatusSeeOther)
}

// configDosValores traduz as chaves salvas para os campos da tela.
func configDosValores(valores map[string]string) views.ConfigProps {
	texto := func(chave, padrao string) string {
		if v := strings.TrimSpace(valores[chave]); v != "" {
			return v
		}
		return padrao
	}
	ligado := true
	if v := strings.TrimSpace(valores[schedule.KeyEnabled]); v != "" {
		ligado = strings.EqualFold(v, "true")
	}
	dias := schedule.DefaultDays
	if v := strings.TrimSpace(valores[schedule.KeyDays]); v != "" {
		dias = v
	}
	return views.ConfigProps{
		Enabled:       ligado,
		Dias:          idsDosDias(dias),
		Horario:       texto(schedule.KeyTime, schedule.DefaultTime),
		Fuso:          texto(schedule.KeyTimezone, schedule.DefaultTimezone),
		Catchup:       texto(schedule.KeyCatchupDays, strconv.Itoa(schedule.DefaultCatchupDays)),
		MaxDocs:       texto(schedule.KeyMaxDocsPerRun, strconv.Itoa(schedule.DefaultMaxDocsPerRun)),
		NtfyURL:       strings.TrimSpace(valores[schedule.KeyNtfyBaseURL]),
		NtfyTopico:    strings.TrimSpace(valores[schedule.KeyNtfyTopic]),
		TokenDefinido: tokenDefinido(valores),
		NtfyUsuario:   strings.TrimSpace(valores[schedule.KeyNtfyUsername]),
		SenhaDefinida: senhaDefinida(valores),
		Prioridade:    texto(schedule.KeyNtfyPriority, strconv.Itoa(schedule.DefaultNtfyPriority)),
		ModoCorpo:     texto(schedule.KeyNtfyBodyMode, string(delivery.BodyFull)),
		MaxBytes:      texto(schedule.KeyNtfyMaxBytes, strconv.Itoa(schedule.DefaultMaxMessageBytes)),
		PlanWPM:       texto(planconfig.KeyWPM, strconv.Itoa(planconfig.DefaultWPM)),
		PlanDiff:      texto(planconfig.KeyDifficulty, decimal(planconfig.DefaultDifficulty)),
		PlanMinFill:   texto(planconfig.KeyMinFill, decimal(planconfig.DefaultMinFill)),
		PlanMaxOver:   texto(planconfig.KeyMaxOvershoot, decimal(planconfig.DefaultMaxOvershoot)),
		PlanRecap:     planRecapDosValores(valores),
	}
}

// planRecapDosValores mostra o checkbox de recap, aplicando o padrão quando a
// chave está ausente ou ilegível.
func planRecapDosValores(valores map[string]string) bool {
	bruto := strings.TrimSpace(valores[planconfig.KeyRecap])
	if bruto == "" {
		return planconfig.DefaultRecap
	}
	v, err := strconv.ParseBool(bruto)
	if err != nil {
		return planconfig.DefaultRecap
	}
	return v
}

func decimal(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// configDoFormulario devolve os campos para re-render e as chaves para
// validação/gravação. Token em branco mantém o atual; o checkbox de remoção
// grava vazio.
func configDoFormulario(r *http.Request) (views.ConfigProps, map[string]string) {
	dias := make([]string, 0, 7)
	for _, dia := range r.PostForm["dias"] {
		if dia = strings.TrimSpace(dia); dia != "" {
			dias = append(dias, dia)
		}
	}

	props := views.ConfigProps{
		Enabled:     strings.TrimSpace(r.PostFormValue("agenda_ligada")) != "",
		Dias:        dias,
		Horario:     strings.TrimSpace(r.PostFormValue("horario")),
		Fuso:        strings.TrimSpace(r.PostFormValue("fuso")),
		Catchup:     strings.TrimSpace(r.PostFormValue("catchup")),
		MaxDocs:     strings.TrimSpace(r.PostFormValue("max_docs")),
		NtfyURL:     strings.TrimSpace(r.PostFormValue("ntfy_url")),
		NtfyTopico:  strings.TrimSpace(r.PostFormValue("ntfy_topico")),
		NtfyUsuario: strings.TrimSpace(r.PostFormValue("ntfy_usuario")),
		Prioridade:  strings.TrimSpace(r.PostFormValue("ntfy_prioridade")),
		ModoCorpo:   strings.TrimSpace(r.PostFormValue("modo_corpo")),
		MaxBytes:    strings.TrimSpace(r.PostFormValue("max_bytes")),
		PlanWPM:     strings.TrimSpace(r.PostFormValue("plan_wpm")),
		PlanDiff:    strings.TrimSpace(r.PostFormValue("plan_dificuldade")),
		PlanMinFill: strings.TrimSpace(r.PostFormValue("plan_min_fill")),
		PlanMaxOver: strings.TrimSpace(r.PostFormValue("plan_max_overshoot")),
		PlanRecap:   strings.TrimSpace(r.PostFormValue("plan_recap")) != "",
	}

	valores := map[string]string{
		schedule.KeyEnabled:        strconv.FormatBool(props.Enabled),
		schedule.KeyDays:           strings.Join(props.Dias, ","),
		schedule.KeyTime:           props.Horario,
		schedule.KeyTimezone:       props.Fuso,
		schedule.KeyCatchupDays:    props.Catchup,
		schedule.KeyMaxDocsPerRun:  props.MaxDocs,
		schedule.KeyNtfyBaseURL:    props.NtfyURL,
		schedule.KeyNtfyTopic:      props.NtfyTopico,
		schedule.KeyNtfyUsername:   props.NtfyUsuario,
		schedule.KeyNtfyPriority:   props.Prioridade,
		schedule.KeyNtfyBodyMode:   props.ModoCorpo,
		schedule.KeyNtfyMaxBytes:   props.MaxBytes,
		planconfig.KeyWPM:          props.PlanWPM,
		planconfig.KeyDifficulty:   props.PlanDiff,
		planconfig.KeyMinFill:      props.PlanMinFill,
		planconfig.KeyMaxOvershoot: props.PlanMaxOver,
		planconfig.KeyRecap:        strconv.FormatBool(props.PlanRecap),
	}

	token := strings.TrimSpace(r.PostFormValue("ntfy_token"))
	switch {
	case strings.TrimSpace(r.PostFormValue("ntfy_remover_token")) != "":
		valores[schedule.KeyNtfyToken] = ""
		props.TokenDefinido = false
	case token != "":
		valores[schedule.KeyNtfyToken] = token
		props.TokenDefinido = true
	}

	senha := r.PostFormValue("ntfy_senha")
	switch {
	case props.NtfyUsuario == "":
		// Basic exige usuário; apagar a senha junto evita configuração órfã.
		valores[schedule.KeyNtfyPassword] = ""
		props.SenhaDefinida = false
	case senha != "":
		valores[schedule.KeyNtfyPassword] = senha
		props.SenhaDefinida = true
	}
	return props, valores
}

// idsDosDias normaliza "mon, wed" para ["mon","wed"], preservando ids
// desconhecidos para que o formulário mostre o valor salvo e a validação
// explique o erro.
func idsDosDias(bruto string) []string {
	out := make([]string, 0, 7)
	for _, parte := range strings.Split(bruto, ",") {
		if id := strings.ToLower(strings.TrimSpace(parte)); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func tokenDefinido(valores map[string]string) bool {
	return strings.TrimSpace(valores[schedule.KeyNtfyToken]) != ""
}

func senhaDefinida(valores map[string]string) bool {
	return valores[schedule.KeyNtfyPassword] != ""
}

// testeDaQuery monta o resultado do envio de teste que voltou pelo redirect.
func testeDaQuery(r *http.Request) *views.TesteProps {
	q := r.URL.Query()
	switch q.Get("teste") {
	case "":
		return nil
	case "ok":
		return &views.TesteProps{
			OK:     true,
			Status: atoiSeguro(q.Get("http")),
			MS:     atoiSeguro(q.Get("ms")),
			Bytes:  atoiSeguro(q.Get("bytes")),
		}
	default:
		return &views.TesteProps{Motivo: truncarMotivo(q.Get("motivo"), motivoMax)}
	}
}

// direcionarTeste volta para /settings com o resultado do envio de teste.
func direcionarTeste(w http.ResponseWriter, r *http.Request, envio delivery.SendResult, err error) {
	q := url.Values{}
	if err != nil {
		motivo := "falha ao enviar: " + truncarMotivo(err.Error(), motivoMax)
		if errors.Is(err, schedule.ErrNtfyNaoConfigurado) {
			motivo = "configure a URL e o tópico do ntfy antes de testar"
		} else if errors.Is(err, context.DeadlineExceeded) {
			motivo = "o ntfy não respondeu a tempo"
		}
		q.Set("teste", "erro")
		q.Set("motivo", motivo)
	} else {
		q.Set("teste", "ok")
		q.Set("http", strconv.Itoa(envio.Status))
		q.Set("ms", strconv.FormatInt(envio.Latency.Milliseconds(), 10))
		q.Set("bytes", strconv.Itoa(envio.Bytes))
	}
	http.Redirect(w, r, "/settings?"+q.Encode(), http.StatusSeeOther)
}

// avisoDoEnvio traduz o erro do envio manual em um código estável para a URL.
func avisoDoEnvio(err error) string {
	switch {
	case errors.Is(err, schedule.ErrSemTrechos):
		return "sem-trechos"
	case errors.Is(err, schedule.ErrJaEnviadoHoje):
		return "ja-enviado"
	case errors.Is(err, schedule.ErrNtfyNaoConfigurado):
		return "ntfy"
	case errors.Is(err, schedule.ErrEnvioEmAndamento):
		return "ocupado"
	case errors.Is(err, schedule.ErrDocumentoNaoPronto), errors.Is(err, schedule.ErrDocumentoNaoEncontrado):
		return "indisponivel"
	case errors.Is(err, context.DeadlineExceeded):
		return "tempo"
	default:
		return "falha"
	}
}

// motivoDoEnvio só devolve texto livre para falhas de publicação; os demais
// códigos têm mensagem fixa na view.
func motivoDoEnvio(err error) string {
	if avisoDoEnvio(err) != "falha" {
		return ""
	}
	return truncarMotivo(err.Error(), motivoMax)
}

func destinoEnvio(id int64, chave, valor, motivo string) string {
	caminho := "/documents/" + strconv.FormatInt(id, 10)
	q := url.Values{}
	if valor != "" {
		q.Set(chave, valor)
	}
	if motivo != "" {
		q.Set("motivo", motivo)
	}
	if len(q) == 0 {
		return caminho
	}
	return caminho + "?" + q.Encode()
}

func atoiSeguro(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	if n > 1_000_000_000 {
		return 1_000_000_000
	}
	return n
}

func truncarMotivo(s string, limite int) string {
	s = strings.TrimSpace(s)
	if limite <= 0 || len(s) <= limite {
		return s
	}
	corte := limite
	for corte > 0 && !utf8.RuneStart(s[corte]) {
		corte--
	}
	return s[:corte] + "…"
}
