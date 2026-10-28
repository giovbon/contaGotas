package store

import (
	"context"
	"testing"
	"time"
)

func TestSettingsSetGetAll(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	if _, ok, err := s.Settings().Get(ctx, "ntfy_topic"); err != nil || ok {
		t.Fatalf("chave ausente: ok=%v err=%v", ok, err)
	}
	if err := s.Settings().Set(ctx, "ntfy_topic", "leitura", agora); err != nil {
		t.Fatal(err)
	}
	valor, ok, err := s.Settings().Get(ctx, "ntfy_topic")
	if err != nil || !ok || valor != "leitura" {
		t.Fatalf("get = %q, %v, %v", valor, ok, err)
	}

	// Sobrescreve sem duplicar.
	if err := s.Settings().Set(ctx, "ntfy_topic", "outro", agora.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	valor, _, err = s.Settings().Get(ctx, "ntfy_topic")
	if err != nil || valor != "outro" {
		t.Fatalf("valor após sobrescrever = %q, %v", valor, err)
	}

	todos, err := s.Settings().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 1 || todos["ntfy_topic"] != "outro" {
		t.Fatalf("all = %v", todos)
	}
}

func TestSettingsSetMany(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	if err := s.Settings().Set(ctx, "schedule.time", "07:00", agora); err != nil {
		t.Fatal(err)
	}
	err := s.Settings().SetMany(ctx, map[string]string{
		"schedule.time": "21:30",
		"ntfy.topic":    "leitura",
		"ntfy.token":    "",
	}, agora.Add(time.Minute))
	if err != nil {
		t.Fatalf("gravar em lote: %v", err)
	}

	todos, err := s.Settings().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if todos["schedule.time"] != "21:30" || todos["ntfy.topic"] != "leitura" {
		t.Fatalf("all = %v", todos)
	}
	if v, ok := todos["ntfy.token"]; !ok || v != "" {
		t.Errorf("token vazio deveria ser gravado como vazio: %q, %v", v, ok)
	}

	// Chave vazia aborta o lote inteiro (transação).
	err = s.Settings().SetMany(ctx, map[string]string{"schedule.enabled": "false", "": "x"}, agora)
	if err == nil {
		t.Fatal("esperado erro para chave vazia")
	}
	if _, ok, err := s.Settings().Get(ctx, "schedule.enabled"); err != nil || ok {
		t.Errorf("lote inválido não pode gravar nada: ok=%v err=%v", ok, err)
	}

	if err := s.Settings().SetMany(ctx, nil, agora); err != nil {
		t.Errorf("lote vazio deveria ser no-op: %v", err)
	}
}

func TestEventsAddList(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id, err := s.Documents().Create(ctx, docExemplo("Livro", "sha-ev"))
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Events().Add(ctx, Event{At: agora, Level: EventInfo, Kind: "upload", Message: "recebido"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Events().Add(ctx, Event{At: agora.Add(time.Second), Level: EventError, Kind: "extract", DocumentID: &id, Message: "falhou"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Events().Add(ctx, Event{Level: EventInfo, Kind: "x"}); err == nil {
		t.Fatal("esperado erro para evento sem data")
	}

	eventos, err := s.Events().List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventos) != 2 {
		t.Fatalf("eventos = %d, quer 2", len(eventos))
	}
	if eventos[0].Kind != "extract" {
		t.Errorf("ordem: primeiro = %q, quer o mais recente", eventos[0].Kind)
	}
	if eventos[0].DocumentID == nil || *eventos[0].DocumentID != id {
		t.Errorf("document_id = %v", eventos[0].DocumentID)
	}
	if eventos[1].DocumentID != nil {
		t.Errorf("document_id deveria ser nulo, veio %v", eventos[1].DocumentID)
	}
	if !eventos[0].At.Equal(agora.Add(time.Second)) {
		t.Errorf("at = %v", eventos[0].At)
	}
}

func TestEventsListLimitePadrao(t *testing.T) {
	s := abrirTeste(t)
	eventos, err := s.Events().List(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventos) != 0 {
		t.Errorf("eventos = %d, quer 0", len(eventos))
	}
}
