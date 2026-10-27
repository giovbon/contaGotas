---
title: ADR-0009 — Dois modos de ritmo
status: aceito
date: 2026-10-26
---

# ADR-0009 — Dois modos de ritmo

## Contexto

O usuário pode querer “ler 10 minutos por dia” (ritmo constante) ou “terminar este livro em 20 dias de leitura” (prazo). São objetivos diferentes e ambos fazem sentido.

## Decisão

O pacing é por documento e tem dois modos mutuamente exclusivos:

| Modo | Alvo do chunk | Semântica |
|---|---|---|
| `minutes_per_day` | `minutos × 60` | ritmo fixo; o término emerge |
| `finish_by_days` | `total_seconds ÷ dias` | horizonte de N **dias de leitura**; dias perdidos empurram o fim |

Ambos usam os mesmos `min_fill`/`max_overshoot` e o mesmo empacotamento. Mudar o modo em andamento oferece regenerar o plano a partir do cursor.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| Só minutos/dia | não atende “quero terminar até o fim do mês” |
| Só prazo | força trechos ruins em livros muito longos |
| Data-alvo no calendário | mais intuitivo, porém mistura dias corridos com dias de leitura; fica para depois |
| Modo global (não por documento) | livros de tamanhos diferentes precisam de ritmos diferentes |

## Consequências

### Positivas
- Cobre os dois casos sem dois algoritmos.
- `finish_by_days` distribui melhor o conteúdo e evita “último dia gigante”.
- Fácil de explicar na UI (“10 min/dia” vs “terminar em 20 leituras”).

### Negativas / riscos
- `finish_by_days` pode gerar chunks muito pequenos ou muito grandes se N for irreal.
- Mudar de modo no meio exige regeneração consciente.
- “Dias de leitura” pode ser confundido com “dias corridos”.

### Mitigações
- Avisos de sanidade (`target < 3 min`, `target > 60 min`) e na UI.
- Texto explícito: “N dias de leitura (dias da semana configurados)”.
- Regeneração preserva chunks já enviados.
