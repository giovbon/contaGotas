// Package schedule implementa o ticker de entrega: lê as configurações, decide
// se é hora de uma run, monta as doses e publica no ntfy.
package schedule

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/giovani/readingdrip/internal/delivery"
	"github.com/giovani/readingdrip/internal/store"
)

// Chaves de configuração (docs/scheduling.md e docs/delivery-ntfy.md).
const (
	KeyEnabled           = "schedule.enabled"
	KeyDays              = "schedule.days"
	KeyTime              = "schedule.time"
	KeyTimezone          = "schedule.timezone"
	KeyCatchupDays       = "schedule.catchup_days"
	KeyMaxDocsPerRun     = "schedule.max_docs_per_run"
	KeyNtfyBaseURL       = "ntfy.base_url"
	KeyNtfyTopic         = "ntfy.topic"
	KeyNtfyToken         = "ntfy.token"
	KeyNtfyPriority      = "ntfy.priority"
	KeyNtfyBodyMode      = "ntfy.body_mode"
	KeyNtfyMaxBytes      = "ntfy.max_message_bytes"
	KeyReaderTTLDays     = "reader.link_ttl_days"
	KeyReaderDoneTTLDays = "reader.done_ttl_days"
)

// Padrões documentados.
const (
	DefaultDays            = "mon,tue,wed,thu,fri"
	DefaultTime            = "07:00"
	DefaultTimezone        = "America/Sao_Paulo"
	DefaultCatchupDays     = 1
	DefaultMaxDocsPerRun   = 5
	DefaultNtfyPriority    = 3
	DefaultMaxMessageBytes = delivery.DefaultMaxMessageBytes
	DefaultReaderTTLDays   = 180
	DefaultReaderDoneDays  = 30
	maxTTLDias             = 3650
)

var nomesDia = map[string]time.Weekday{
	"sun": time.Sunday,
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
}

// Config é a parte de agenda.
type Config struct {
	Enabled       bool
	Days          []time.Weekday
	Time          string
	Hour          int
	Minute        int
	Location      *time.Location
	CatchupDays   int
	MaxDocsPerRun int
}

// NtfyConfig é a parte de entrega.
type NtfyConfig struct {
	BaseURL         string
	Topic           string
	Token           string
	Priority        int
	BodyMode        delivery.BodyMode
	MaxMessageBytes int
}

// ReaderConfig guarda as validades dos links assinados do leitor.
type ReaderConfig struct {
	TTLDays     int
	DoneTTLDays int
}

// Settings agrega agenda, ntfy e leitor.
type Settings struct {
	Schedule Config
	Ntfy     NtfyConfig
	Reader   ReaderConfig
}

// LinkTTLs devolve as validades em dias convertidas para duração.
func (st Settings) LinkTTLs() (leitor, lido time.Duration) {
	return time.Duration(st.Reader.TTLDays) * 24 * time.Hour,
		time.Duration(st.Reader.DoneTTLDays) * 24 * time.Hour
}

// LoadSettings lê as configurações do banco e aplica os padrões. Valores
// inválidos devolvem erro citando a chave; nada é corrigido silenciosamente.
func LoadSettings(ctx context.Context, s *store.Settings) (Settings, error) {
	if s == nil {
		return Settings{}, fmt.Errorf("schedule: repositório de configurações nulo")
	}
	all, err := s.All(ctx)
	if err != nil {
		return Settings{}, err
	}
	return Parse(all)
}

