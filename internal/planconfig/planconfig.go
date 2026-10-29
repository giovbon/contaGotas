// Package planconfig lê e valida os padrões de plano usados na importação e
// na extração. São valores globais; o pacing efetivo continua por documento.
package planconfig

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/giovani/readingdrip/internal/store"
)

// Chaves de configuração (docs/segmentation.md).
const (
	KeyWPM          = "plan.wpm"
	KeyDifficulty   = "plan.difficulty"
	KeyMinFill      = "plan.min_fill"
	KeyMaxOvershoot = "plan.max_overshoot"
	KeyRecap        = "plan.recap"
)

// Padrões documentados.
const (
	DefaultWPM          = 200
	DefaultDifficulty   = 1.0
	DefaultMinFill      = 0.60
	DefaultMaxOvershoot = 1.40
	DefaultRecap        = true

	MinWPM          = 50
	MaxWPM          = 1000
	MinDifficulty   = 0.5
	MaxDifficulty   = 3.0
	MinFillMin      = 0.30
	MinFillMax      = 0.95
	MaxOvershootMin = 1.0
	MaxOvershootMax = 3.0
)

// Config são os padrões de plano.
type Config struct {
	WPM          int
	Difficulty   float64
	MinFill      float64
	MaxOvershoot float64
	Recap        bool
}

// Default devolve os padrões de docs/segmentation.md.
func Default() Config {
	return Config{
		WPM:          DefaultWPM,
		Difficulty:   DefaultDifficulty,
		MinFill:      DefaultMinFill,
		MaxOvershoot: DefaultMaxOvershoot,
		Recap:        DefaultRecap,
	}
}

// Load lê as configurações do banco e aplica os padrões. Valores inválidos
// devolvem erro citando a chave; nada é corrigido silenciosamente.
func Load(ctx context.Context, s *store.Settings) (Config, error) {
	if s == nil {
		return Config{}, fmt.Errorf("planconfig: repositório de configurações nulo")
	}
	all, err := s.All(ctx)
	if err != nil {
		return Config{}, err
	}
	return Parse(all)
}

// Parse valida um mapa de chaves/valores e devolve os padrões efetivos,
// aplicando Default para chaves ausentes. A UI usa a mesma função antes de
// gravar, para não persistir um valor que a importação recusaria.
func Parse(all map[string]string) (Config, error) {
	cfg := Default()

	if bruto, ok := valor(all, KeyWPM); ok {
		n, err := strconv.Atoi(bruto)
		if err != nil || n < MinWPM || n > MaxWPM {
			return Config{}, fmt.Errorf("planconfig: %s inválido: %q (use %d..%d)",
				KeyWPM, bruto, MinWPM, MaxWPM)
		}
		cfg.WPM = n
	}

	if bruto, ok := valor(all, KeyDifficulty); ok {
		v, err := decimal(bruto)
		if err != nil || v < MinDifficulty || v > MaxDifficulty {
			return Config{}, fmt.Errorf("planconfig: %s inválido: %q (use %.1f..%.1f)",
				KeyDifficulty, bruto, MinDifficulty, MaxDifficulty)
		}
		cfg.Difficulty = v
	}

	if bruto, ok := valor(all, KeyMinFill); ok {
		v, err := decimal(bruto)
		if err != nil || v < MinFillMin || v > MinFillMax {
			return Config{}, fmt.Errorf("planconfig: %s inválido: %q (use %.2f..%.2f)",
				KeyMinFill, bruto, MinFillMin, MinFillMax)
		}
		cfg.MinFill = v
	}

	if bruto, ok := valor(all, KeyMaxOvershoot); ok {
		v, err := decimal(bruto)
		if err != nil || v < MaxOvershootMin || v > MaxOvershootMax {
			return Config{}, fmt.Errorf("planconfig: %s inválido: %q (use %.2f..%.2f)",
				KeyMaxOvershoot, bruto, MaxOvershootMin, MaxOvershootMax)
		}
		cfg.MaxOvershoot = v
	}

	if bruto, ok := valor(all, KeyRecap); ok {
		b, err := strconv.ParseBool(bruto)
		if err != nil {
			return Config{}, fmt.Errorf("planconfig: %s inválido: %q", KeyRecap, bruto)
		}
		cfg.Recap = b
	}

	return cfg, nil
}

// decimal aceita apenas números finitos; NaN e Inf não são valores válidos de
// configuração.
func decimal(bruto string) (float64, error) {
	v, err := strconv.ParseFloat(bruto, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("valor não finito: %q", bruto)
	}
	return v, nil
}

func valor(all map[string]string, chave string) (string, bool) {
	v, ok := all[chave]
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	return v, true
}
