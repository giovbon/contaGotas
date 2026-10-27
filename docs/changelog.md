---
title: Changelog
status: ativo
updated: 2026-10-26
---

# Changelog

Append-only, mais recente no topo. Uma entrada por fase ou mudança relevante. Não reescreva entradas antigas.

## 2026-10-26 — FASE-0.3: preparação para extração PDF

**Adicionado**
- `internal/docs/pdf`: pacote com `extract_cgo.go` (build tag `cgo`) e `extract_stub.go` (build tag `!cgo`), seguindo isolamento CGO via ADR-0003.
- `cmd/spike-pdf`: spike `pdfspike` para investigar API do `go-fitz` (spans, fonte/estilo/posição), memória e comportamento em arm64.
- CLI `plan` agora aceita `.pdf` e roteia para `internal/docs/pdf`. Uso atualizado.
- Dependência `github.com/gen2brain/go-fitz v1.24.14` adicionada ao `go.mod`.
- Diretórios `internal/docs/pdf/spike/`, `testdata/pdf/`, `cmd/spike-pdf/` criados.

**Comportamento**
- Sem CGO, `pdf.Extract` retorna erro claro; com CGO ainda não implementado (aguarda `SPIKE-01`).

**Pendente**
- `SPIKE-01` (fixar versão, testar spans/structured text, memória), extrator PDF (coluna única + duas colunas), golden para PDF, marcos da FASE-0.

## 2026-10-26 — FASE-0.2: extração EPUB

**Adicionado**
- `internal/docs/epub`: leitura de EPUB em memória (`archive/zip` + `x/net/html`), container/OPF/spine, metadados (título, autor, idioma), sanitização de HTML e blocos para headings, parágrafos, listas, código, citações, tabelas, figuras, `hr` e notas.
- CLI `plan` aceita `.epub` além de `.md`.
- Fixture determinística `testdata/epub/livro.epub`, gerador `testdata/epub/generate.go` e golden `testdata/golden/epub.plan.txt`.
- Testes de extração, sanitização, links seguros, spine não linear, fallback de título e erros de ZIP/container/OPF.
- `BenchmarkPlan10kBlocks` (~5 ms e ~4,8 MB por plano de 10 mil blocos no ambiente de desenvolvimento).
- ADR-0012: EPUB com `archive/zip` e `x/net/html`.

**Comportamento**
- Itens `linear="no"` são ignorados quando há itens lineares; itens `properties="nav"` são sempre ignorados.
- Links só sobrevivem com esquemas seguros e recebem `rel="noopener noreferrer"`; `script`, `style`, `iframe`, `svg` e `math` são descartados.
- Imagens preservam apenas `alt`/legenda; cópia de mídia fica para a FASE-1.
- Limites: 2 MiB de metadados, 8 MiB por XHTML, 64 MiB de XHTML por EPUB.

**Ajustado**
- Balanceamento agora move várias unidades entre grupos vizinhos até estabilizar, em vez de uma única unidade.
- `collectUnits` desce nós acima do alvo, não apenas acima do máximo.

**Pendente**
- `SPIKE-01`, extrator PDF, benchmarks.
- Mídia/capa do EPUB, TOC enriquecendo títulos e ligação de footnotes por referência.
- `go test -race` neste ambiente Windows (sem `gcc`); rodar em CI Linux/ARM64.

## 2026-10-26 — FASE-0.1: blocos, plano e extração Markdown

**Adicionado**
- `internal/blocks`: modelo `Block`, hashes SHA-256 de 16 caracteres, normalização NFC, estimador de tempo, divisão de frases/parágrafos, árvore de seções e `SectionPath`.
- `internal/plan`: geração determinística nos modos `minutes_per_day` e `finish_by_days`, empacotamento por unidades, balanceamento, materialização, `Validate` e formatação textual/JSON.
- `internal/docs`: detecção de formato e extrator Markdown com goldmark (GFM + footnotes) para headings, parágrafos, listas, código, tabelas, citações, figuras, `hr` e footnotes.
- `cmd/readingdrip`: CLI `plan` com `--minutes`, `--days`, `--wpm`, `--difficulty`, `--min-fill`, `--max-overshoot`, `--max-para-words` e `--json`; comando `version`.
- Fixtures `md/headings.md`, `md/plain.md`, `md/long-para.md`, `md/oversized.md` e cinco golden plans.
- Testes unitários, de propriedade (300 documentos aleatórios) e golden; `go test ./...` e `CGO_ENABLED=0 go test ./internal/plan/... ./internal/blocks/...` verdes.
- ADR-0011: parágrafos longos são pré-divididos em blocos alinhados a frases.

