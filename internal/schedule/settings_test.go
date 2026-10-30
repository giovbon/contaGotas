package schedule

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/planconfig"
	"github.com/giovani/readingdrip/internal/store"
)

func abrirStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})
	return s
}

func definir(t *testing.T, s *store.Store, chave, valor string) {
	t.Helper()
	if err := s.Settings().Set(context.Background(), chave, valor, time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("gravar %s: %v", chave, err)
	}
}

func TestLoadSettingsPadroes(t *testing.T) {
	s := abrirStore(t)
	st, err := LoadSettings(context.Background(), s.Settings())
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}
	if !st.Schedule.Enabled {
		t.Error("agendador deveria vir ligado")
	}
	if st.Schedule.Time != DefaultTime || st.Schedule.Hour != 7 || st.Schedule.Minute != 0 {
		t.Errorf("horário = %q (%02d:%02d)", st.Schedule.Time, st.Schedule.Hour, st.Schedule.Minute)
	}
	if st.Schedule.Location == nil || st.Schedule.Location.String() != DefaultTimezone {
		t.Errorf("fuso = %v", st.Schedule.Location)
	}
	if st.Schedule.CatchupDays != DefaultCatchupDays {
		t.Errorf("catchup = %d", st.Schedule.CatchupDays)
	}
	if st.Schedule.MaxDocsPerRun != DefaultMaxDocsPerRun {
		t.Errorf("max docs = %d", st.Schedule.MaxDocsPerRun)
	}
	if len(st.Schedule.Days) != 5 || st.Schedule.Days[0] != time.Monday || st.Schedule.Days[4] != time.Friday {
		t.Errorf("dias = %v", st.Schedule.Days)
	}
	if st.Ntfy.BaseURL != "" || st.Ntfy.Topic != "" || st.Ntfy.Token != "" ||
		st.Ntfy.Username != "" || st.Ntfy.Password != "" {
		t.Errorf("ntfy deveria vir vazio: %+v", st.Ntfy)
	}
	if st.Ntfy.Priority != DefaultNtfyPriority {
		t.Errorf("prioridade = %d", st.Ntfy.Priority)
	}
	if st.Ntfy.BodyMode != "full" {
		t.Errorf("body_mode = %q", st.Ntfy.BodyMode)
	}
	if st.Ntfy.MaxMessageBytes != DefaultMaxMessageBytes {
		t.Errorf("max bytes = %d", st.Ntfy.MaxMessageBytes)
	}
	if st.Reader.TTLDays != DefaultReaderTTLDays || st.Reader.DoneTTLDays != DefaultReaderDoneDays {
		t.Errorf("validade dos links = %+v", st.Reader)
	}
	if st.Plan != planconfig.Default() {
		t.Errorf("plano = %+v, quer %+v", st.Plan, planconfig.Default())
	}
	if leitor, lido := st.LinkTTLs(); leitor != 180*24*time.Hour || lido != 30*24*time.Hour {
		t.Errorf("TTLs = %v, %v", leitor, lido)
	}
}

