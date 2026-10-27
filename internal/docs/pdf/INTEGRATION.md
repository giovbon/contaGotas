# Integração PDF

Estado atual:
- Extrator MVP funcional com go-fitz
- Detecção de colunas, headings por estilo, cabeçalho/rodapé
- Normalização, hifenização, junção de linhas
- SPIKE-01 preparado

Para integrar completamente:
1. Usar `HTML` com spans quando disponível para obter posição/fonte
2. Aplicar detecção de colunas em páginas com múltiplas colunas
3. Filtrar cabeçalho/rodapé repetidos por documento
4. Melhorar detecção de headings por tamanho de fonte
5. Testar com fixtures reais

Build com CGO:
```sh
CGO_ENABLED=1 go build ./internal/docs/pdf/...
CGO_ENABLED=1 go test ./internal/docs/pdf/... -tags cgo
```

SPIKE:
```sh
go run -tags pdfspike,cgo ./cmd/spike-pdf arquivo.pdf
```