**Comportamento**
- Parágrafos acima de 300 palavras viram vários blocos `p`; o segmentador nunca corta dentro de um bloco.
- Nós que cabem no alvo são preservados inteiros; nós maiores descem um nível.
- Headings isolados e headings no fim de um nó são transportados para o próximo conteúdo.
- `oversized` marca qualquer chunk acima de `max`, inclusive quando o excesso vem de fechar uma seção.
- `starts_mid_section` e `ends_mid_section` derivam dos blocos vizinhos, não de offsets.

**Corrigido durante a implementação**
- Headings órfãos no fim de um nó/arquivo não podem mais criar chunks terminando em heading no meio do documento.
- Unidades agregadas mantêm métricas coerentes com o intervalo de blocos coberto.

**Pendente**
- Extrator EPUB, `SPIKE-01` e extrator PDF.
- Benchmarks (`BenchmarkPlan10kBlocks`).
- Front matter/autor, `rel="noopener noreferrer"` e ligação de footnotes por referência.
- `go test -race` neste ambiente Windows (sem `gcc`); rodar em CI Linux/ARM64.

## 2026-10-26 — Documentação inicial e decisões

**Adicionado**
- Árvore de documentação em `docs/`, com contexto, requisitos, arquitetura, modelo de dados, segmentação, tempo de leitura, extração, continuidade, entrega ntfy, agendamento, UI, operações, testes, contribuição, roadmap e perguntas em aberto.
- 10 ADRs registrando as decisões estruturantes.

**Decidido**
- Stack Go + templ + Alpine + Tailwind, binário único.
- Particionamento sobre blocos normalizados com âncoras por hash.
- Plano global persistido e determinístico.
- Dois modos de ritmo (`minutes_per_day`, `finish_by_days`).
- Documentos em paralelo, cada um com plano e pacing próprios.
- ntfy self-hosted com corpo completo em Markdown e link para o leitor.
- PDF via `go-fitz` (CGO isolado).
- SQLite `modernc.org/sqlite` em WAL.
- Imagem ARM64 distroless, non-root, read-only.
- Exposição via Cloudflare Tunnel + domínio próprio, com links assinados no leitor.

**Pendente**
- Implementação da `FASE-0` (núcleo headless).
- `SPIKE-01`: confirmar API do `go-fitz` para fonte/estilo.
- `Q-003`: decidir entre Cloudflare Access e auth própria.

## 2026-10-26 — FASE-0.3 (continuação): extrator PDF MVP

**Adicionado**
- `internal/docs/pdf/extract_cgo.go`: extração via `go-fitz` com cópia para temp file, detecção de PDF escaneado (< 50 chars/página), tentativa HTML primeiro com fallback para texto, normalização (NFC, hifenização, espaços), inferência básica de headings por padrão numérico.
- `internal/docs/pdf/pdf_test.go` e `extract_test_stub.go` para testes com/ssem CGO.
- Spike aprimorado em `cmd/spike-pdf` com medição de memória.

**Ajustado**
- `internal/docs/pdf/extract_stub.go` atualizado.
- `docs/extraction.md` com seção PDF FASE-0 detalhada.

**Pendente**
- SPIKE-01 completo (spans com fonte/estilo/posição), detecção de colunas, cabeçalho/rodapé repetidos, fixtures PDF, golden tests, fechar FASE-0.

## 2026-10-26 — FASE-0.3 (continuação 2): detecção de colunas e headings

**Adicionado**
- `internal/docs/pdf/col.go`: detecção de colunas por clustering de posições x, ordenação por coluna+y, utilitários estatísticos.
- `internal/docs/pdf/heading.go`: detecção de headings por tamanho de fonte, negrito e padrões numéricos.
- Função `isRepetido` para detecção de cabeçalho/rodapé repetidos.

**Pendente**
- SPIKE-01 completo, integração com go-fitz spans/fonte, fixtures PDF, golden tests, fechar FASE-0.
