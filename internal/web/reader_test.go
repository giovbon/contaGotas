package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/links"
	"github.com/giovani/readingdrip/internal/store"
)

const segredoTeste = "chave-de-teste-com-32-bytes-ok!!"

// assinadorDeTeste cria um signer apontado para o servidor de teste e o injeta
// no servidor. O relógio pode ser movido para exercitar a expiração.
func assinadorDeTeste(t *testing.T, a *ambiente, agora *time.Time) *links.Signer {
	t.Helper()
	s, err := links.New(links.Config{
		Secret:  []byte(segredoTeste),
		BaseURL: a.http.URL,
		Now:     func() time.Time { return *agora },
	})
	if err != nil {
		t.Fatalf("criar assinador: %v", err)
	}
	a.srv.Links = s
	return s
}

// caminhoDaURL devolve só o caminho e a query de uma URL absoluta, para usar
// nos helpers de requisição.
func caminhoDaURL(t *testing.T, bruta string) string {
	t.Helper()
	u, err := url.Parse(bruta)
	if err != nil {
		t.Fatalf("parsear %q: %v", bruta, err)
	}
	return u.RequestURI()
}

// documentoLongo importa e processa um texto com vários trechos e devolve o
// documento e os chunks do plano ativo em ordem.
func documentoLongo(t *testing.T, a *ambiente) (*store.Document, []store.Chunk) {
	t.Helper()
	corpo := "# Um\n\n" + strings.Repeat("Palavra de enchimento para o texto longo. ", 900) +
		"\n\n# Dois\n\n" + strings.Repeat("Outra frase de enchimento qualquer. ", 900)
	doc := importarDoc(t, a, "Longo.md", corpo)
	processar(t, a, doc.ID)

	plano, err := a.store.Plans().Active(context.Background(), doc.ID)
	if err != nil || plano == nil {
		t.Fatalf("plano ativo = %+v, %v", plano, err)
	}
	if len(plano.Chunks) < 3 {
		t.Fatalf("plano com %d chunk(s), quer pelo menos 3", len(plano.Chunks))
	}
	return doc, plano.Chunks
}

func TestLeitorSemAssinador(t *testing.T) {
	a, _ := ambienteComLogin(t)
	_, chunks := documentoLongo(t, a)
	alvo := chunks[0].ID

	resp := getComCookies(t, a, "/r/"+itoa(alvo))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("leitor: status = %d, quer 503", resp.StatusCode)
	}
	resp = postForm(t, a, "/api/actions/chunks/"+itoa(alvo)+"/done", url.Values{})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("marcar lido: status = %d, quer 503", resp.StatusCode)
	}
}

func TestLeitorTokenInvalido(t *testing.T) {
	a, _ := ambienteComLogin(t)
	_, chunks := documentoLongo(t, a)
	alvo := chunks[0]

	relogio := agora
	s := assinadorDeTeste(t, a, &relogio)
	expirado := s.Token(links.ScopeRead, alvo.ID, time.Hour)
	semEscopo := s.Token(links.ScopeDone, alvo.ID, time.Hour)

	casos := []struct {
		nome  string
		query string
	}{
		{"sem token", ""},
		{"token vazio", "?t="},
		{"token lixo", "?t=abc.def"},
		{"token de outro escopo", "?t=" + semEscopo},
		{"token adulterado", "?t=" + s.Token(links.ScopeRead, alvo.ID+1, time.Hour)},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			resp := getComCookies(t, a, "/r/"+itoa(alvo.ID)+caso.query)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, quer 401", resp.StatusCode)
			}
			corpo := lerCorpo(t, resp)
			if !strings.Contains(corpo, "Este link expirou ou é inválido") {
				t.Error("página de link inválido sem a mensagem esperada")
			}
			if strings.Contains(corpo, "Palavra de enchimento") {
				t.Error("página inválida vazou o corpo do trecho")
			}
		})
	}

	// Expirado depois da criação: mesma resposta 401.
	relogio = agora.Add(2 * time.Hour)
	resp := getComCookies(t, a, "/r/"+itoa(alvo.ID)+"?t="+expirado)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("token expirado: status = %d, quer 401", resp.StatusCode)
	}

	// A ação “marcar lido” exige o escopo próprio.
	leitura := s.Token(links.ScopeRead, alvo.ID, time.Hour)
	resp = postForm(t, a, "/api/actions/chunks/"+itoa(alvo.ID)+"/done?t="+leitura, url.Values{})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("ação com token do leitor: status = %d, quer 401", resp.StatusCode)
	}
}

func TestLeitorChunkInexistente(t *testing.T) {
	a, _ := ambienteComLogin(t)
	relogio := agora
	s := assinadorDeTeste(t, a, &relogio)

	const fantasma = 987654
	resp := getComCookies(t, a, "/r/"+itoa(fantasma)+"?t="+s.Token(links.ScopeRead, fantasma, 0))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, quer 404", resp.StatusCode)
	}
	resp = postForm(t, a, "/api/actions/chunks/"+itoa(fantasma)+"/done?t="+s.Token(links.ScopeDone, fantasma, 0), url.Values{})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("ação: status = %d, quer 404", resp.StatusCode)
	}
}

