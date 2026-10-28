package web

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/giovani/readingdrip/internal/store"
	"github.com/giovani/readingdrip/internal/web/views"
)

// handleBiblioteca lista os documentos e o formulário de upload.
func (s *Server) handleBiblioteca(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}
	docs, err := s.Store.Documents().List(r.Context())
	if err != nil {
		s.logger().Error("listar documentos", "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar a biblioteca")
		return
	}
	s.render(w, r, http.StatusOK, views.Biblioteca(views.BibliotecaProps{
		Documentos: docs,
		Importados: queryInt(r, "importados"),
		Duplicados: queryInt(r, "duplicados"),
		Excluidos:  queryInt(r, "excluido"),
		Erro:       strings.TrimSpace(r.URL.Query().Get("erro")),
	}))
}

// handleDocumento mostra status, metadados e o plano ativo.
func (s *Server) handleDocumento(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return
	}
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}

	doc, err := s.Store.Documents().Get(r.Context(), id)
	if err != nil {
		s.logger().Error("buscar documento", "id", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o documento")
		return
	}
	if doc == nil {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return
	}

	plano, err := s.Store.Plans().Active(r.Context(), id)
	if err != nil {
		s.logger().Error("buscar plano ativo", "id", id, "erro", err)
		plano = nil
	}

	_, temOriginal := s.caminhoOriginal(doc.SourcePath)
	aviso, falha := mensagensDoEnvio(r)
	s.render(w, r, http.StatusOK, views.Detalhe(views.DetalheProps{
		Doc:         *doc,
		Plano:       plano,
		Avisos:      avisosDoPlano(plano),
		TemOriginal: temOriginal,
		Duplicado:   queryBool(r, "duplicado"),
		Aviso:       aviso,
		Falha:       falha,
	}))
}

// handlePlano mostra o plano ativo em uma página própria, com a mesma tabela
// da prévia do detalhe.
func (s *Server) handlePlano(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return
	}
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}

	doc, err := s.Store.Documents().Get(r.Context(), id)
	if err != nil {
		s.logger().Error("buscar documento para o plano", "id", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o plano")
		return
	}
	if doc == nil {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return
	}

	plano, err := s.Store.Plans().Active(r.Context(), id)
	if err != nil {
		s.logger().Error("buscar plano ativo", "id", id, "erro", err)
		plano = nil
	}
	s.render(w, r, http.StatusOK, views.Plano(views.DetalheProps{
		Doc:    *doc,
		Plano:  plano,
		Avisos: avisosDoPlano(plano),
	}))
}

// avisosDoPlano deriva os avisos de pacing do plano ativo (RF-026). O store
// guarda alvo e parâmetros; os chunks dizem quantos dias de leitura existem.
func avisosDoPlano(plano *store.Plan) []string {
	if plano == nil {
		return nil
	}
	return plano.Params.Warnings(plano.TargetSeconds, len(plano.Chunks))
}

// handleDownload entrega o arquivo original como anexo, apenas se o caminho
// estiver dentro de {data}/documents.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return
	}
	if s.Store == nil || s.Importer == nil {
		http.Error(w, "download indisponível", http.StatusServiceUnavailable)
		return
	}

	doc, err := s.Store.Documents().Get(r.Context(), id)
	if err != nil {
		s.logger().Error("buscar documento para download", "id", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível baixar o arquivo")
		return
	}
	if doc == nil {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return
	}

	caminho, ok := s.caminhoOriginal(doc.SourcePath)
	if !ok {
		s.renderErro(w, r, http.StatusNotFound, "Arquivo original não encontrado")
		return
	}
	arquivo, err := os.Open(caminho)
	if err != nil {
		s.logger().Error("abrir original", "id", id, "erro", err)
		s.renderErro(w, r, http.StatusNotFound, "Arquivo original não encontrado")
		return
	}
	defer arquivo.Close()

	info, err := arquivo.Stat()
	if err != nil || info.IsDir() {
		s.logger().Error("inspecionar original", "id", id, "erro", err)
		s.renderErro(w, r, http.StatusNotFound, "Arquivo original não encontrado")
		return
	}

	nome := doc.SourceName
	if nome == "" {
		nome = "original" + strings.ToLower(filepath.Ext(caminho))
	}
	disposicao := mime.FormatMediaType("attachment", map[string]string{"filename": nome})
	if disposicao == "" {
		disposicao = "attachment"
	}
	w.Header().Set("Content-Type", tipoDoOriginal(filepath.Ext(caminho)))
	w.Header().Set("Content-Disposition", disposicao)
	http.ServeContent(w, r, nome, info.ModTime(), arquivo)
}

