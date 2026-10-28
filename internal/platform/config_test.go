package platform

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func mapa(valores map[string]string) func(string) string {
	return func(chave string) string { return valores[chave] }
}

func TestLoadEnvPadroes(t *testing.T) {
	cfg, err := LoadEnv(mapa(map[string]string{
		"APP_PASSWORD": "segredo",
		"APP_SECRET":   strings.Repeat("k", 32),
	}))
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Addr != DefaultAddr {
		t.Errorf("Addr = %q", cfg.Addr)
	}
	if cfg.DataDir != DefaultDataDir {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.MaxUploadBytes != DefaultMaxUploadBytes {
		t.Errorf("MaxUploadBytes = %d", cfg.MaxUploadBytes)
	}
	if cfg.BehindCloudflare {
		t.Error("BehindCloudflare ligado por padrão")
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v", cfg.LogLevel)
	}
	if cfg.Password != "segredo" || string(cfg.Secret) != strings.Repeat("k", 32) {
		t.Error("credenciais não preservadas")
	}
}

func TestLoadEnvSobrescreve(t *testing.T) {
	cfg, err := LoadEnv(mapa(map[string]string{
		"APP_ADDR":          "127.0.0.1:9000",
		"APP_DATA_DIR":      "/tmp/dados",
		"APP_BASE_URL":      "https://leitura.exemplo.com",
		"APP_PASSWORD":      "outra",
		"APP_SECRET":        strings.Repeat("s", 32),
		"BEHIND_CLOUDFLARE": "true",
		"MAX_UPLOAD_BYTES":  "1048576",
		"LOG_LEVEL":         "DEBUG",
	}))
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9000" || cfg.DataDir != "/tmp/dados" {
		t.Errorf("addr/data = %q / %q", cfg.Addr, cfg.DataDir)
	}
	if cfg.BaseURL != "https://leitura.exemplo.com" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if !cfg.BehindCloudflare || cfg.MaxUploadBytes != 1_048_576 {
		t.Errorf("flags = %v / %d", cfg.BehindCloudflare, cfg.MaxUploadBytes)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v", cfg.LogLevel)
	}
	if quer := filepath.Join("/tmp/dados", "db", "app.db"); cfg.DBPath() != quer {
		t.Errorf("DBPath = %q, quer %q", cfg.DBPath(), quer)
	}
}

func TestLoadEnvErros(t *testing.T) {
	casos := []struct {
		nome    string
		valores map[string]string
		trecho  string
	}{
		{"max upload", map[string]string{"MAX_UPLOAD_BYTES": "zero"}, "MAX_UPLOAD_BYTES"},
		{"max upload negativo", map[string]string{"MAX_UPLOAD_BYTES": "-1"}, "MAX_UPLOAD_BYTES"},
		{"cloudflare", map[string]string{"BEHIND_CLOUDFLARE": "talvez"}, "BEHIND_CLOUDFLARE"},
		{"log level", map[string]string{"LOG_LEVEL": "trace"}, "LOG_LEVEL"},
		{"base url sem esquema", map[string]string{"APP_BASE_URL": "leitura.exemplo.com"}, "APP_BASE_URL"},
		{"base url sem host", map[string]string{"APP_BASE_URL": "https://"}, "APP_BASE_URL"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := LoadEnv(mapa(c.valores))
			if err == nil {
				t.Fatal("esperava erro")
			}
			if !strings.Contains(err.Error(), c.trecho) {
				t.Errorf("erro = %q, quer menção a %q", err, c.trecho)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	casos := map[string]slog.Level{
		"":        slog.LevelInfo,
		"info":    slog.LevelInfo,
		"DEBUG":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
	}
	for bruto, esperado := range casos {
		nivel, err := ParseLogLevel(bruto)
		if err != nil {
			t.Errorf("%q: %v", bruto, err)
			continue
		}
		if nivel != esperado {
			t.Errorf("%q = %v, quer %v", bruto, nivel, esperado)
		}
	}
	if _, err := ParseLogLevel("verboso"); err == nil {
		t.Error("esperava erro para nível desconhecido")
	}
}
