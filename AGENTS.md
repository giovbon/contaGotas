# AGENTS.md

Instruções para agentes de IA que trabalham neste repositório.

## Leia antes de agir

1. `docs/context.md` — decisões fechadas, invariantes, estado atual.
2. `docs/requirements.md` — escopo e critérios de aceite.
3. O doc da área que você vai alterar (índice em `docs/README.md`).
4. `docs/contributing.md` — regras de código, checklists e Definition of Done.

## Regras inegociáveis

- Não invente contratos: schema, rotas e algoritmos são os documentados. Divergência exige atualizar a doc primeiro.
- `internal/plan` é puro e determinístico: sem I/O, sem relógio, sem `rand`, sem iteração de `map` para gerar ordem.
- Nunca quebre as invariantes `I-01`…`I-10` de `docs/context.md`.
- Toda mudança de comportamento atualiza a documentação e `docs/changelog.md` no mesmo commit.
- Decisão estrutural nova → ADR em `docs/adr/`; dúvida → `docs/open-questions.md`. Nada de `TODO` solto.
- Não logue corpo de leitura, tokens ou `APP_SECRET`.
- Sem dependência nova sem ADR.

## Comandos

```sh
go test ./... -race
CGO_ENABLED=0 go test ./internal/plan/... ./internal/blocks/...
go run github.com/a-h/templ/cmd/templ@v0.3.900 generate
```

## Definition of Done

Ver `docs/contributing.md`. Em resumo: requisito testado, testes verdes, docs atualizados, changelog, build ARM64 passando.
