---
title: Continuidade da leitura
status: ativo
updated: 2026-10-26
---

# Continuidade

O problema central: o leitor recebe um trecho hoje, outro amanhã, e precisa retomar sem reler. A solução combina **estrutura** (onde cortamos), **texto de apoio** (recap e rodapé) e **contexto sob demanda** (leitor web).

## Anatomia da mensagem

```markdown
**Dom Casmurro** · Machado de Assis
`Cap. 32 — A carta · 32.4 Primeira desconfiança`
`1.842 palavras · ~9 min · 21% do livro`

> ↩︎ **Onde paramos:** “…e Capitu, sem responder, olhou para a
> janela como quem espera que a noite explicasse o dia.”

## 32.4 — Primeira desconfiança

<corpo do chunk>

---
⏭️ **Amanhã:** 32.5 — O jantar · continuação da mesma seção
🔗 [Abrir no leitor](https://leitura.exemplo.com/r/42?t=…)
```

Regras:

| Elemento | Quando aparece | Limite |
|---|---|---|
| Título do documento + autor | sempre | 1 linha |
| Breadcrumb (capítulo › seção) | sempre | 2 linhas |
| Métricas (palavras, minutos, %) | sempre | 1 linha |
| Recap | a partir do 2º chunk do documento | 2 frases / 240 caracteres |
| Heading repetido | `starts_mid_section=1` | texto do heading |
| Rodapé “amanhã” | `ends_mid_section=1` | 1 heading + 140 caracteres |
| Link do leitor | sempre | token assinado |
| Separador `---` | quando há mais de um documento na mensagem | — |

## Recap extrativo

Sem LLM, determinístico:

1. Pegar o texto do **último bloco de prosa** do chunk anterior (ignorar código, tabela, figura, footnote).
2. Extrair as **duas últimas frases completas**, respeitando o mesmo divisor de frases do segmentador.
3. Remover formatação Markdown (ênfase, links viram texto, imagens somem).
4. Truncar em 240 caracteres **em fronteira de palavra** e acrescentar `…` se cortado.
5. Se o resultado tiver menos de 40 caracteres, usar a última frase inteira mesmo que ultrapasse.
6. Prefixar com `↩︎ **Onde paramos:**`.

Casos de fallback:

| Situação | Recap |
|---|---|
| Chunk anterior terminou em código/tabela | usar o último parágrafo anterior a ele |
| Não há prosa no chunk anterior | omitir recap |
| Chunk anterior foi `skipped` | usar o último chunk efetivamente enviado |
| Documento recém-iniciado | omitir recap |

O recap é calculado no momento da composição e **não** é persistido no chunk (evita divergência se o conteúdo for reprocessado).

## Rodapé “continua”

Quando `ends_mid_section=1`:

```
⏭️ **Amanhã:** {próximo heading}
{primeira frase do próximo chunk, truncada em 140 caracteres}
```

Isso sinaliza que a pausa é intencional — não que o conteúdo acabou. Se `ends_mid_section=0`, o rodapé vira apenas `✓ Fim da seção` (ou do capítulo, se for o caso).

## Heading repetido

Se o chunk começa no meio de uma seção, o heading da seção é repetido como `##` antes do corpo (mesmo que o heading original seja `h3`). Objetivo: o leitor abrir a notificação e saber imediatamente onde está.

## Progresso

`pct = chunks enviados ÷ total de chunks`, arredondado para inteiro. No modo `finish_by_days`, o percentual usa o total de chunks do plano; não o tempo.

## Leitor web (contexto sob demanda)

`GET /r/{chunkID}?t={token}` mostra:

1. cabeçalho com documento, autor, capítulo, progresso;
2. recap do chunk anterior;
3. corpo do chunk renderizado;
4. **contexto expansível**: “ver seção completa” carrega todos os blocos daquela seção;
5. navegação: `← anterior` · `próximo →` · `índice do documento`;
6. ações: “marcar como lido”, “baixar markdown do chunk”.

O leitor é a rede de segurança: mesmo que o corpo da mensagem tenha sido truncado, o contexto completo está a um toque.

## Regras estruturais que protegem a continuidade

1. Nunca cortar em heading (o heading sempre abre o dia seguinte junto com o primeiro parágrafo). Exceção: heading órfão no fim do documento, sem conteúdo seguinte.
2. Preferir fim de seção; quando não der, terminar em fim de parágrafo.
3. Evitar dois cortes consecutivos no mesmo nível dentro da mesma seção.
4. Chunk seguinte a um corte entre blocos de um parágrafo pré-dividido **sempre** tem recap e heading repetido.
5. Nenhum chunk começa com pontuação de continuação (`.` `,` `;`) — validação no `Validate`.

## Exemplo de corte ruim vs. bom

**Ruim** (fim no meio do raciocínio, sem contexto):

```
... e por isso Capitu
```
→ amanhã começa com `não respondeu.` e o leitor não sabe quem, nem por quê.

**Bom** (fim de parágrafo + rodapé):

```
... e por isso Capitu não respondeu.
---
⏭️ Amanhã: 32.5 — O jantar (continuação)
```
→ amanhã abre com heading repetido + recap das duas últimas frases.

## Testes

| Teste | Verifica |
|---|---|
| Recap com Markdown | links/ênfase removidos, tamanho ≤ 240 |
| Recap com abreviações | não corta em `Sr.` / `etc.` |
| Rodapé só quando mid-section | flag consistente |
| Mensagem com 3 documentos | separadores e ordem de prioridade |
| Chunk 0 | sem recap |
| Chunk após `skipped` | recap do último enviado |
