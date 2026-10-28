package plan

import (
	"errors"
	"fmt"

	"github.com/giovani/readingdrip/internal/blocks"
)

// Mode define como o alvo diário é calculado.
type Mode string

const (
	ModeMinutesPerDay Mode = "minutes_per_day"
	ModeFinishByDays  Mode = "finish_by_days"
)

// Params são os parâmetros de geração do plano.
type Params struct {
	Mode             Mode
	Minutes          int     // modo minutes_per_day
	Days             int     // modo finish_by_days (dias de leitura)
	MinFill          float64 // fração mínima do alvo
	MaxOvershoot     float64 // fração máxima do alvo
	WPM              int     // usado apenas para documentação do plano
	Difficulty       float64 // idem
	SectionAlignment bool    // preferência por fim de seção (o empacotamento já é por nó)
}

// Chunk é uma dose diária: intervalo contíguo de blocos.
type Chunk struct {
	Seq              int      `json:"seq"`
	FirstBlock       int      `json:"first_block"`
	LastBlock        int      `json:"last_block"`
	FirstHash        string   `json:"first_hash"`
	LastHash         string   `json:"last_hash"`
	SectionPath      []string `json:"section_path"`
	StartsMidSection bool     `json:"starts_mid_section"`
	EndsMidSection   bool     `json:"ends_mid_section"`
	Oversized        bool     `json:"oversized"`
	Words            int      `json:"words"`
	Seconds          int      `json:"seconds"`
	TargetSeconds    int      `json:"target_seconds"`
}

// Plan é o resultado completo da segmentação.
type Plan struct {
	Mode          Mode    `json:"mode"`
	TargetSeconds int     `json:"target_seconds"`
	TotalSeconds  int     `json:"total_seconds"`
	TotalWords    int     `json:"total_words"`
	Chunks        []Chunk `json:"chunks"`
}

func (p Params) normalized() Params {
	if p.MinFill <= 0 || p.MinFill >= 1 {
		p.MinFill = 0.6
	}
	if p.MaxOvershoot < 1 {
		p.MaxOvershoot = 1.4
	}
	if p.WPM <= 0 {
		p.WPM = 200
	}
	if p.Difficulty <= 0 {
		p.Difficulty = 1
	}
	if p.Mode == "" {
		p.Mode = ModeMinutesPerDay
	}
	return p
}

// Target devolve o alvo em segundos para o total informado.
func (p Params) Target(totalSeconds int) (int, error) {
	p = p.normalized()

	switch p.Mode {
	case ModeMinutesPerDay:
		if p.Minutes <= 0 {
			return 0, errors.New("plano: minutes_per_day exige minutos > 0")
		}
		return p.Minutes * 60, nil
	case ModeFinishByDays:
		if p.Days <= 0 {
			return 0, errors.New("plano: finish_by_days exige dias > 0")
		}
		target := totalSeconds / p.Days
		if target < 1 {
			target = 1
		}
		return target, nil
	default:
		return 0, fmt.Errorf("plano: modo desconhecido %q", p.Mode)
	}
}

// Warnings devolve avisos de sanidade do ritmo (RF-026). O plano continua
// válido: os avisos só indicam que o pacing escolhido produz fragmentação,
// dias longos demais ou dias sem conteúdo. É uma função pura e determinística.
func (p Params) Warnings(targetSeconds, chunks int) []string {
	p = p.normalized()
	if p.Mode != ModeFinishByDays {
		return nil
	}

	var avisos []string
	if targetSeconds < 180 {
		avisos = append(avisos, fmt.Sprintf(
			"Alvo de %s por dia é baixo demais para este conteúdo (%s): a leitura fica fragmentada.",
			FormatDuration(targetSeconds), diasTexto(p.Days)))
	} else if targetSeconds > 3600 {
		avisos = append(avisos, fmt.Sprintf(
			"Alvo de %s por dia é alto demais para este conteúdo (%s): cada dia terá mais de 1 h.",
			FormatDuration(targetSeconds), diasTexto(p.Days)))
	}
	if chunks > 0 && p.Days > chunks {
		restantes := p.Days - chunks
		if restantes == 1 {
			avisos = append(avisos, fmt.Sprintf(
				"Sobra 1 dia sem conteúdo: o documento termina em %s de leitura.",
				diasTexto(chunks)))
		} else {
			avisos = append(avisos, fmt.Sprintf(
				"Sobram %s sem conteúdo: o documento termina em %s de leitura.",
				diasTexto(restantes), diasTexto(chunks)))
		}
	}
	return avisos
}

func diasTexto(n int) string {
	if n == 1 {
		return "1 dia"
	}
	return fmt.Sprintf("%d dias", n)
}

// bounds devolve alvo, mínimo e máximo em segundos.
func (p Params) bounds(totalSeconds int) (target, min, max int, err error) {
	p = p.normalized()
	target, err = p.Target(totalSeconds)
	if err != nil {
		return 0, 0, 0, err
	}
	min = int(float64(target) * p.MinFill)
	max = int(float64(target) * p.MaxOvershoot)
	if max < target {
		max = target
	}
	if min > target {
		min = target
	}
	return target, min, max, nil
}

// Generate produz o plano completo de um documento.
//
// É uma função pura: o mesmo conjunto de blocos e parâmetros produz exatamente
// o mesmo plano, sem depender de relógio, aleatoriedade ou ordem de mapas.
func Generate(bs []blocks.Block, p Params) (*Plan, error) {
	if len(bs) == 0 {
		return nil, errors.New("plano: documento sem blocos")
	}
	for i := range bs {
		if bs[i].Ord != i {
			return nil, fmt.Errorf("plano: bloco %d com Ord %d (esperado %d)", i, bs[i].Ord, i)
		}
	}

	p = p.normalized()

	totalSeconds, totalWords := 0, 0
	for _, b := range bs {
		totalSeconds += b.Seconds
		totalWords += b.Words
	}

	target, min, max, err := p.bounds(totalSeconds)
	if err != nil {
		return nil, err
	}

	tree := blocks.BuildTree(bs)
	units := collectUnits(bs, tree.Items, target, max)
	units = mergeHeadings(bs, units)
	units = carryTrailingHeadings(bs, units)
	groups := packUnits(units, target, min, max)
	groups = balance(groups, target, min, max)

	chunks, err := materialize(bs, groups, target, max)
	if err != nil {
		return nil, err
	}

	pl := &Plan{
		Mode:          p.Mode,
		TargetSeconds: target,
		TotalSeconds:  totalSeconds,
		TotalWords:    totalWords,
		Chunks:        chunks,
	}
	if err := Validate(bs, pl, p); err != nil {
		return nil, err
	}
	return pl, nil
}
