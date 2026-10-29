package planconfig_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/planconfig"
	"github.com/giovani/readingdrip/internal/store"
)

func agora() time.Time {
	return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
}

func TestParseDefaults(t *testing.T) {
	cfg, err := planconfig.Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if cfg != planconfig.Default() {
		t.Fatalf("Parse(nil) = %+v, quer %+v", cfg, planconfig.Default())
	}
}

func TestParseValores(t *testing.T) {
	cfg, err := planconfig.Parse(map[string]string{
		planconfig.KeyWPM:          "250",
		planconfig.KeyDifficulty:   "1.25",
		planconfig.KeyMinFill:      "0.5",
		planconfig.KeyMaxOvershoot: "1.6",
		planconfig.KeyRecap:        "false",
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := planconfig.Config{
		WPM:          250,
		Difficulty:   1.25,
		MinFill:      0.5,
		MaxOvershoot: 1.6,
		Recap:        false,
	}
	if cfg != want {
		t.Fatalf("Parse = %+v, quer %+v", cfg, want)
	}
}

func TestParseIgnoraVazio(t *testing.T) {
	cfg, err := planconfig.Parse(map[string]string{
		planconfig.KeyWPM:          "  ",
		planconfig.KeyDifficulty:   "",
		planconfig.KeyMinFill:      "\t",
		planconfig.KeyMaxOvershoot: "",
		planconfig.KeyRecap:        " ",
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg != planconfig.Default() {
		t.Fatalf("Parse = %+v, quer padrões", cfg)
	}
}

func TestParseInvalidos(t *testing.T) {
	casos := []struct {
		nome   string
		chave  string
		valor  string
		trecho string
	}{
		{"wpm texto", planconfig.KeyWPM, "rápido", planconfig.KeyWPM},
		{"wpm baixo", planconfig.KeyWPM, "49", planconfig.KeyWPM},
		{"wpm alto", planconfig.KeyWPM, "1001", planconfig.KeyWPM},
		{"dificuldade baixa", planconfig.KeyDifficulty, "0.4", planconfig.KeyDifficulty},
		{"dificuldade alta", planconfig.KeyDifficulty, "3.1", planconfig.KeyDifficulty},
		{"dificuldade nan", planconfig.KeyDifficulty, "NaN", planconfig.KeyDifficulty},
		{"min_fill baixo", planconfig.KeyMinFill, "0.29", planconfig.KeyMinFill},
		{"min_fill alto", planconfig.KeyMinFill, "0.96", planconfig.KeyMinFill},
		{"min_fill um", planconfig.KeyMinFill, "1", planconfig.KeyMinFill},
		{"max_overshoot baixo", planconfig.KeyMaxOvershoot, "0.99", planconfig.KeyMaxOvershoot},
		{"max_overshoot alto", planconfig.KeyMaxOvershoot, "3.01", planconfig.KeyMaxOvershoot},
		{"max_overshoot inf", planconfig.KeyMaxOvershoot, "+Inf", planconfig.KeyMaxOvershoot},
		{"recap inválido", planconfig.KeyRecap, "talvez", planconfig.KeyRecap},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			_, err := planconfig.Parse(map[string]string{tc.chave: tc.valor})
			if err == nil {
				t.Fatalf("Parse(%s=%q) não falhou", tc.chave, tc.valor)
			}
			if !strings.Contains(err.Error(), tc.trecho) {
				t.Fatalf("erro = %q, quer menção a %q", err, tc.trecho)
			}
		})
	}
}

func TestParseLimites(t *testing.T) {
	casos := []map[string]string{
		{planconfig.KeyWPM: fmt.Sprint(planconfig.MinWPM)},
		{planconfig.KeyWPM: fmt.Sprint(planconfig.MaxWPM)},
		{planconfig.KeyDifficulty: fmt.Sprint(planconfig.MinDifficulty)},
		{planconfig.KeyDifficulty: fmt.Sprint(planconfig.MaxDifficulty)},
		{planconfig.KeyMinFill: fmt.Sprint(planconfig.MinFillMin)},
		{planconfig.KeyMinFill: fmt.Sprint(planconfig.MinFillMax)},
		{planconfig.KeyMaxOvershoot: fmt.Sprint(planconfig.MaxOvershootMin)},
		{planconfig.KeyMaxOvershoot: fmt.Sprint(planconfig.MaxOvershootMax)},
	}
	for i, valores := range casos {
		if _, err := planconfig.Parse(valores); err != nil {
			t.Errorf("caso %d (%v): limite deveria ser aceito: %v", i, valores, err)
		}
	}
}

func TestLoad(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})
	if _, err := planconfig.Load(ctx, s.Settings()); err != nil {
		t.Fatalf("Load sem chaves: %v", err)
	}
	if err := s.Settings().Set(ctx, planconfig.KeyWPM, "300", agora()); err != nil {
		t.Fatal(err)
	}
	cfg, err := planconfig.Load(ctx, s.Settings())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WPM != 300 {
		t.Fatalf("WPM = %d, quer 300", cfg.WPM)
	}
}

func TestLoadNulo(t *testing.T) {
	if _, err := planconfig.Load(context.Background(), nil); err == nil {
		t.Fatal("Load(nil) não falhou")
	}
}
