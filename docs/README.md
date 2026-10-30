---
title: Documentação do Reading Drip
status: ativo
updated: 2026-10-30
---

# Reading Drip — documentação

Sistema self-hosted, single-binary, que transforma arquivos `.md`, `.pdf` e `.epub` em **doses diárias de leitura** entregues via ntfy. Cada documento é particionado em blocos coerentes e servido em pedaços que respeitam o tempo de leitura escolhido pelo usuário.

> **Leia primeiro:** [`context.md`](./context.md) — pacote de contexto com as decisões fechadas, invariantes e estado atual. Ele é a fonte de verdade para humanos e agentes.

---

## Mapa da documentação

| Arquivo | Responde |
|---|---|
| [`context.md`](./context.md) | O que é, decisões fechadas, invariantes, estado atual, próximos passos |
| [`glossary.md`](./glossary.md) | Vocabulário do domínio (bloco, chunk, plano, pacing, run…) |
| [`requirements.md`](./requirements.md) | Requisitos funcionais e não funcionais com IDs e critérios de aceite |
| [`architecture.md`](./architecture.md) | Componentes, fluxo, estrutura de pastas, dependências, orçamento de recursos |
| [`data-model.md`](./data-model.md) | Schema SQLite, invariantes, índices, migrações, retenção |
| [`segmentation.md`](./segmentation.md) | Algoritmo de particionamento (o núcleo do produto) |
| [`reading-time.md`](./reading-time.md) | Estimativa de tempo de leitura e calibração |
| [`extraction.md`](./extraction.md) | Pipelines de extração MD / EPUB / PDF |
| [`continuity.md`](./continuity.md) | Como preservar o fio da leitura entre dias |
| [`delivery-ntfy.md`](./delivery-ntfy.md) | Entrega ntfy self-hosted, payloads, retry, idempotência |
| [`scheduling.md`](./scheduling.md) | Agendador, fuso/DST, catch-up, paralelismo |
| [`web-ui.md`](./web-ui.md) | Telas, rotas, componentes templ, CSS/JS e autenticação |
| [`operations.md`](./operations.md) | Build ARM64, container, backup, upgrade, troubleshooting |
| [`testing.md`](./testing.md) | Estratégia de testes, golden files, fixtures, benchmarks |
| [`spikes.md`](./spikes.md) | Investigações técnicas com prazo e critério de saída |
| [`contributing.md`](./contributing.md) | Fluxo de trabalho para humanos e agentes, checklists |
| [`documentation-pattern.md`](./documentation-pattern.md) | Padrão de documentação adotado (docs-as-code, ADR, contexto para agentes) |
| [`roadmap.md`](./roadmap.md) | Fases, critérios de saída, status |
| [`open-questions.md`](./open-questions.md) | Decisões pendentes com ID, impacto e bloqueio |
| [`changelog.md`](./changelog.md) | Histórico incremental (append-only) |
| [`tasks/`](./tasks/README.md) | Briefs curtos por tarefa (leitura econômica para agentes) |
| [`adr/`](./adr/README.md) | Architecture Decision Records — uma decisão por arquivo |

---

## Leitura econômica (para agentes)

O caminho padrão de uma tarefa é curto. **Não leia a documentação inteira.**

1. [`context.md`](./context.md) — decisões, invariantes e estado.
2. O brief da tarefa em [`tasks/`](./tasks/README.md), quando existir.
3. Apenas a seção citada da área afetada (mapa abaixo).
4. O requisito `RF-###` correspondente em [`requirements.md`](./requirements.md).
5. O checklist final em [`contributing.md`](./contributing.md).