// Parse valida um mapa de chaves/valores (as mesmas chaves lidas do banco) e
// devolve as configurações efetivas, aplicando os padrões para chaves
// ausentes. A UI usa a mesma função antes de gravar, para nunca persistir um
// valor que o agendador depois recusaria.
func Parse(all map[string]string) (Settings, error) {
	st := Settings{
		Schedule: Config{
			Enabled:       true,
			Time:          DefaultTime,
			Hour:          7,
			CatchupDays:   DefaultCatchupDays,
			MaxDocsPerRun: DefaultMaxDocsPerRun,
		},
		Ntfy: NtfyConfig{
			Priority:        DefaultNtfyPriority,
			BodyMode:        delivery.BodyFull,
			MaxMessageBytes: DefaultMaxMessageBytes,
		},
		Reader: ReaderConfig{
			TTLDays:     DefaultReaderTTLDays,
			DoneTTLDays: DefaultReaderDoneDays,
		},
	}

	if bruto, ok := valor(all, KeyEnabled); ok {
		b, err := strconv.ParseBool(bruto)
		if err != nil {
			return Settings{}, fmt.Errorf("schedule: %s inválido: %q", KeyEnabled, bruto)
		}
		st.Schedule.Enabled = b
	}

	dias := DefaultDays
	if bruto, ok := valor(all, KeyDays); ok {
		dias = bruto
	}
	listaDias, err := parsearDias(dias)
	if err != nil {
		return Settings{}, err
	}
	st.Schedule.Days = listaDias

	if bruto, ok := valor(all, KeyTime); ok {
		st.Schedule.Time = bruto
	}
	h, m, err := parsearHora(st.Schedule.Time)
	if err != nil {
		return Settings{}, err
	}
	st.Schedule.Hour, st.Schedule.Minute = h, m

	fuso := DefaultTimezone
	if bruto, ok := valor(all, KeyTimezone); ok {
		fuso = bruto
	}
	if st.Schedule.Location, err = time.LoadLocation(fuso); err != nil {
		return Settings{}, fmt.Errorf("schedule: %s inválido: %q: %w", KeyTimezone, fuso, err)
	}

	if bruto, ok := valor(all, KeyCatchupDays); ok {
		n, err := strconv.Atoi(bruto)
		if err != nil || n < 0 {
			return Settings{}, fmt.Errorf("schedule: %s inválido: %q", KeyCatchupDays, bruto)
		}
		st.Schedule.CatchupDays = n
	}

	if bruto, ok := valor(all, KeyMaxDocsPerRun); ok {
		n, err := strconv.Atoi(bruto)
		if err != nil || n <= 0 {
			return Settings{}, fmt.Errorf("schedule: %s inválido: %q", KeyMaxDocsPerRun, bruto)
		}
		st.Schedule.MaxDocsPerRun = n
	}

	if st.Reader.TTLDays, err = diasDeLink(all, KeyReaderTTLDays, st.Reader.TTLDays); err != nil {
		return Settings{}, err
	}
	if st.Reader.DoneTTLDays, err = diasDeLink(all, KeyReaderDoneTTLDays, st.Reader.DoneTTLDays); err != nil {
		return Settings{}, err
	}

	st.Ntfy.BaseURL, _ = valor(all, KeyNtfyBaseURL)
	st.Ntfy.Topic, _ = valor(all, KeyNtfyTopic)
	st.Ntfy.Token, _ = valor(all, KeyNtfyToken)

	if bruto, ok := valor(all, KeyNtfyPriority); ok {
		n, err := strconv.Atoi(bruto)
		if err != nil || n < 1 || n > 5 {
			return Settings{}, fmt.Errorf("schedule: %s inválido: %q (use 1..5)", KeyNtfyPriority, bruto)
		}
		st.Ntfy.Priority = n
	}

	if bruto, ok := valor(all, KeyNtfyBodyMode); ok {
		switch delivery.BodyMode(bruto) {
		case delivery.BodyFull, delivery.BodyPreview:
			st.Ntfy.BodyMode = delivery.BodyMode(bruto)
		default:
			return Settings{}, fmt.Errorf("schedule: %s inválido: %q (use full ou preview)", KeyNtfyBodyMode, bruto)
		}
	}

	if bruto, ok := valor(all, KeyNtfyMaxBytes); ok {
		n, err := strconv.Atoi(bruto)
		if err != nil || n < delivery.MinMessageBytes {
			return Settings{}, fmt.Errorf("schedule: %s inválido: %q (mínimo %d)",
				KeyNtfyMaxBytes, bruto, delivery.MinMessageBytes)
		}
		st.Ntfy.MaxMessageBytes = n
	}

	return st, nil
}

// diasDeLink valida uma validade em dias (1..maxTTLDias), aplicando o padrão
// quando a chave não existe.
func diasDeLink(all map[string]string, chave string, padrao int) (int, error) {
	bruto, ok := valor(all, chave)
	if !ok {
		return padrao, nil
	}
	n, err := strconv.Atoi(bruto)
	if err != nil || n <= 0 || n > maxTTLDias {
		return 0, fmt.Errorf("schedule: %s inválido: %q (use 1..%d dias)", chave, bruto, maxTTLDias)
	}
	return n, nil
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

func parsearDias(bruto string) ([]time.Weekday, error) {
	partes := strings.Split(bruto, ",")
	vistos := make(map[time.Weekday]bool)
	dias := make([]time.Weekday, 0, len(partes))
	for _, parte := range partes {
		nome := strings.ToLower(strings.TrimSpace(parte))
		if nome == "" {
			continue
		}
		dia, ok := nomesDia[nome]
		if !ok {
			return nil, fmt.Errorf("schedule: %s inválido: %q (use sun, mon, tue, wed, thu, fri, sat)", KeyDays, bruto)
		}
		if vistos[dia] {
			continue
		}
		vistos[dia] = true
		dias = append(dias, dia)
	}
	if len(dias) == 0 {
		return nil, fmt.Errorf("schedule: %s vazio", KeyDays)
	}
	return dias, nil
}

func parsearHora(bruto string) (int, int, error) {
	t, err := time.Parse("15:04", bruto)
	if err != nil {
		return 0, 0, fmt.Errorf("schedule: %s inválido: %q (use HH:MM)", KeyTime, bruto)
	}
	return t.Hour(), t.Minute(), nil
}

// scheduledFor devolve o horário configurado para o dia de `dia`, no fuso
// configurado.
func (c Config) scheduledFor(dia time.Time) time.Time {
	return time.Date(dia.Year(), dia.Month(), dia.Day(), c.Hour, c.Minute, 0, 0, c.Location)
}

// isReadingDay informa se o dia da semana está configurado.
func (c Config) isReadingDay(dia time.Weekday) bool {
	for _, d := range c.Days {
		if d == dia {
			return true
		}
	}
	return false
}
