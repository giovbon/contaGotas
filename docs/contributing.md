---
title: Contribuição
status: ativo
updated: 2026-10-30
---

# Contribuição

Regras para humanos e agentes de IA. O objetivo é manter o repositório previsível: mudança pequena, testada e documentada.

## Antes de escrever código

1. Leia [`context.md`](./context.md).
2. Leia o brief da tarefa em [`tasks/`](./tasks/README.md), se existir; senão, só a seção do doc de área.
3. Identifique o requisito (`RF-###`) ou a decisão (`ADR-####`) que a mudança atende.
4. Se não existir, **pare**: crie/atualize o requisito e, se for decisão estrutural, um ADR.
5. Verifique [`open-questions.md`](./open-questions.md): a dúvida já tem resposta?

## Fluxo

```
main
 └── feat/<slug>        código + testes + docs
 └── fix/<slug>         correção + teste de regressão
 └── docs/<slug>        somente documentação
```

Commits no imperativo, escopo curto:

```
feat(plan): adiciona balanceamento entre chunks
fix(ntfy): respeita Retry-After em 429
docs(adr): registra exposição via Cloudflare
test(plan): cobre corte por frase em parágrafo longo
```

## Regras de código

### Gerais

- `go vet ./...` limpo; `gofmt` aplicado.
- Erros sempre com contexto: `fmt.Errorf("extrair pdf %q: %w", name, err)`.
- Sem `panic` fora de `main`/inicialização.
- Sem `time.Now()` fora de `internal/platform`.
- Sem `math/rand` em `plan`; determinismo é invariante.
- Sem dependência nova sem ADR.
- Sem `TODO` no código: vira issue ou `Q-###`.

### Segmentador (`internal/plan`)

- Função pura, sem I/O, sem relógio.
- Toda ordenação explícita; nunca iterar `map` para gerar saída.
- Toda mudança de comportamento exige golden file atualizado **e** justificativa no PR.
- Nunca quebrar `I-01`…`I-05` de [`context.md`](./context.md).

### Extração (`internal/docs`)

- Cada formato em seu pacote, atrás da interface `Extractor`.
- PDF só compila com `cgo`; stub obrigatório para `CGO_ENABLED=0`.
- Nenhuma chamada de rede.
- Timeout de 5 min por extração, via `context`.

### Store (`internal/store`)

- Toda query em função do repositório; handler não escreve SQL.
- Migração nova = arquivo novo; nunca editar publicada.
- Toda escrita multi-tabela em transação.
- Datas sempre com fuso explícito.

### Web (`internal/web`)

- Handler fino: parse → valida → chama serviço → renderiza.
- Nenhum segredo em template ou `x-data`.
- HTML sempre via templ (nunca `template.HTML` de conteúdo não sanitizado).
- `Cache-Control: no-store` em HTML autenticado.

## Checklist por tipo de mudança

### Nova feature

- [ ] `RF-###` com critério de aceite
- [ ] teste unitário/integração
- [ ] doc da área atualizado
- [ ] `changelog.md`
- [ ] UI: estado vazio, erro e loading

### Mudança de algoritmo de corte

- [ ] golden files atualizados com justificativa
- [ ] testes de propriedade verdes
- [ ] `segmentation.md` atualizado
- [ ] comparação antes/depois de distribuição de minutos no PR

### Mudança de schema

- [ ] migração `NNNN_*.sql`
- [ ] `data-model.md`
- [ ] teste de migração com dados existentes
- [ ] plano de backup mencionado
- [ ] ADR se destrutiva

### Mudança de entrega ntfy

- [ ] `delivery-ntfy.md`
- [ ] teste com `httptest` (sucesso, 5xx, 4xx, limite de bytes)
- [ ] nada de token/corpo em log
- [ ] teste manual no celular

### Mudança de build/deploy

- [ ] `operations.md`
- [ ] build arm64 no CI
- [ ] healthcheck funcionando
- [ ] tamanho da imagem medido

## Documentação

Toda mudança de comportamento atualiza o doc **no mesmo commit**. Atualização de doc não é “PR separado”.

- decisão → ADR imutável (novo arquivo);
- requisito → `requirements.md`;
- dúvida → `open-questions.md`;
- fase → `roadmap.md` + `changelog.md`;
- `context.md` só muda se uma decisão/invariante mudou.

## Definition of Done

- [ ] requisito atendido e testado
- [ ] `go test ./...` e `-race` verdes
- [ ] `CGO_ENABLED=0 go test ./internal/plan/...` verde
- [ ] docs atualizados
- [ ] changelog atualizado
- [ ] nenhum segredo em log, código ou fixture
- [ ] build arm64 passa
- [ ] mudanças de UI verificadas em 360 px

## O que não fazer

- Copiar conteúdo entre docs (duplicação apodrece).
- Renumerar IDs existentes.
- Adicionar biblioteca para resolver 20 linhas de código.
- Colocar regra de negócio no handler.
- Usar `context.Background()` em request path.
- Enviar chunk já enviado novamente por “consistência”.
- Expor token do ntfy, `APP_SECRET` ou corpo de leitura em log/erro.

## Para agentes de IA

Instruções específicas:

1. **Não invente contratos.** Schema, rotas e algoritmos são os documentados; divergência exige atualizar o doc primeiro.
2. **Leia o arquivo antes de editar.** Não reescreva arquivos inteiros por conveniência.
3. **Mudança mínima.** Não refatore o entorno sem pedido.
4. **Rode os testes que tocam o pacote** antes de concluir.
5. **Se um teste falhar por comportamento esperado novo**, atualize o golden e explique; se falhar por bug, corrija o código.
6. **Registre decisões.** Se tomou uma decisão não documentada, ela precisa ir para um ADR ou `open-questions.md`.
7. **Nunca apague histórico**: entradas de changelog e ADRs são append-only.
8. **Idioma**: documentação e UI em pt-BR; código e identificadores em inglês.
9. **Leitura econômica**: leia `context.md` + brief e só as seções citadas; não abra docs inteiros.
