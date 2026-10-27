---
title: Roadmap
status: ativo
updated: 2026-10-26
---

# Roadmap

Fases entregáveis, com critério de saída verificável. Status: `planejado` · `em andamento` · `concluído`.

| Fase | Tema | Status |
|---|---|---|
| `FASE-0` | Núcleo headless (extração + tempo + segmentador) | em andamento |
| `FASE-1` | MVP utilizável (upload, plano, ntfy, leitor) | planejado |
| `FASE-2` | Automação (agendador, histórico, retry) | planejado |
| `FASE-3` | Refinamento (edição de plano, calibração, PDF) | planejado |
| `FASE-4` | Operação (imagem ARM64, backup, hardening) | planejado |

---

## FASE-0 — Núcleo headless

**Objetivo:** provar que o particionamento funciona antes de investir em UI.

- [x] modelo `blocks.Block` + árvore de seções
- [x] estimador de tempo + testes
- [x] extrator Markdown (goldmark) + golden
- [x] extrator EPUB + golden
- [ ] `SPIKE-01`: confirmar API do `go-fitz` para fonte/estilo
- [ ] extrator PDF (coluna única e duas colunas) + golden
- [x] segmentador: modo A e modo B, empacotamento e balanceamento
- [x] pré-divisão de parágrafo gigante em blocos alinhados a frases
- [x] `Validate` + testes de propriedade
- [x] CLI `readingdrip plan --input arquivo --minutes 10` imprimindo o plano
- [x] benchmarks (`BenchmarkPlan10kBlocks`)

**Incremento 0.3 (em andamento):** estrutura para extrator PDF com build tag `cgo`, stubs `!cgo`, spike `cmd/spike-pdf` preparado, CLI aceita `.pdf` (rota para extrator), `github.com/gen2brain/go-fitz` adicionado ao `go.mod`. Pendentes `SPIKE-01` e implementação.

**Incremento 0.1 (2026-10-26):** `internal/blocks`, `internal/plan`, `internal/docs/markdown` e `cmd/readingdrip` funcionando; 5 goldens de Markdown (incluindo modo B e bloco oversized); 300 casos de propriedade. Ver [`changelog.md`](./changelog.md).

**Incremento 0.2 (2026-10-26):** extrator EPUB com `archive/zip` + `x/net/html`, sanitização de HTML, metadados, spine, fixture determinística e golden `epub.plan.txt`; CLI aceita `.epub`. Ver [ADR-0012](./adr/0012-epub-zip-xhtml.md).


**Saída:** rodar o CLI em 3 fixtures reais e obter planos que “fazem sentido para ler” em revisão manual, com todos os testes verdes.

**Riscos:** qualidade do PDF em duas colunas; API do `go-fitz`; tempo de extração em ARM64.

---

## FASE-1 — MVP utilizável

- [ ] SQLite + migrações + repositórios
- [ ] upload multipart com limite e deduplicação por sha256
- [ ] worker de extração (1 por vez) + status
- [ ] persistência de plano e chunks
- [ ] UI: biblioteca, detalhe, preview do plano
- [ ] configurações: ntfy, agenda, plano
- [ ] composição da mensagem (breadcrumb, recap, rodapé)
- [ ] cliente ntfy + envio de teste + “enviar agora”
- [ ] leitor com token assinado + “marcar lido”
- [ ] autenticação por senha + sessão
- [ ] Dockerfile + imagem arm64
- [ ] deploy atrás de Cloudflare Tunnel

**Saída:** três documentos ativos, envio manual funcionando no celular, leitor abrindo por link sem login, container rodando em ARM64 com < 100 MiB.

---

## FASE-2 — Automação

- [ ] ticker + calendário + timezone/DST
- [ ] idempotência por `run_date`
- [ ] catch-up
- [ ] lock de run + retomada após restart
- [ ] retry com backoff e classificação de erro
- [ ] histórico de runs e itens
- [ ] pausar/retomar por documento
- [ ] limite `max_docs_per_run`
- [ ] avisos de pacing inviável

**Saída:** uma semana inteira de entregas automáticas sem intervenção, incluindo um restart forçado no meio.

---

## FASE-3 — Refinamento

- [ ] edição de fronteiras (mesclar/dividir/mover)
- [ ] regenerar plano a partir do cursor
- [ ] simulação de parâmetros no preview
- [ ] calibração de `difficulty` por feedback
- [ ] hardening de PDF (3+ colunas, notas, figuras)
- [ ] `SPIKE-02`: renderização de figuras de PDF
- [ ] recaps opcionais por LLM (desligado por padrão)
- [ ] busca full-text (FTS5)
- [ ] exportar plano em Markdown/CSV

**Saída:** usuário consegue corrigir um plano ruim sem reimportar o documento.

---

## FASE-4 — Operação

- [ ] backup automático + restore testado
- [ ] `/healthz` e `/readyz` completos
- [ ] métricas leves (contadores em memória)
- [ ] rotação de `APP_SECRET` com regeneração de links
- [ ] checklist de segurança aplicado
- [ ] documentação de upgrade
- [ ] pipeline de release (multi-arch? foco arm64)
- [ ] teste de carga com 50 documentos

**Saída:** upgrade de versão sem perda de dados e restore validado em container limpo.

---

## Fora do roadmap

- OCR (`Q-002`)
- multi-usuário (`Q-008`)
- envio para Kindle
- apps nativos
- TTS

## Próximo passo imediato

`SPIKE-01` e extrator PDF; em seguida benchmarks e revisão manual dos planos. Blocos, segmentador, Markdown, EPUB e o CLI já estão implementados.

**Incremento 0.3b (em andamento):** extrator PDF MVP (coluna única, HTML/Text via go-fitz, detecção `needs_ocr`, headings por padrão numérico). SPIKE-01 aprimorado com medição de memória.

**Incremento 0.3c (em andamento):** detecção de colunas, detecção de headings por estilo, detecção de cabeçalho/rodapé repetidos.

**Incremento 0.3d (em andamento):** desfazer hifenização, juntar linhas em parágrafos, spike analisando fontes/imagens.

**Incremento 0.3e (em andamento):** junção de linhas com quebra por pontuação, detecção de cabeçalho/rodapé repetidos.

**Incremento 0.3f (em andamento):** golden tests PDF, preparação para integração.