func TestLoadSettingsLeValores(t *testing.T) {
	s := abrirStore(t)
	definir(t, s, KeyEnabled, "false")
	definir(t, s, KeyDays, "mon, wed ,sun")
	definir(t, s, KeyTime, "21:45")
	definir(t, s, KeyTimezone, "UTC")
	definir(t, s, KeyCatchupDays, "3")
	definir(t, s, KeyMaxDocsPerRun, "2")
	definir(t, s, KeyNtfyBaseURL, "http://ntfy:8080")
	definir(t, s, KeyNtfyTopic, "leitura")
	definir(t, s, KeyNtfyToken, "tk_segredo")
	definir(t, s, KeyNtfyUsername, "leitor")
	definir(t, s, KeyNtfyPassword, "s3nh@")
	definir(t, s, KeyNtfyPriority, "5")
	definir(t, s, KeyNtfyBodyMode, "preview")
	definir(t, s, KeyNtfyMaxBytes, "12000")
	definir(t, s, KeyReaderTTLDays, "7")
	definir(t, s, KeyReaderDoneTTLDays, "2")
	definir(t, s, planconfig.KeyWPM, "240")
	definir(t, s, planconfig.KeyDifficulty, "1.2")
	definir(t, s, planconfig.KeyMinFill, "0.55")
	definir(t, s, planconfig.KeyMaxOvershoot, "1.7")
	definir(t, s, planconfig.KeyRecap, "false")

	st, err := LoadSettings(context.Background(), s.Settings())
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}
	if st.Schedule.Enabled {
		t.Error("agendador deveria estar desligado")
	}
	if len(st.Schedule.Days) != 3 ||
		st.Schedule.Days[0] != time.Monday ||
		st.Schedule.Days[1] != time.Wednesday ||
		st.Schedule.Days[2] != time.Sunday {
		t.Errorf("dias = %v", st.Schedule.Days)
	}
	if st.Schedule.Hour != 21 || st.Schedule.Minute != 45 {
		t.Errorf("horário = %02d:%02d", st.Schedule.Hour, st.Schedule.Minute)
	}
	if st.Schedule.Location.String() != "UTC" {
		t.Errorf("fuso = %v", st.Schedule.Location)
	}
	if st.Schedule.CatchupDays != 3 || st.Schedule.MaxDocsPerRun != 2 {
		t.Errorf("catchup/max docs = %d/%d", st.Schedule.CatchupDays, st.Schedule.MaxDocsPerRun)
	}
	if st.Ntfy.BaseURL != "http://ntfy:8080" || st.Ntfy.Topic != "leitura" || st.Ntfy.Token != "tk_segredo" {
		t.Errorf("ntfy = %+v", st.Ntfy)
	}
	if st.Ntfy.Username != "leitor" || st.Ntfy.Password != "s3nh@" {
		t.Errorf("ntfy basic = %+v", st.Ntfy)
	}
	if st.Ntfy.Priority != 5 || st.Ntfy.BodyMode != "preview" || st.Ntfy.MaxMessageBytes != 12000 {
		t.Errorf("ntfy entrega = %+v", st.Ntfy)
	}
	if st.Reader.TTLDays != 7 || st.Reader.DoneTTLDays != 2 {
		t.Errorf("validade dos links = %+v", st.Reader)
	}
	if st.Plan.WPM != 240 || st.Plan.Difficulty != 1.2 || st.Plan.MinFill != 0.55 ||
		st.Plan.MaxOvershoot != 1.7 || st.Plan.Recap {
		t.Errorf("plano = %+v", st.Plan)
	}
}

func TestLoadSettingsInvalidos(t *testing.T) {
	casos := []struct {
		chave, valor string
	}{
		{KeyEnabled, "talvez"},
		{KeyDays, "mon,feriado"},
		{KeyDays, " , "},
		{KeyTime, "7h"},
		{KeyTimezone, "Marte/Olympus"},
		{KeyCatchupDays, "-1"},
		{KeyCatchupDays, "muitos"},
		{KeyMaxDocsPerRun, "0"},
		{KeyNtfyPriority, "9"},
		{KeyNtfyBodyMode, "resumo"},
		{KeyNtfyMaxBytes, "10"},
		{KeyReaderTTLDays, "0"},
		{KeyReaderTTLDays, "4000"},
		{KeyReaderTTLDays, "muitos"},
		{KeyReaderDoneTTLDays, "-5"},
		{planconfig.KeyWPM, "10"},
		{planconfig.KeyMinFill, "1"},
		{planconfig.KeyRecap, "talvez"},
	}
	for _, caso := range casos {
		t.Run(caso.chave+"="+caso.valor, func(t *testing.T) {
			s := abrirStore(t)
			definir(t, s, caso.chave, caso.valor)
			if _, err := LoadSettings(context.Background(), s.Settings()); err == nil {
				t.Fatalf("%s=%q deveria falhar", caso.chave, caso.valor)
			}
		})
	}
}

func TestLoadSettingsBasicAuthIncompleto(t *testing.T) {
	casos := []struct {
		nome    string
		valores map[string]string
	}{
		{"usuario sem senha", map[string]string{KeyNtfyUsername: "leitor"}},
		{"senha sem usuario", map[string]string{KeyNtfyPassword: "s3nh@"}},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if _, err := Parse(caso.valores); err == nil {
				t.Fatalf("%v deveria falhar", caso.valores)
			}
		})
	}
}

func TestLoadSettingsSemRepositorio(t *testing.T) {
	if _, err := LoadSettings(context.Background(), nil); err == nil {
		t.Fatal("esperava erro para repositório nulo")
	}
}
