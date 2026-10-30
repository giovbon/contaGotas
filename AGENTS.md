# AGENTS.md

Instruções para agentes de IA que trabalham neste repositório.

## Leitura econômica (obrigatória)

Orçamento: no máximo ~10 mil tokens de documentação antes de tocar no código. Não leia documentos inteiros sem necessidade.

1. Sempre leia `docs/context.md` — decisões, invariantes e estado atual.
2. Se existir um brief da tarefa em `docs/tasks/<RF>.md`, leia o brief em vez dos docs de área. Em conflito, os docs de área vencem.
3. Para o resto, localize e leia apenas a seção necessária:
   - `rg -n 'RF-023' docs/requirements.md`
   - `rg -n '^## |^### ' docs/web-ui.md`
   - no `docs/changelog.md`, leia só a entrada mais recente.
4. Leia no máximo: o requisito afetado, a seção da área afetada, o checklist de `docs/contributing.md` e a entrada mais recente do changelog.
5. Não carregue `changelog.md`, `testing.md`, `requirements.md`, `data-model.md` ou `web-ui.md` inteiros.
6. Ao concluir, atualize o brief (se houver), os docs afetados e o changelog.

## Brief de tarefa

Antes de codar, crie ou atualize `docs/tasks/<RF>.md` a partir de [`docs/tasks/TEMPLATE.md`](docs/tasks/TEMPLATE.md), com no máximo ~60 linhas. O brief resume objetivo, contrato já fechado, invariantes, arquivos prováveis, testes e docs a atualizar; decisões em aberto ficam explícitas. Ele não substitui os docs de área.

## Regras inegociáveis

- Não invente contratos: schema, rotas e algoritmos são os documentados. Divergência exige atualizar a doc primeiro.
- `internal/plan` é puro e determinístico: sem I/O, sem relógio, sem `rand`, sem iteração de `map` para gerar ordem.
- Nunca quebre as invariantes `I-01`…`I-10` de `docs/context.md`.
- Toda mudança de comportamento atualiza a documentação e `docs/changelog.md` no mesmo commit.
- Decisão estrutural nova → ADR em `docs/adr/`; dúvida → `docs/open-questions.md`. Nada de `TODO` solto.
- Não logue corpo de leitura, tokens ou `APP_SECRET`.
- Sem dependência nova sem ADR.
- Preocupação com o uso de tokens: com o projeto usa intensivamente agentes de ia, o uso excessivo de tokens (principalmente pela documentação extensa existente) deve ser evitado a todo custo.

## Comandos

```sh
go test ./... -race
CGO_ENABLED=0 go test ./internal/plan/... ./internal/blocks/...
go run github.com/a-h/templ/cmd/templ@v0.3.1070 generate
```

## Definition of Done

Ver `docs/contributing.md`. Em resumo: requisito testado, testes verdes, docs atualizados, changelog, build ARM64 passando.
