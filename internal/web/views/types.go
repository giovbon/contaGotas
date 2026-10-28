package views

import (
	"time"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/store"
)

// LoginProps são os dados do formulário de entrada.
type LoginProps struct {
	Next string
	Erro string
}

// BibliotecaProps são os dados da lista de documentos.
type BibliotecaProps struct {
	Documentos []store.Document
	Importados int
	Duplicados int
	Erro       string
}

// DetalheProps são os dados da página de um documento.
type DetalheProps struct {
	Doc         store.Document
	Plano       *store.Plan
	TemOriginal bool
	Duplicado   bool
	Aviso       string
	Falha       string
}

// TesteProps é o resultado do envio de teste do ntfy.
type TesteProps struct {
	OK     bool
	Status int
	MS     int
	Bytes  int
	Motivo string
}

// ConfigProps são os dados de /settings.
type ConfigProps struct {
	Enabled       bool
	Dias          []string
	Horario       string
	Fuso          string
	Catchup       string
	MaxDocs       string
	NtfyURL       string
	NtfyTopico    string
	TokenDefinido bool
	Prioridade    string
	ModoCorpo     string
	MaxBytes      string
	Salvo         bool
	Erro          string
	Teste         *TesteProps
}

// LeitorVizinho é um chunk anterior ou seguinte do mesmo plano.
type LeitorVizinho struct {
	URL string
	Seq int
}

// LeitorProps são os dados da página de leitura de um chunk.
type LeitorProps struct {
	Doc          store.Document
	Chunk        store.Chunk
	Corpo        []blocks.Block
	Secao        []blocks.Block
	MostrarSecao bool
	Anterior     *LeitorVizinho
	Proximo      *LeitorVizinho
	Percent      int
	Acao         string // POST de “marcar como lido” (com token)
	Voltar       string
	LidoEm       *time.Time
	MarcadoAgora bool
}

// ErroProps são os dados da página de erro.
type ErroProps struct {
	Codigo   int
	Mensagem string
}
