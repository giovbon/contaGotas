---
title: ADR-0012 — EPUB com archive/zip e x/net/html
status: aceito
date: 2026-10-26
---

# ADR-0012 — EPUB com archive/zip e x/net/html

## Contexto

EPUB é um ZIP com `META-INF/container.xml`, um pacote OPF (metadados + manifest + spine) e documentos XHTML. XHTML real quase nunca é XML estrito: entidades HTML, tags não fechadas e namespaces ausentes aparecem em livros produzidos por ferramentas diferentes.

O extrator precisa ser tolerante sem baixar dependências nativas (o binário final é ARM64/distroless) e sem abrir caminho para HTML perigoso.

## Decisão

- Abrir o arquivo com `archive/zip` e ler entradas por nome via `fs.FS`, sem extrair para disco (evita zip slip).
- Parsear `container.xml` e o OPF com `encoding/xml`, que é estrito e suficiente para os metadados.
- Parsear cada XHTML com `golang.org/x/net/html`, o parser HTML5 tolerante mantido pelo projeto Go.
- Percorrer o `body` e emitir blocos para headings, parágrafos, itens de lista, código, tabelas, citações, figuras, `hr` e notas.
- Sanitizar o HTML gerado: whitelist de tags/atributos, remoção de `script`/`style`/`iframe`/`on*`, URLs perigosas descartadas e `rel="noopener noreferrer"` em links.

Mídia (imagens) fica para a etapa de armazenamento do leitor: nesta fase o bloco `figure` preserva o texto alternativo e não referencia caminhos internos do EPUB.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| `encoding/xml` para o XHTML | falha em HTML5 tolerante e entidades comuns |
| `goquery`/Cascadia | dependência extra sobre o mesmo parser; não precisamos de seletores CSS |
| Extrair o ZIP para disco | risco de zip slip, I/O desnecessário, limpeza de temporários |
| Renderizar HTML cru do EPUB | XSS e links quebrados |

## Consequências

### Positivas
- Um único parser tolerante, sem CGO, com custo de memória controlado.
- Leitura direta do ZIP, sem arquivos temporários.
- Superfície de HTML reduzida por whitelist.

### Negativas / riscos
- `x/net/html` é uma dependência de módulo (não nativa), atualizada com o ecossistema Go.
- A sanitização é própria: precisa de testes com payloads.
- Imagens e capa ainda não são servidas.

## Referências

- [ADR-0002](./0002-modelo-de-blocos-normalizado.md)
- [Extração](../extraction.md)
- `internal/docs/epub/epub.go`
