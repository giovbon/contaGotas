---
title: Segmentação
status: ativo
updated: 2026-10-28
---

# Segmentação

O núcleo do produto. Define **onde** um documento pode ser cortado para virar doses diárias sem quebrar o fio da leitura.

## Princípio

Corta-se entre **blocos**, não entre caracteres. Um chunk é:

```go
type Chunk struct {
    Seq              int
    FirstBlockID     int64
    LastBlockID      int64
    FirstHash        string   // hash do primeiro bloco
    LastHash         string   // hash do último bloco
    SectionPath      []string // breadcrumb
    StartsMidSection bool
    EndsMidSection   bool
    Oversized        bool
    Words            int
    Seconds          int
    TargetSeconds    int
}
```

As pontas por hash permitem detectar plano obsoleto se o documento for reprocessado (conteúdo mudou) sem depender de offsets.

## Árvore

A partir dos headings:

```
Documento
├── Cap. 1 (h1)
│   ├── 1.1 (h2)
│   │   ├── 1.1.1 (h3) → blocos
│   │   └── blocos
│   └── blocos soltos da seção 1
└── Cap. 2 (h1) …
```

Regras de construção:

1. Heading de nível maior abre nó filho do último heading de nível menor.
2. Heading é o **primeiro bloco** do nó; nunca fica órfão no fim de um chunk.
3. Conteúdo antes do primeiro heading forma um nó raiz sintético (`Front matter`).
4. Um nó é **divisível** quando tem mais de um item (filhos ou blocos diretos) ou quando seu único item é um nó divisível. Código, tabela, lista, citação, figura e footnote são indivisíveis.

## Tempo

Cada nó tem `Seconds = Σ blocos`. Fórmula em [`reading-time.md`](./reading-time.md).

## Parâmetros

| Parâmetro | Padrão | Efeito |
|---|---|---|
| `target` | modo A: `minutos × 60`; modo B: `total ÷ dias` | duração pretendida |
| `min_fill` | `0.60` | abaixo disso, evita fechar chunk |
| `max_overshoot` | `1.40` | acima disso, evita abrir chunk |
| `section_alignment` | `true` | reservado; o empacotamento já preserva seções inteiras quando cabem no alvo |
| `max_para_words` | `300` | limite de normalização: parágrafos maiores viram vários blocos `p` (não é parâmetro do plano) |

Derivados: `min = target × min_fill`, `max = target × max_overshoot`.

O corte de parágrafos longos acontece em [`blocks.Prepare`](../internal/blocks/normalize.go), não no segmentador — ver [ADR-0011](./adr/0011-paragrafos-longos-em-blocos.md).


## Algoritmo

### 1. Unidades

A árvore é achatada em **unidades** na ordem do documento:

- nó com `Seconds <= target` → uma unidade inteira (preserva a seção);
- nó com `Seconds > target` e divisível → desce um nível; o heading do nó vira uma unidade própria;
- bloco solto → uma unidade.

Headings isolados são colados ao próximo conteúdo (`mergeHeadings`). Se uma unidade termina em heading — por exemplo, uma subseção vazia no fim de um capítulo — o heading é transportado para a unidade seguinte (`carryTrailingHeadings`). Assim nenhum chunk termina em heading, exceto quando o heading é o último bloco do documento.

Um nó que caberia em `max` mas não no alvo pode ser subdividido para permitir um encaixe melhor. A seção só é preservada quando cabe no alvo.

### 2. Empacotamento

```go
for _, u := range units {
    // Bloco indivisível maior que max: chunk próprio, marcado oversized.
    if u.oversized {
        flush()
        out = append(out, []unit{u})
        continue
    }

    // Adicionar estouraria o máximo e o chunk atual já é aceitável.
    if len(cur) > 0 && curSeconds+u.Seconds > max && curSeconds >= min {
        flush()
    }

    cur = append(cur, u)
    curSeconds += u.Seconds

    if curSeconds >= target {
        flush()
    }
}
flush()
```