// tipoDoOriginal fixa o Content-Type dos formatos aceitos: mime.TypeByExtension
// depende do registro do sistema operacional (no Windows .md vira text/plain).
func tipoDoOriginal(ext string) string {
	switch strings.ToLower(ext) {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".epub":
		return "application/epub+zip"
	case ".pdf":
		return "application/pdf"
	}
	if tipo := mime.TypeByExtension(strings.ToLower(ext)); tipo != "" {
		return tipo
	}
	return "application/octet-stream"
}

// caminhoOriginal valida que o caminho gravado no banco continua dentro do
// diretório de documentos do importador.
func (s *Server) caminhoOriginal(caminho string) (string, bool) {
	if s.Importer == nil || s.Importer.Root == "" || caminho == "" {
		return "", false
	}
	raiz, err := filepath.Abs(filepath.Join(s.Importer.Root, "documents"))
	if err != nil {
		return "", false
	}
	alvo, err := filepath.Abs(caminho)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(raiz, alvo)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	info, err := os.Stat(alvo)
	if err != nil || info.IsDir() {
		return "", false
	}
	return alvo, true
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, componente templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := componente.Render(r.Context(), w); err != nil {
		s.logger().Error("renderizar página", "erro", err)
	}
}

func (s *Server) renderErro(w http.ResponseWriter, r *http.Request, status int, mensagem string) {
	s.render(w, r, status, views.ErroPagina(views.ErroProps{Codigo: status, Mensagem: mensagem}))
}

func idDaRota(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func queryInt(r *http.Request, chave string) int {
	n, err := strconv.Atoi(r.URL.Query().Get(chave))
	if err != nil || n < 0 {
		return 0
	}
	if n > 1_000_000 {
		return 1_000_000
	}
	return n
}

func queryBool(r *http.Request, chave string) bool {
	v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(chave)))
	return v == "1" || v == "true" || v == "sim"
}

// mensagensDoEnvio traduz o resultado do envio manual e das ações de gestão
// (query do redirect) em texto para o detalhe. Códigos fixos; só `falha` mostra
// o motivo livre.
func mensagensDoEnvio(r *http.Request) (aviso, falha string) {
	q := r.URL.Query()
	if queryBool(r, "enviado") {
		aviso = "Trecho enviado para o ntfy."
	}
	switch q.Get("acao") {
	case "pausado":
		aviso = "Documento pausado: ele não entra nas próximas entregas até você retomar."
	case "retomado":
		aviso = "Documento retomado: a leitura continua do trecho pendente."
	case "reprocessando":
		aviso = "Documento reenfileirado: a extração recomeça em segundo plano."
	}
	switch q.Get("aviso") {
	case "sem-trechos":
		aviso = "Não há trechos pendentes neste documento."
	case "ja-enviado":
		aviso = "A run de hoje já entregou este documento; o próximo trecho sai na próxima run."
	case "ntfy":
		aviso = "Configure a URL e o tópico do ntfy em Configurações antes de enviar."
	case "ocupado":
		aviso = "Uma entrega está em andamento. Tente de novo em instantes."
	case "indisponivel":
		aviso = "O documento precisa estar pronto para ser enviado."
	case "processando":
		aviso = "A extração deste documento ainda está em andamento."
	case "pausado":
		aviso = "Retome o documento antes de reprocessar."
	case "historico":
		aviso = "Este documento já tem trechos entregues e não pode ser reextraído. Exclua e envie o arquivo de novo para gerar outro plano."
	case "tempo":
		falha = "O ntfy não respondeu a tempo."
	case "falha":
		if motivo := truncarMotivo(q.Get("motivo"), motivoMax); motivo != "" {
			falha = "Não foi possível enviar: " + motivo
		} else {
			falha = "Não foi possível enviar o trecho."
		}
	}
	return aviso, falha
}
