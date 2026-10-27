# SPIKE-01 — go-fitz

Objetivo: confirmar API do go-fitz (versão a fixar) para extrair spans com fonte/estilo/posição, comportamento em CGO arm64, consumo de memória com PDF ~300 páginas.

## Perguntas
- go-fitz expõe spans com font-size, flags (bold/italic), posição (x,y)?
- Há API para structured text/HTML com informações de estilo?
- Como obter ordem de leitura correta em duas colunas?
- Consumo de memória em PDF de 300 páginas?

## Método
1. Fixar versão em go.mod
2. Criar spike com build tag `pdfspike`
3. Testar com PDFs reais
4. Medir memória (RSS)
5. Documentar achados

## Build
```sh
go run -tags pdfspike,cgo ./cmd/spike-pdf arquivo.pdf
```

## Entregável
Nota em `docs/open-questions.md` (Q-010) e `docs/spikes.md` com API escolhida e critérios de saída.
