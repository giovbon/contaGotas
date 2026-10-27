---
title: Testes
status: ativo
updated: 2026-10-26
---

# Testes

O risco do projeto está concentrado no **segmentador** e na **extração**. A estratégia prioriza esses dois com testes determinísticos e fixtures reais; o resto usa testes de integração com dublês.

## Pirâmide

| Nível | Escopo | Ferramenta |
|---|---|---|
| Unitário | blocos, tempo, fronteiras, recap, calendário | `testing` |
| Golden | extração e plano completo | `testdata/golden` |
| Propriedade | invariantes do plano | `testing/quick` ou gerador próprio |
| Integração | store + HTTP + ntfy | `httptest`, SQLite em `t.TempDir()` |
| E2E manual | fluxo completo com ntfy real | checklist por release |

## Comandos

```sh
go test ./...                      # tudo (CGO ligado)
CGO_ENABLED=0 go test ./internal/plan/... ./internal/blocks/...
go test ./internal/plan -run TestGolden -update   # regenera golden
go test ./... -race
go test ./... -coverprofile=cover.out && go tool cover -func=cover.out
go test ./internal/plan -bench . -benchmem
```

## Golden files

Formato determinístico e legível:

```
testdata/golden/headings.plan.txt
```

```
# doc: testdata/md/headings.md  mode=minutes_per_day  target=120s  min=72s  max=168s
000  [0..14]  2m22s       397w  A arte de ler devagar  start=0 end=0
001  [15..27] 2m18s       370w  A arte de ler devagar › Capítulo 1 — O começo › 1.2 Um exemplo concreto  start=0 end=0
# total: 2 chunks, 4m40s, coverage=ok
```

Regras:
- caminho e parâmetros no cabeçalho;
- uma linha por chunk com `seq`, intervalo, duração, palavras, breadcrumb, flags e `oversized=1` quando aplicável;
- rodapé com totais e validação;
- `-update` só em PR que explique a mudança de comportamento.

Casos obrigatórios:

| Golden | Fixture | Cobre | Estado |
|---|---|---|---|
| `headings` | `md/headings.md` | hierarquia, listas, código, tabela, figura | implementado |
| `plain` | `md/plain.md` | sem headings | implementado |
| `long-para` | `md/long-para.md` | pré-divisão de parágrafo | implementado |
| `oversized` | `md/oversized.md` | código maior que o máximo | implementado |
| `headings-days` | `md/headings.md` | modo `finish_by_days` | implementado |
| `epub` | `epub/livro.epub` | spine, sanitização, listas, código, tabela, figura | implementado |
| `pdf-coluna-unica` | `pdf/coluna-unica.pdf` | headings por fonte | planejado |
| `pdf-duas-colunas` | `pdf/duas-colunas.pdf` | ordem de leitura | planejado |
| `parallel` | dois docs | composição da mensagem | planejado |

## Testes de propriedade

Gerar florestas de nós aleatórias (tamanho, profundidade, tipos) e verificar:

| Propriedade | Invariante |
|---|---|
| Cobertura | todo bloco em exatamente um chunk |
| Contiguidade | `first[n+1] = last[n] + 1` |
| Sem heading final | `kind(last) != h*`, exceto heading como último bloco do documento |
| Indivisível | nenhum corte dentro de `code/table/li/quote/figure` |
| Faixa | `dur <= max` exceto `oversized`; chunks curtos são permitidos quando inevitáveis |
| Flags | `starts_mid_section` / `ends_mid_section` coerentes com os blocos vizinhos |
| Hashes | `first_hash`/`last_hash` conferem com os blocos |
| Determinismo | gerar duas vezes → bytes iguais |
| Estabilidade | reordenar mapas internos não muda o resultado |

Semântica: se uma propriedade falhar, o teste imprime o plano completo em formato golden para reprodução.

## EPUB

`internal/docs/epub` cobre:

