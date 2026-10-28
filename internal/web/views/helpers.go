package views

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/giovani/readingdrip/internal/store"
)

func urlDocumento(id int64) string { return "/documents/" + strconv.FormatInt(id, 10) }

func urlDownload(id int64) string { return urlDocumento(id) + "/download" }

func urlPlano(id int64) string { return urlDocumento(id) + "/plan" }

func urlEnviarAgora(id int64) string { return urlDocumento(id) + "/send-now" }

// DiaDaSemana é um item do seletor de dias da agenda.
type DiaDaSemana struct {
	ID     string
	Rotulo string
}

// DiasDaSemana é a ordem fixa do formulário de configurações.
func DiasDaSemana() []DiaDaSemana {
	return []DiaDaSemana{
		{"mon", "seg"},
		{"tue", "ter"},
		{"wed", "qua"},
		{"thu", "qui"},
		{"fri", "sex"},
		{"sat", "sáb"},
		{"sun", "dom"},
	}
}

func diaSelecionado(dias []string, id string) bool {
	for _, d := range dias {
		if strings.EqualFold(strings.TrimSpace(d), id) {
			return true
		}
	}
	return false
}

func prioridadeSelecionada(prioridade, valor string) bool {
	return strings.TrimSpace(prioridade) == valor
}

func modoSelecionado(modo, valor string) bool {
	return strings.EqualFold(strings.TrimSpace(modo), valor)
}

func numero(n int) string { return strconv.Itoa(n) }

func rotuloFormato(formato string) string {
	if formato == "" {
		return "—"
	}
	return strings.ToUpper(formato)
}

func rotuloStatus(status store.DocumentStatus) string {
	switch status {
	case store.StatusProcessing:
		return "Processando"
	case store.StatusReady:
		return "Pronto"
	case store.StatusPaused:
		return "Pausado"
	case store.StatusCompleted:
		return "Concluído"
	case store.StatusFailed:
		return "Falhou"
	case store.StatusNeedsOCR:
		return "Precisa OCR"
	default:
		if status == "" {
			return "—"
		}
		return string(status)
	}
}

func rotuloChunk(status store.ChunkStatus) string {
	switch status {
	case store.ChunkPending:
		return "Pendente"
	case store.ChunkSent:
		return "Enviado"
	case store.ChunkSkipped:
		return "Pulado"
	default:
		if status == "" {
			return "—"
		}
		return string(status)
	}
}

func formatarPalavras(n int) string {
	if n == 1 {
		return "1 palavra"
	}
	return strconv.Itoa(n) + " palavras"
}

func formatarDuracao(segundos int) string {
	if segundos <= 0 {
		return "—"
	}
	if segundos < 60 {
		return strconv.Itoa(segundos) + " s"
	}
	minutos := float64(segundos) / 60
	if minutos < 10 {
		return fmt.Sprintf("%.1f min", minutos)
	}
	return fmt.Sprintf("%.0f min", minutos)
}

func formatarData(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("02/01/2006 15:04")
}

func autor(d store.Document) string {
	if d.Author == nil {
		return ""
	}
	return strings.TrimSpace(*d.Author)
}

func subtituloDocumento(d store.Document) string {
	partes := []string{rotuloFormato(d.Format)}
	if a := autor(d); a != "" {
		partes = append(partes, a)
	}
	if d.TotalWords > 0 {
		partes = append(partes, formatarPalavras(d.TotalWords))
	}
	if d.TotalSeconds > 0 {
		partes = append(partes, formatarDuracao(d.TotalSeconds))
	}
	return strings.Join(partes, " · ")
}

func mensagemErro(d store.Document) string {
	if d.Error != nil && strings.TrimSpace(*d.Error) != "" {
		return strings.TrimSpace(*d.Error)
	}
	switch d.Status {
	case store.StatusFailed:
		return "A extração falhou. Reprocesse ou envie o arquivo novamente."
	case store.StatusNeedsOCR:
		return "O PDF não tem camada de texto. Converta com OCR e envie novamente."
	default:
		return ""
	}
}

func estaProcessando(d store.Document) bool { return d.Status == store.StatusProcessing }

// podeEnviarAgora exibe o botão de envio manual só quando há plano ativo e o
// documento está pronto.
func podeEnviarAgora(d store.Document, plano *store.Plan) bool {
	return plano != nil && d.Status == store.StatusReady
}

func precisaOCR(d store.Document) bool { return d.Status == store.StatusNeedsOCR }

func textoSecao(caminho []string) string {
	limpo := make([]string, 0, len(caminho))
	for _, parte := range caminho {
		if t := strings.TrimSpace(parte); t != "" {
			limpo = append(limpo, t)
		}
	}
	if len(limpo) == 0 {
		return "—"
	}
	return strings.Join(limpo, " › ")
}

func pluralArquivos(n int) string {
	if n == 1 {
		return "1 arquivo importado."
	}
	return strconv.Itoa(n) + " arquivos importados."
}

func pluralDuplicados(n int) string {
	if n == 1 {
		return "1 duplicado ignorado."
	}
	return strconv.Itoa(n) + " duplicados ignorados."
}
