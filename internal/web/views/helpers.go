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

func urlPausar(id int64) string { return urlDocumento(id) + "/pause" }

func urlRetomar(id int64) string { return urlDocumento(id) + "/resume" }

func urlReprocessar(id int64) string { return urlDocumento(id) + "/reprocess" }

func urlExcluir(id int64) string { return urlDocumento(id) + "/delete" }

func urlHistorico(filtro int64) string {
	if filtro <= 0 {
		return "/history"
	}
	return "/history?documento=" + strconv.FormatInt(filtro, 10)
}

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

func numero64(n int64) string { return strconv.FormatInt(n, 10) }

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

func estaPausado(d store.Document) bool { return d.Status == store.StatusPaused }

// podeEnviarAgora exibe o botão de envio manual só quando há plano ativo e o
// documento está pronto.
func podeEnviarAgora(d store.Document, plano *store.Plan) bool {
	return plano != nil && d.Status == store.StatusReady
}

// podePausar/podeRetomar/podeReprocessar decidem quais botões de gestão
// aparecem no detalhe. `failed` e `needs_ocr` só podem ser reprocessados.
func podePausar(d store.Document) bool { return d.Status == store.StatusReady }

func podeRetomar(d store.Document) bool { return d.Status == store.StatusPaused }

func podeReprocessar(d store.Document) bool {
	switch d.Status {
	case store.StatusReady, store.StatusFailed, store.StatusNeedsOCR:
		return true
	default:
		return false
	}
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

func pluralExcluidos(n int) string {
	if n == 1 {
		return "1 documento excluído."
	}
	return strconv.Itoa(n) + " documentos excluídos."
}

func rotuloRun(status store.RunStatus) string {
	switch status {
	case store.RunRunning:
		return "Em andamento"
	case store.RunSent:
		return "Enviada"
	case store.RunPartial:
		return "Parcial"
	case store.RunFailed:
		return "Falhou"
	default:
		if status == "" {
			return "—"
		}
		return string(status)
	}
}

func rotuloItem(status store.ItemStatus) string {
	switch status {
	case store.ItemPending:
		return "Pendente"
	case store.ItemSent:
		return "Enviado"
	case store.ItemFailed:
		return "Falhou"
	case store.ItemSkipped:
		return "Pulado"
	default:
		if status == "" {
			return "—"
		}
		return string(status)
	}
}

func formatarBytes(n int) string {
	switch {
	case n <= 0:
		return "—"
	case n < 1024:
		return strconv.Itoa(n) + " B"
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	}
}

func formatarLatencia(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	if d < time.Second {
		return strconv.FormatInt(d.Milliseconds(), 10) + " ms"
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
	return fmt.Sprintf("%.0f min", d.Minutes())
}

func formatarHora(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "—"
	}
	return t.Format("02/01/2006 15:04")
}

func tituloDoc(d store.Document) string {
	if t := strings.TrimSpace(d.Title); t != "" {
		return t
	}
	return "Documento " + strconv.FormatInt(d.ID, 10)
}

func docSelecionado(filtro, id int64) bool { return filtro > 0 && filtro == id }

// bytesDoItem trata o ponteiro opcional de delivery_items.bytes.
func bytesDoItem(item store.DeliveryItem) int {
	if item.Bytes == nil {
		return 0
	}
	return *item.Bytes
}

func erroDoItem(item store.DeliveryItem) string {
	if item.Error == nil {
		return "—"
	}
	if msg := strings.TrimSpace(*item.Error); msg != "" {
		return msg
	}
	return "—"
}

func erroDaRun(run store.DeliveryRun) string {
	if run.Error == nil {
		return ""
	}
	return strings.TrimSpace(*run.Error)
}

func pluralItens(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}
