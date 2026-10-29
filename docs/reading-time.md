---
title: Tempo de leitura
status: ativo
updated: 2026-10-26
---

# Tempo de leitura

Estimativa determinística, calculada em Go, por bloco. Não usamos biblioteca externa: os parâmetros precisam ser por idioma, por tipo de bloco e calibráveis.

## Fórmula

```
base    = palavras × 60 / wpm
ajuste  = base × (difficulty − 1)
extras  = Σ custoFixo(kind) + custoVariável(kind)
seconds = round(base + ajuste + extras)
```

- `wpm`: padrão **200** para português não-ficção; 120–150 para técnico denso.
- `difficulty`: multiplicador por documento (padrão `1.0`).
- Novos documentos herdam `plan.wpm` e `plan.difficulty` de `/settings` no momento da importação; documentos já criados mantêm os valores gravados. Ver [`segmentation.md`](./segmentation.md#parâmetros).
- `round`: arredonda para o segundo; exibição em minutos usa `ceil` (nunca “0 min”).

## Custos fixos e variáveis

| Tipo (`kind`) | Fixo | Variável |
|---|---|---|
| `h1`–`h6` | 2 s | — |
| `p` | 0 | — |
| `li` | 0,5 s | — |
| `quote` | 1 s | — |
| `code` | 2 s | 1,5 s por linha |
| `table` | 3 s | 1 s por linha de dados (`<tr>` menos o cabeçalho) |
| `figure` | 12 s | — |
| `footnote` | 1 s | — |
| `hr` | 0 | — |

`difficulty` incide sobre a base de prosa. Código e figura têm base zero: o custo vem dos extras. Se o resultado for menor que 1 s e o bloco não for `hr`, arredonda para 1 s.

Justificativa: código e tabela exigem releitura; figura exige olhar; heading é transição.

## Contagem de palavras

- `words` = campos separados por espaço Unicode no texto normalizado.
- Markdown: usa o texto do nó (sem sintaxe), exceto código, cuja contagem é por linha.
- PDF: conta após junção de linhas e deshifenização.
- Não contar URLs, números de página nem marcações.

## Exemplos

10 minutos a 200 wpm:

| Conteúdo | Palavras | Extras | Total |
|---|---|---|---|
| Prosa pura | 2.000 | 0 | 10 min |
| 1 heading + 1 figura | 1.900 | 2 + 12 s | ~9,7 min |
| 40 linhas de código | 40 (linhas) | 2 + 60 s | ~1 min |
| Tabela com 10 palavras e 5 `<tr>` (1 cabeçalho + 4 linhas) | 10 | 3 + 4 s | 10 s |

## Calibração

Opcional, por documento:

1. Após cada entrega, a UI oferece “levou mais / menos tempo que o previsto”.
2. O feedback ajusta `difficulty` com média móvel exponencial:
   `difficulty ← 0,7 × difficulty + 0,3 × (tempoReal / tempoPrevisto)`.
3. Limitado a `[0,5 … 2,0]` para evitar distorções.
4. Só é aplicado ao **próximo plano** do documento; nunca altera chunks já enviados.

Sem feedback, `difficulty` permanece `1.0`.

## Onde é calculado

- `internal/blocks`: `Estimator.Estimate(kind, text, html) (words, seconds int)`.
- `internal/blocks.Prepare` aplica o estimador e grava `words`/`seconds` em cada bloco.
- A soma por nó da árvore é feita no `internal/plan`.
- O total do documento (`total_seconds`) será gravado em `documents`.
- O preview e a mensagem usam os mesmos números — não há recálculo divergente.

## Testes

| Teste | Entrada | Esperado |
|---|---|---|
| Prosa | 2.000 palavras, wpm 200 | 600 s |
| Arredondamento | 1 palavra, wpm 200 | 1 s (nunca 0) |
| Dificuldade | 2.000 palavras, wpm 200, difficulty 1,2 | 720 s |
| Código | 10 linhas | 2 + 15 = 17 s |
| Tabela | 10 palavras + 5 `<tr>` | 3 (base) + 3 + 4 = 10 s |
| Figura | 1 figura | 12 s |
| Unicode | texto com acentos e emoji | contagem estável |