| Tarefa | Leitura mínima |
|---|---|
| Núcleo / segmentação | [`segmentation.md`](./segmentation.md#parâmetros), [`reading-time.md`](./reading-time.md), [`testing.md`](./testing.md#golden-files) |
| Extração | [`extraction.md`](./extraction.md), [`testing.md`](./testing.md#epub), [`testing.md`](./testing.md#pdf) |
| Entrega / agenda | [`delivery-ntfy.md`](./delivery-ntfy.md), [`scheduling.md`](./scheduling.md), [`continuity.md`](./continuity.md) |
| Web / UI | [`web-ui.md`](./web-ui.md#rotas) + a tela afetada, [`testing.md`](./testing.md#http) |
| Store / dados | [`data-model.md`](./data-model.md#schema), [`data-model.md`](./data-model.md#migrações), [`testing.md`](./testing.md#store) |
| Operação / deploy | [`operations.md`](./operations.md#imagem-dockerfile), [`operations.md`](./operations.md#compose-composeyaml), [`testing.md`](./testing.md#ci) |
| Documentação | [`contributing.md`](./contributing.md), [`documentation-pattern.md`](./documentation-pattern.md) |

Regras:

- leia só a seção; `rg -n '^## |^### ' docs/<arquivo>.md` lista os títulos;
- no [`changelog.md`](./changelog.md), leia apenas a entrada mais recente;
- não carregue `testing.md`, `requirements.md`, `data-model.md` ou `web-ui.md` inteiros.

## Trilha completa (onboarding e revisão de arquitetura)

Só para entender o sistema de ponta a ponta:

1. [`context.md`](./context.md)
2. [`requirements.md`](./requirements.md)
3. [`architecture.md`](./architecture.md) + [`data-model.md`](./data-model.md)
4. [`contributing.md`](./contributing.md)

---

## Convenções

### Front matter obrigatório

Todo documento começa com:

```yaml
---
title: Nome curto
status: rascunho | ativo | obsoleto
updated: AAAA-MM-DD
---
```

`status: obsoleto` significa: não use como referência; siga o link indicado no topo do arquivo.

ADRs usam `date` em vez de `updated` e são imutáveis após aceite (ver [`adr/README.md`](./adr/README.md)).

O arquivo [`../AGENTS.md`](../AGENTS.md) é o ponto de entrada para ferramentas de IA; ele só aponta para esta pasta, sem duplicar conteúdo.

### IDs estáveis

| Prefixo | Uso | Exemplo |
|---|---|---|
| `RF-###` / `RNF-###` | Requisitos | `RF-014` |
| `ADR-####` | Decisão arquitetural (imutável após aceite) | `ADR-0003` |
| `Q-###` | Pergunta em aberto | `Q-004` |
| `I-##` | Invariante do sistema | `I-03` |
| `SPIKE-##` | Investigação técnica com prazo | `SPIKE-01` |
| `FASE-#` | Fase do roadmap | `FASE-0` |

IDs **nunca são reutilizados**. Requisito removido vira `status: removido` na tabela, não desaparece.

### Regras de manutenção

1. **Fonte única.** Um assunto mora em um arquivo; os outros linkam. Não copie tabelas de schema nem algoritmos.
2. **Decisão não trivial → ADR.** Se a mudança altera contrato, formato persistido, dependência ou operação, crie `adr/ADR-####-slug.md` e marque o anterior como `substituído`.
3. **Comportamento novo → requisito.** Toda feature tem `RF-###` com critério de aceite verificável.
4. **Dúvida sem resposta → `open-questions.md`.** Não deixe `TODO` solto no meio de um doc.
5. **Toda fase concluída → entrada no `changelog.md`** (mais recente no topo).
6. **`context.md` tem teto de ~150 linhas.** Se crescer, o detalhe pertence a outro arquivo.
7. **Links relativos**, sempre. Nada de URL absoluta para arquivos do repositório.
8. **Datas em ISO 8601** (`2026-10-26`). Fuso de exemplo: `America/Sao_Paulo`.

### Quando atualizar o quê

| Mudança | Atualizar |
|---|---|
| Novo requisito / mudança de escopo | `requirements.md` + `changelog.md` |
| Mudança de algoritmo de corte | `segmentation.md` (+ ADR se mudar contrato) |
| Mudança de schema | `data-model.md` + migração + ADR se destrutiva |
| Novo formato de arquivo | `extraction.md` + `requirements.md` |
| Mudança no payload ntfy | `delivery-ntfy.md` |
| Nova tela / rota | `web-ui.md` |
| Mudança de build, base image, limites | `operations.md` |
| Decisão pendente resolvida | `open-questions.md` (fechar com data) + ADR se relevante |
| Fase concluída | `roadmap.md` (status) + `changelog.md` |

### Definição de pronto (documentação)

Uma entrega só é considerada pronta se:

- [ ] o comportamento está descrito no doc da área;
- [ ] os requisitos afetados têm critério de aceite atendido;
- [ ] decisões novas têm ADR (ou estão registradas em `open-questions.md`);
- [ ] `changelog.md` recebeu entrada;
- [ ] links novos foram verificados;
- [ ] `context.md` continua verdadeiro (decisões e estado).

---

## Estrutura planejada do repositório

```
docs/                  # esta documentação
cmd/app/               # entrypoint HTTP (wiring + shutdown)
cmd/readingdrip/       # CLI headless (FASE-0)
cmd/spike-pdf/         # spike de extração PDF
internal/
  docs/                # detecção + extratores md/epub/pdf
  docs/markdown/       # extrator Markdown
  blocks/              # modelo + tempo de leitura
  plan/                # segmentador (puro, sem I/O)
  delivery/            # composição da mensagem + cliente ntfy
  schedule/            # ticker, calendário, idempotência
  store/               # SQLite + migrações
  web/                 # handlers, templ, assets embutidos
web/                   # fontes templ/tailwind/alpine
testdata/              # fixtures e golden files
```

O nome do produto é provisório (`Q-001`).