- container/OPF/`spine`, `linear="no"`, itens `nav` e mídia não XHTML;
- título/autor/idioma e fallback de título;
- headings, parágrafos, listas, código, citação, tabela, figura, `hr` e nota;
- links seguros, `rel="noopener noreferrer"` e remoção de `script`/`style`/`on*`;
- ZIP inválido, container/OPF ausente, spine vazio, zero blocos e `href` fora do pacote.

A fixture `testdata/epub/livro.epub` é determinística e pode ser regenerada com:

```sh
go run testdata/epub/generate.go
```

## Determinismo

- `plan.Generate` não recebe `time.Time` nem `rand`.
- Proibido iterar `map` para produzir ordem; sempre ordenar.
- Teste `TestDeterminism` roda 100 vezes em processo e compara hashes.

## Store

- Banco em `t.TempDir()` com migrações reais (não mock).
- Teste de migração: criar schema antigo, aplicar migrações, validar dados.
- `PRAGMA integrity_check` ao final de cada teste.
- Concorrência: 10 goroutines criando runs no mesmo dia → exatamente 1 linha.

## HTTP

- `httptest.NewServer` com router real.
- Upload multipart com arquivo de fixture.
- Sessão: cookie assinado, expiração, CSRF via `SameSite` + token em formulário.
- Token de leitor: válido, expirado, escopo errado, assinatura adulterada.
- Limite de upload: 413.
- Headers de segurança presentes.

## ntfy

Dublê com `httptest.Server` que captura o JSON:

```go
type capture struct {
    Topic    string   `json:"topic"`
    Title    string   `json:"title"`
    Message  string   `json:"message"`
    Markdown bool     `json:"markdown"`
    Click    string   `json:"click"`
    Actions  []Action `json:"actions"`
}
```

Cenários: sucesso, 5xx, 429 com `Retry-After`, 401, corpo grande, múltiplos documentos, truncamento.

## Relógio

`platform.Clock` falso controla o ticker. Nenhum teste depende do relógio real.

## Fixtures e licença

- Textos de domínio público (Machado de Assis, Eça de Queirós) ou gerados.
- EPUB criado por script (`testdata/gen`) para ser reprodutível.
- PDFs gerados com LaTeX/Typst a partir dos mesmos textos.
- Nenhum arquivo com direitos autorais no repositório.
- Tamanho total de `testdata` < 15 MiB.

## Benchmarks

```go
func BenchmarkPlan10kBlocks(b *testing.B)
func BenchmarkExtractPDF300p(b *testing.B)
func BenchmarkComposeMessage5Docs(b *testing.B)
```

Metas: plano de 10k blocos < 50 ms; composição < 5 ms. Medir alocação (`-benchmem`) e travar regressões no CI.

## Cobertura

| Pacote | Meta |
|---|---|
| `internal/plan` | ≥ 90% |
| `internal/blocks` | ≥ 85% |
| `internal/docs` | ≥ 75% |
| `internal/store` | ≥ 80% |
| `internal/delivery` | ≥ 80% |
| `internal/schedule` | ≥ 85% |
| `internal/web` | ≥ 60% |

Cobertura é piso, não meta: o que importa são os cenários de borda de [`segmentation.md`](./segmentation.md).

## CI

```yaml
jobs:
  test:
    steps:
      - go test ./... -race
      - go vet ./...
      - CGO_ENABLED=0 go test ./internal/plan/... ./internal/blocks/...
      - go test ./... -coverprofile=cover.out
  build-arm64:
    steps:
      - docker buildx build --platform linux/arm64 -t readingdrip:ci .
```

`golangci-lint` opcional (não bloqueante no início).

## E2E manual (por release)

- [ ] Subir com volume vazio, criar senha, logar.
- [ ] Importar 1 MD, 1 EPUB, 1 PDF (nativo e escaneado).
- [ ] Conferir plano nos dois modos.
- [ ] Editar uma fronteira e regenerar.
- [ ] Enviar teste pelo ntfy.
- [ ] “Enviar agora” com 3 documentos ativos.
- [ ] Reiniciar o container no meio da run.
- [ ] Abrir link do leitor no celular (sem sessão).
- [ ] Marcar como lido.
- [ ] Backup + restore em container limpo.