Propriedades:
- nunca corta dentro de um bloco;
- se `cur < min` e a próxima unidade não cabe em `max`, a unidade é incluída mesmo assim. O chunk resultante ultrapassa `max` e é marcado `oversized`: é o preço de fechar uma seção inteira em vez de emitir uma dose faminta;
- um bloco indivisível que sozinho excede `max` também é `oversized` e não é cortado.

### 3. Balanceamento

O empacotamento guloso pode gerar `3 min + 17 min` quando um nó de 17 min segue um de 3 min. Uma passada linear move **unidades inteiras** entre grupos vizinhos:

```go
for i := 1; i < len(groups); i++ {
    a, b := groups[i-1], groups[i]

    // a está curto: puxa unidades de b até chegar perto do mínimo.
    for sum(a) < min && len(b) > 1 {
        u := b[0]
        if sum(a)+u.Seconds > max || sum(b)-u.Seconds < min {
            break
        }
        moveFirst(b, a)
    }

    // a está longo e b curto: empurra unidades de a até estabilizar.
    for sum(a) > target && sum(b) < min && len(a) > 1 {
        u := a[len(a)-1]
        if sum(b)+u.Seconds > max {
            break
        }
        moveLast(a, b)
    }
}
```

O balanceamento não cria cortes novos e não altera a cobertura.

### 4. Parágrafos longos

Parágrafos com mais de `max_para_words` palavras são **pré-divididos na normalização** (`blocks.Prepare`) em vários blocos `p`, sempre em fronteira de frase:

1. `SplitSentences` quebra após `.!?…` seguido de espaço e início de frase;
2. abreviações comuns (`Sr.`, `Dra.`, `etc.`, `pág.`, `art.`, `cap.`, `Fig.`, `ex.`) e iniciais (`J.`) não quebram;
3. decimais (`3.14`) não quebram;
4. uma frase sozinha maior que o limite permanece inteira.

O segmentador nunca corta dentro de um bloco. Se uma única frase ainda exceder `max`, o bloco é indivisível na prática e o chunk vira `oversized`. O próximo bloco de um parágrafo pré-dividido começa com frase completa; `starts_mid_section` indica que a dose começa fora de um heading.

### 5. Materialização

Cada grupo vira um `Chunk` com:
- `first_block` / `last_block` (índices; no banco, IDs) e hashes das pontas;
- `section_path` = caminho de headings do **primeiro** bloco;
- `starts_mid_section = primeiro bloco não é heading e não é o bloco 0`;
- `ends_mid_section = existe bloco seguinte e ele não é heading`;
- `oversized = segundos > max`;
- `target_seconds` do plano.

`ends_mid_section` alimenta o rodapé “continua amanhã”; `starts_mid_section` alimenta o breadcrumb repetido e o recap.


## Modos de ritmo

### A — `minutes_per_day`

```
target = pacing_minutes × 60
```

Número de chunks emerge do empacotamento. Exemplo: 100 min de conteúdo, 10 min/dia → ~10 chunks (pode variar ±1 por alinhamento de seção).

### B — `finish_by_days`

```
target = total_seconds ÷ pacing_days
```

`pacing_days` é o número de **dias de leitura** (não dias corridos). O plano nasce com esse horizonte; dias perdidos empurram o término no calendário, mas não re-cortam o conteúdo.

Limites de sanidade (avisos, não erros):

| Condição | Aviso |
|---|---|
| `target < 180 s` | “Alvo baixo demais para este conteúdo; leitura fragmentada” |
| `target > 3600 s` | “Alvo alto demais; cada dia terá mais de 1 h” |
| `pacing_days > número de chunks possível` | “Sobram dias sem conteúdo” |

Os avisos são derivados por `Params.Warnings(target, chunks)`, uma função pura que não altera o plano nem o status. Aparecem como `# aviso:` no CLI, em “Ritmo inviável” no detalhe e no plano completo; o plano continua válido (RF-026).

### Mudança de pacing

