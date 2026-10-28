# Fixtures PDF

Fixtures determinísticas usadas pelos testes de extração (`internal/docs/pdf`):

| Arquivo | Cobre |
|---|---|
| `coluna-unica.pdf` | coluna única, títulos por fonte, parágrafos, hifenização, cabeçalho/rodapé |
| `duas-colunas.pdf` | duas colunas com fluxo de conteúdo intercalado |
| `escaneado.pdf` | página sem camada de texto (erro `needs_ocr`) |
| `headings-10pg.pdf` | 10 páginas com 20 títulos (critério do `SPIKE-01`) |

Os arquivos são montados à mão (PDF 1.4, fonte base-14 Helvetica), sem
dependência externa. Para regenerar, a partir da raiz do repositório:

```sh
go run testdata/pdf/generate.go
```

O texto é ASCII de propósito: evita ambiguidade de codificação entre o
gerador e o `WinAnsiEncoding` da fonte.
