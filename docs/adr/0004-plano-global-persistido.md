---
title: ADR-0004 — Plano global persistido
status: aceito
date: 2026-10-26
---

# ADR-0004 — Plano global persistido

## Contexto

Duas abordagens possíveis: (a) decidir o corte do dia na hora da entrega, olhando apenas o restante; (b) gerar o plano inteiro na importação e consumi-lo como uma fila.

O corte incremental tende a acumular erro (últimos dias enormes), não permite preview, dificulta edição e complica restart/catch-up.

## Decisão

Gerar o plano completo no momento da importação (ou de uma regeneração explícita) e persisti-lo:

- `plans` versionado, um `active` por documento;
- `chunks` com `first/last_block_id` + hashes;
- consumo por cursor (`menor seq pending`);
- mudanças de pacing geram **nova versão**, preservando chunks enviados;
- `plan.Generate` é função pura e determinística.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| Cálculo no momento do envio | sem preview, sujeito a drift, difícil de testar |
| Plano em memória | perdido no restart; sem histórico |
| Plano no arquivo original | não edita fronteiras nem versiona |
| Recalcular a cada dia | muda o conteúdo já prometido ao leitor |

## Consequências

### Positivas
- Preview e edição antes de começar.
- Determinismo testável com golden files.
- Restart/catch-up triviais: a fila está no banco.
- Mudar dias da semana não afeta o conteúdo.

### Negativas / riscos
- O plano ocupa espaço (uma linha por chunk, não por bloco).
- Mudanças de configuração exigem regeneração consciente.
- Edição manual pode violar invariantes se não validada.

### Mitigações
- `Validate` obrigatório antes de ativar.
- Edição só em chunks `pending`.
- `params_json` gravado para auditoria.
- Regeneração sempre “a partir do cursor”.