func TestLeitorAbreTrechoComNavegacao(t *testing.T) {
	a, _ := ambienteComLogin(t)
	doc, chunks := documentoLongo(t, a)
	relogio := agora
	s := assinadorDeTeste(t, a, &relogio)

	meio := chunks[1]
	caminho := caminhoDaURL(t, s.ReadURL(meio.ID, 0))
	resp := getComCookies(t, a, caminho)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	corpo := lerCorpo(t, resp)

	for _, trecho := range []string{
		`href="/static/app.css"`,
		`noindex, nofollow`,
		`class="leitura-corpo"`,
		"Palavra de enchimento",
		`href="/documents/` + itoa(doc.ID) + `"`,
		"Anterior",
		"Próximo",
		"% do livro",
		"/api/actions/chunks/" + itoa(meio.ID) + "/done?t=",
		"ui=1",
		"Marcar como lido",
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página do leitor sem %q", trecho)
		}
	}

	// Os vizinhos linkam com token próprio do escopo de leitura.
	for _, vizinho := range []store.Chunk{chunks[0], chunks[2]} {
		if !strings.Contains(corpo, "/r/"+itoa(vizinho.ID)+"?t=") {
			t.Errorf("página sem link para o chunk %d", vizinho.ID)
		}
	}

	// Toda URL de leitor na página passa pelo token logo após o id.
	for _, parte := range strings.Split(corpo, "/r/")[1:] {
		if i := strings.Index(parte, "?t="); i < 0 || i > 8 {
			t.Errorf("link de leitor sem token: %.40q", parte)
		}
	}

	// A seção completa só aparece quando o trecho é um recorte dela.
	temSecao := strings.Contains(corpo, "Ver seção completa")
	if quer := meio.StartsMidSection || meio.EndsMidSection; quer != temSecao {
		t.Errorf("seção completa = %v, corte no meio = %v/%v", temSecao, meio.StartsMidSection, meio.EndsMidSection)
	}
	if temSecao && !strings.Contains(corpo, "<details") {
		t.Error("seção completa fora de <details>")
	}

	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
}

func TestLeitorPrimeiroTrechoSemAnterior(t *testing.T) {
	a, _ := ambienteComLogin(t)
	_, chunks := documentoLongo(t, a)
	relogio := agora
	s := assinadorDeTeste(t, a, &relogio)

	corpo := lerCorpo(t, getComCookies(t, a, caminhoDaURL(t, s.ReadURL(chunks[0].ID, 0))))
	if strings.Contains(corpo, "← Anterior</a>") {
		t.Error("primeiro trecho não deveria linkar um anterior")
	}
	if !strings.Contains(corpo, "Próximo →") {
		t.Error("primeiro trecho deveria linkar o próximo")
	}
}

func TestMarcarLidoPeloNtfy(t *testing.T) {
	a, _ := ambienteComLogin(t)
	_, chunks := documentoLongo(t, a)
	relogio := agora
	s := assinadorDeTeste(t, a, &relogio)
	alvo := chunks[0].ID

	acao := "/api/actions/chunks/" + itoa(alvo) + "/done?t=" + s.Token(links.ScopeDone, alvo, 0)
	resp := postForm(t, a, acao, url.Values{})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, quer 204", resp.StatusCode)
	}
	lido := chunkDoBanco(t, a, alvo)
	if lido.DoneAt == nil {
		t.Fatal("chunk não foi marcado como lido")
	}

	// Repetir a ação responde igual e mantém o estado.
	resp = postForm(t, a, acao, url.Values{})
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("repetição: status = %d, quer 204", resp.StatusCode)
	}
	if depois := chunkDoBanco(t, a, alvo); depois.DoneAt == nil || !depois.DoneAt.Equal(*lido.DoneAt) {
		t.Error("repetição perdeu o estado de lido")
	}
}

func TestMarcarLidoPeloFormulario(t *testing.T) {
	a, _ := ambienteComLogin(t)
	_, chunks := documentoLongo(t, a)
	relogio := agora
	s := assinadorDeTeste(t, a, &relogio)
	alvo := chunks[0].ID

	token := s.Token(links.ScopeDone, alvo, 0)
	resp := postForm(t, a, "/api/actions/chunks/"+itoa(alvo)+"/done?t="+token+"&ui=1", url.Values{})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	destino := resp.Header.Get("Location")
	if !strings.HasPrefix(destino, "/r/"+itoa(alvo)+"?t=") || !strings.Contains(destino, "lido=1") {
		t.Fatalf("Location = %q", destino)
	}
	novo := destino[strings.Index(destino, "t=")+2:]
	if i := strings.IndexByte(novo, '&'); i >= 0 {
		novo = novo[:i]
	}
	if err := s.Verify(links.ScopeRead, alvo, novo); err != nil {
		t.Errorf("redirect sem token de leitura válido: %v", err)
	}

	pagina := lerCorpo(t, getComCookies(t, a, caminhoDaURL(t, destino)))
	if !strings.Contains(pagina, "Trecho marcado como lido.") {
		t.Error("página após marcar não confirmou a leitura")
	}
	if !strings.Contains(pagina, "Lido em ") {
		t.Error("página após marcar não mostrou a data")
	}
	if strings.Contains(pagina, "Marcar como lido</button>") {
		t.Error("botão de marcar continuou disponível")
	}
}

func TestMarcarLidoAceitaNavegador(t *testing.T) {
	a, _ := ambienteComLogin(t)
	_, chunks := documentoLongo(t, a)
	relogio := agora
	s := assinadorDeTeste(t, a, &relogio)
	alvo := chunks[1].ID

	req, err := http.NewRequest(http.MethodPost,
		a.http.URL+"/api/actions/chunks/"+itoa(alvo)+"/done?t="+s.Token(links.ScopeDone, alvo, 0),
		strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := semRedirect.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, quer 303", resp.StatusCode)
	}
}

// chunkDoBanco relê o chunk para conferir o que ficou gravado.
func chunkDoBanco(t *testing.T, a *ambiente, id int64) *store.Chunk {
	t.Helper()
	chunk, err := a.store.Plans().Chunk(context.Background(), id)
	if err != nil || chunk == nil {
		t.Fatalf("chunk %d = %+v, %v", id, chunk, err)
	}
	return chunk
}
