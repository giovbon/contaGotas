---
title: Padrão de documentação
status: ativo
updated: 2026-10-30
---

# Padrão de documentação

Este documento explica **o padrão de documentação adotado no projeto**: de onde ele vem, como cada arquivo se encaixa, e qual nome usar ao se referir a ele.

## Resposta curta

Não existe **um** nome consagrado para o conjunto inteiro. O que está no repositório é uma **composição de padrões já estabelecidos**, montada com uma camada específica para agentes de IA. Se precisar de um rótulo para pesquisar ou explicar a terceiros, os mais próximos são:

| Rótulo | O que cobre | Onde está no projeto |
|---|---|---|
| **Docs-as-code** | documentação versionada, revisada e testada como código | tudo em `docs/`, front matter, links relativos |
| **ADR** (Architecture Decision Records) | uma decisão imutável por arquivo | `docs/adr/0001…0012` |
| **Spec-driven development** (spec-first) | requisito com ID e critério de aceite antes do código | `docs/requirements.md` (`RF-###`/`RNF-###`) |
| **Context engineering / repo-as-context** | empacotar o contexto mínimo para o agente não alucinar | `AGENTS.md` + `docs/context.md` |
| **Memory Bank** (padrão do Cline) | arquivos que funcionam como memória entre sessões | `context.md`, `changelog.md`, `roadmap.md`, `open-questions.md` |
| **Living documentation** | doc que evolui junto com o comportamento | regra “doc no mesmo commit” |
| **Definition of Done** (Scrum) | checklist de conclusão | `docs/contributing.md` |
| **Spike** (XP) | investigação com prazo e critério de saída | `docs/spikes.md` |
| **Golden / approval testing** | saída congelada como contrato de regressão | `testdata/golden/` |

O nome que melhor descreve este arranjo, se for necessário escolher um: **“documentação orientada a contexto para agentes”** — docs-as-code + ADRs + spec-driven, com uma camada explícita de contexto (`AGENTS.md` → `context.md`) desenhada para consumo por LLM.

## O que foi adotado, na prática

### 1. `AGENTS.md` como ponto de entrada (convenção `agents.md`)

É a convenção aberta (Codex, Jules, Cursor e afins) de um arquivo na raiz que instrui agentes. A decisão aqui foi **não duplicar conteúdo**: o arquivo só referencia, lê e define regras inegociáveis. Isso evita o clássico “dois lugares dizem coisas diferentes”.

### 2. `context.md` como “pacote de contexto” (Memory Bank)

Concentra o que um agente precisa saber **antes de qualquer ação**: o que é o produto, decisões fechadas (`D-01…D-10`), invariantes (`I-01…I-10`), estado atual e próximo passo. O teto de ~150 linhas é *context budget*: se crescer, o detalhe vai para outro arquivo. É exatamente o padrão **Memory Bank** — um arquivo de contexto central e arquivos satélites por tema.

### 3. ADRs (Michael Nygard, 2011)

`docs/adr/` implementa o formato canônico — contexto, decisão, alternativas, consequências (inclusive negativas), imutabilidade após aceite, substituição em vez de edição. É o registro que impede o agente de “reinventar” uma decisão já tomada.

### 4. Spec-driven development

`requirements.md` usa IDs estáveis (`RF-001`…), prioridade (M/S/C) e **critério de aceite verificável**. A regra “sem requisito, pare” força o fluxo spec → teste → código. É a mesma ideia do GitHub Spec Kit / Kiro, só que em Markdown puro.

### 5. IDs estáveis como “endereços citáveis”

`RF-###`, `ADR-####`, `Q-###`, `I-##`, `SPIKE-##`, `FASE-#`. Isso é o que permite humano e agente **citarem a mesma coisa** sem ambiguidade (“não quebre I-04”, “isso atende RF-034”). IDs nunca são reutilizados; requisito removido vira `status: removido`. É um detalhe pequeno com efeito grande em consistência.

### 6. Invariantes como “constituição”

`I-01…I-10` são restrições duras (nunca cortar por offset, nunca terminar chunk em heading, determinismo total etc.). Funcionam como *guardrails* contra o agente “melhorar” o algoritmo e quebrar contrato. É a versão documental do que em engenharia se chama **invariant catalog** / **architectural constraints**.

### 7. Append-only como memória episódica

`changelog.md` (mais recente no topo) e ADRs imutáveis dão ao agente **histórico do que já foi decidido e por quê**. Sem isso, cada sessão nova recomeça do zero e tende a reverter decisões.

### 8. `open-questions.md` em vez de `TODO` solto

Cada dúvida tem ID, impacto, status e prazo. Isso é gestão de risco explícita e evita a “dívida de TODO” que apodrece no código.

### 9. Um assunto, um arquivo (single source of truth)

`docs/README.md` define: “um assunto mora em um arquivo; os outros linkam”. É a regra que mantém a documentação **navegável e não redundante** — crucial para contexto de LLM, que degrada com duplicação.

### 10. `status` + `updated` no front matter

Permite detectar documento obsoleto e medir frescor. É a base de *living documentation*.

### 11. Leitura econômica e briefs de tarefa

`AGENTS.md` impõe um orçamento de leitura (~10 mil tokens antes do código) e manda ler só a seção afetada. `docs/tasks/` guarda briefs de ~60 linhas por tarefa, com o contrato fechado e as dúvidas explícitas; o brief acelera a sessão do agente sem virar uma segunda fonte de verdade. Em conflito, o doc de área sempre vence.

## O que isso **não** é (e o que falta para ficar “com nome”)

- **Não é Diátaxis** (tutorial/how-to/reference/explanation). Os docs são majoritariamente *reference* + *explanation*; falta a camada de tutorial.
- **Não é um “RFC process”** formal (estilo Rust/Python). O ADR cobre a decisão, mas não há período de comentário público.
- **Não é `llms.txt`** (proposta de índice legível por LLM). Existe o equivalente humano em `docs/README.md`.
- **Falta automação**: não há lint de doc (front matter, links quebrados, IDs duplicados, `context.md` acima de 150 linhas) no CI. Hoje a regra é honrada por disciplina do agente, não por teste.
- **Falta rastreabilidade requisito→teste→código** de forma verificável (ex.: `//go:test RF-034` ou tabela gerada).

## Em uma frase

O padrão adotado é **docs-as-code com ADRs e desenvolvimento guiado por especificação, embrulhado numa camada de contexto para agentes** (`AGENTS.md` + `context.md` + IDs estáveis + invariantes + changelog append-only). O nome de mercado mais próximo é **context engineering** ou **Memory Bank**; o nome descritivo deste arranjo é **documentação orientada a contexto para agentes**.

## Próximos passos sugeridos

1. Registrar esta decisão como `ADR-0013 — Documentação orientada a contexto`.
2. Adicionar lint de documentação no CI (front matter, links, IDs, teto do `context.md`).
3. Avaliar rastreabilidade requisito→teste→código.