Ao alterar o ritmo com o documento em andamento, o sistema oferece **regenerar a partir do cursor**: os chunks já enviados ficam no plano antigo (`superseded`), e uma nova versão cobre de `first_block_id` do cursor até o fim, com o tempo restante. Nunca se reescreve chunk enviado.

## Fronteiras e custos

Referência para a otimização futura (`Q-012`). A implementação atual (gulosa + balanceamento) preserva nós inteiros e não usa pontuação de fronteira para escolher entre dois cortes válidos.


| Fronteira | Custo | Observação |
|---|---|---|
| Fim de capítulo (h1) | 0 | melhor |
| Fim de seção (h2/h3+) | 1 | ótimo |
| Fim de parágrafo | 3 | aceitável |
| Fim de frase dentro de parágrafo | 8 | só forçado |
| Dentro de bloco indivisível | ∞ | proibido |
| Antes/depois de heading (isolado) | ∞ | proibido |

Desempate futuro: menor custo; persistindo, maior duração ≤ `max`; persistindo, menor `seq`.

## Casos de borda

| Caso | Comportamento |
|---|---|
| Documento menor que `target` | 1 chunk único |
| Capítulo de 3× o alvo | desce para seções; se necessário, parágrafos |
| Parágrafo de 3.000 palavras | pré-dividido em blocos de até 300 palavras; o chunk corta entre eles |
| Frase sozinha maior que `max` | bloco indivisível na prática; chunk `oversized` |
| Tabela/código maior que `max` | chunk `oversized`, aviso no preview |
| Sem headings | árvore plana; corta em parágrafos |
| Heading sem conteúdo (seguido de outro heading) | transportado para o próximo conteúdo; nunca fica no fim |
| Heading como último bloco do arquivo | exceção: o último chunk pode terminar nele, pois não há conteúdo seguinte |
| Notas de rodapé (FASE-0) | um bloco `footnote` no fim do documento; ligação por referência fica para a FASE-1 |
| `pacing_days` inviável | plano gerado mesmo assim + aviso na UI/CLI (RF-026) |
| Plano antigo com bloco removido | validação de hashes marca `stale` e impede entrega |

## Determinismo e versionamento

- `plan.Generate(blocks, params)` é pura: sem `time.Now`, sem `rand`, sem mapas iterados.
- Ordenações sempre explícitas por `(ord)`, `(seq)`.
- `params_json` gravado no plano.
- Reprocessar o mesmo arquivo gera os mesmos hashes e o mesmo plano.
- Alterar `min_fill`/`max_overshoot`/pacing **gera nova versão**, nunca muta a ativa.

## Validação pós-geração

```go
func Validate(blocks []Block, chunks []Chunk, params Params) error {
    // 1. cobertura total, sem gap/overlap
    // 2. first/last hashes conferem
    // 3. nenhum chunk termina em heading, exceto heading final do documento
    // 4. todo chunk tem >= 1 bloco
    // 5. seconds <= max, exceto oversized; chunks curtos são permitidos quando inevitáveis
    // 6. seq contíguo a partir de 0
    // 7. flags starts/ends_mid_section coerentes com os blocos vizinhos
}
```

Se a validação falhar, o plano **não** é ativado e o documento fica `failed` com o motivo.

## Testes obrigatórios

Ver [`testing.md`](./testing.md). Resumo:

- golden files por fixture (MD com headings, MD plano, parágrafo longo, bloco oversized; EPUB e PDF entram nas próximas etapas);
- propriedades: determinismo, cobertura, não-corte de bloco, heading não-final, faixa de duração;
- teste de ambos os modos com o mesmo conteúdo;
- tabela de casos de borda acima, um teste por linha.

## Trabalho futuro

Otimização global por programação dinâmica sobre as fronteiras permitidas, minimizando `Σ custo + λ·Σ|duração − alvo|`. O resultado atual (guloso + balanceamento) é determinístico e explicável; o DP fica para quando houver evidência de que a distribuição não é boa o suficiente (`Q-012`).
