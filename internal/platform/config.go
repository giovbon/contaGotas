// Package platform concentra configuração, logging e utilidades de processo.
package platform

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// Defaults de configuração (docs/operations.md).
const (
	DefaultAddr           = ":8080"
	DefaultDataDir        = "/data"
	DefaultBaseURL        = "http://localhost:8080"
	DefaultMaxUploadBytes = 99_614_720 // 95 MiB
)

// Config é a configuração carregada do ambiente. Senha e segredo são
// validados por auth.New; aqui só se garante o formato dos demais campos.
type Config struct {
	Addr             string
	DataDir          string
	BaseURL          string
	Password         string
	Secret           []byte
	BehindCloudflare bool
	MaxUploadBytes   int64
	LogLevel         slog.Level
}

// DBPath é o caminho do SQLite dentro do diretório de dados.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "db", "app.db") }

// BackupDir é o diretório dos backups gravados pelo app (pré-migração).
func (c Config) BackupDir() string { return filepath.Join(c.DataDir, "backup") }

// LoadEnv lê a configuração de getenv. Valores ausentes usam os padrões de
// docs/operations.md; valores inválidos devolvem erro em vez de cair no
// padrão silenciosamente.
func LoadEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, errors.New("platform: getenv nulo")
	}

	cfg := Config{
		Addr:           valor(getenv, "APP_ADDR", DefaultAddr),
		DataDir:        valor(getenv, "APP_DATA_DIR", DefaultDataDir),
		BaseURL:        valor(getenv, "APP_BASE_URL", DefaultBaseURL),
		Password:       getenv("APP_PASSWORD"),
		Secret:         []byte(getenv("APP_SECRET")),
		MaxUploadBytes: DefaultMaxUploadBytes,
		LogLevel:       slog.LevelInfo,
	}

	if bruto := strings.TrimSpace(getenv("MAX_UPLOAD_BYTES")); bruto != "" {
		n, err := strconv.ParseInt(bruto, 10, 64)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("platform: MAX_UPLOAD_BYTES inválido: %q", bruto)
		}
		cfg.MaxUploadBytes = n
	}

	if bruto := strings.TrimSpace(getenv("BEHIND_CLOUDFLARE")); bruto != "" {
		b, err := strconv.ParseBool(bruto)
		if err != nil {
			return Config{}, fmt.Errorf("platform: BEHIND_CLOUDFLARE inválido: %q", bruto)
		}
		cfg.BehindCloudflare = b
	}

	if bruto := strings.TrimSpace(getenv("LOG_LEVEL")); bruto != "" {
		level, err := ParseLogLevel(bruto)
		if err != nil {
			return Config{}, err
		}
		cfg.LogLevel = level
	}

	if cfg.DataDir == "" {
		return Config{}, errors.New("platform: APP_DATA_DIR vazio")
	}
	if cfg.Addr == "" {
		return Config{}, errors.New("platform: APP_ADDR vazio")
	}
	if err := validarBaseURL(cfg.BaseURL); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func valor(getenv func(string) string, chave, padrao string) string {
	if v := strings.TrimSpace(getenv(chave)); v != "" {
		return v
	}
	return padrao
}

func validarBaseURL(bruto string) error {
	u, err := url.Parse(bruto)
	if err != nil {
		return fmt.Errorf("platform: APP_BASE_URL inválida: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("platform: APP_BASE_URL precisa ser http(s) com host: %q", bruto)
	}
	return nil
}
