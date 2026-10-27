---
title: ADR-0003 — PDF via go-fitz com CGO isolado
status: aceito
date: 2026-10-26
---

# ADR-0003 — PDF via go-fitz com CGO isolado

## Contexto

PDF é o formato mais difícil: ordem de leitura, colunas, hifenização, cabeçalhos repetidos e headings sem marcação semântica. As opções em Go variam de bibliotecas puras com extração rasa a bindings de MuPDF com libs nativas. O alvo é ARM64 com imagem pequena.

## Decisão

Usar **`github.com/gen2brain/go-fitz`** (wrapper de MuPDF) para PDF, com:

- CGO habilitado **somente** nos pacotes de PDF;
- build tag `cgo` + stub que retorna erro claro quando `CGO_ENABLED=0`;
- `internal/plan` e `internal/blocks` permanecem 100% Go, testáveis sem CGO;
- versão do módulo pinada; libs estáticas fornecidas pelo módulo.

MD e EPUB não passam por CGO.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| `ledongthuc/pdf` (puro Go) | ordem de leitura e parágrafos fracos; exigiria reimplementar layout |
| `unipdf` | licença comercial para uso real |
| `pdftotext`/poppler via shell | dependência externa no runtime, mais uma imagem |
| `go-pdfium` (wasm) | consumo de memória e complexidade de integração |
| Pré-processar PDF fora do app | fere o requisito de upload direto |

## Consequências

### Positivas
- Extração madura (MuPDF) com suporte a EPUB/MOBI no mesmo binding.
- Um binário estático ARM64, sem shell nem biblioteca de sistema.
- Núcleo de segmentação continua testável sem toolchain C.

### Negativas / riscos
- CGO exige toolchain no build e complica cross-compile.
- Binário maior (~30–45 MB).
- Vulnerabilidades do MuPDF precisam de acompanhamento.
- Qualidade ainda depende de heurísticas próprias.

### Mitigações
- Build preferencialmente nativo em runner ARM64; cross documentado.
- `SPIKE-01` valida API e memória antes da implementação.
- Stub sem CGO mantém MD/EPUB utilizáveis.
- Dependência atualizada a cada release, com changelog.
