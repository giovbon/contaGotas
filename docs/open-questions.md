---
title: Perguntas em aberto
status: ativo
updated: 2026-10-27
---

# Perguntas em aberto

Nada de `TODO` solto: toda dúvida tem ID, impacto e gatilho para decisão. Ao resolver, preencha **Decisão**, mude o status para `decidida` e registre a data. Se a resposta mudar contrato, crie um ADR.

| ID | Pergunta | Impacto | Status | Decidir até |
|---|---|---|---|---|
| `Q-001` | Nome definitivo do produto (hoje “Reading Drip”) | branding, binário, imagens | aberta | antes do primeiro release |
| `Q-002` | Suportar OCR de PDF escaneado? | escopo, dependência pesada (tesseract) | aberta — provável v2 | pós-FASE-1 |
| `Q-003` | Usar Cloudflare Access além da auth do app? | segurança, bypass de rotas, UX | aberta | antes do deploy público |
| `Q-004` | Recap por LLM opcional? Qual provedor/local? | custo, privacidade, dependência | aberta | FASE-3 |
| `Q-005` | Retenção de originais e planos antigos | disco, histórico | aberta | FASE-2 |
| `Q-006` | Imagens de EPUB/PDF no corpo da mensagem? | tamanho, fidelidade | aberta | FASE-2 |
| `Q-007` | Suporte a outros idiomas além de pt-BR? | WPM, divisor de frases, UI | aberta | FASE-3 |
| `Q-008` | Multi-usuário? | modelo de dados, auth, isolamento | aberta — fora da v1 | pós-v1 |
| `Q-009` | Exportar plano/documento (Markdown, CSV, JSON)? | interoperabilidade | aberta | FASE-3 |
| `Q-010` | Critério objetivo de qualidade de PDF | testes, regressão | decidida (2026-10-27) | FASE-0 (`SPIKE-01`) |
| `Q-011` | Um topic ntfy por documento ou um único topic? | organização no celular | aberta | FASE-1 |
| `Q-012` | Trocar guloso por DP no segmentador? | qualidade da distribuição | aberta | pós-FASE-3 |
| `Q-013` | Suportar múltiplas réplicas? | lock distribuído, complexidade | aberta — não na v1 | pós-v1 |
| `Q-014` | Notificar o usuário quando um documento termina? | UX | aberta | FASE-2 |
| `Q-015` | Enviar “resumo da semana” ou streak de leitura? | engajamento | aberta | pós-v1 |
| `Q-016` | Como reduzir o pico de memória de PDF com figuras grandes? | orçamento de recursos, `SPIKE-04`, figuras (`SPIKE-02`) | aberta | antes da FASE-3 |

---

## Detalhes

### Q-001 — Nome

“Reading Drip” é descritivo, mas genérico em inglês. Alternativas: `Gota`, `Pingo`, `Conta-gotas`, `Drip`. Impacto: binário, imagem, repositório, título das notificações. Decidir antes do primeiro release para evitar renomear schema/rotas.

### Q-002 — OCR

PDFs escaneados hoje ficam `needs_ocr`. Opções: (a) rejeitar com mensagem, (b) sidecar tesseract (imagem +200 MB, fere orçamento), (c) serviço externo (fere offline). Recomendação: manter (a) na v1.

### Q-003 — Cloudflare Access

A auth do app cobre UI e links assinados. Access adiciona SSO e proteção por e-mail, mas exige bypass para `/r/*` e `/api/actions/*` (ou service tokens), e complica o fluxo do celular. Decidir com o deploy real em mãos.

### Q-004 — LLM

Recap extrativo é gratuito e determinístico. LLM melhoraria a coesão, mas adiciona dependência, custo e latência. Se entrar, deve ser opcional, com cache por chunk e fallback automático para extrativo.

### Q-005 — Retenção

Planos `superseded` são mantidos por causa do histórico. Se o disco crescer, arquivar planos sem `delivery_items` após 180 dias. Originais nunca são removidos automaticamente.

### Q-006 — Imagens

No modo `full`, imagens inline aumentam muito o payload. Alternativa: substituir por `[imagem: alt]` + link para o leitor. Decidir após medir o tamanho real de um livro ilustrado.

### Q-010 — Qualidade de PDF

**Decisão (2026-10-27):** o critério de regressão é um conjunto de fixtures determinísticas com ground truth explícito:

- `pdf/headings-10pg.pdf`: 20 títulos em 10 páginas, exigindo 100% de detecção e zero falso positivo (`TestHeadingsDezPaginas`);
- `pdf/coluna-unica.pdf`: parágrafos, hifenização e remoção de cabeçalho/rodapé, com golden de plano;
- `pdf/duas-colunas.pdf`: ordem de leitura verificada frase a frase, com golden de plano;
- `pdf/escaneado.pdf`: erro `needs_ocr`.

O limite de ≥ 90% continua valendo para documentos reais com ground truth manual, mas não bloqueia a FASE-0: um PDF real de 10 páginas entra na revisão manual de saída da fase. Ver [`spikes.md`](./spikes.md) e [`extraction.md`](./extraction.md).

### Q-011 — Topic

Um topic único (`leitura`) centraliza; um por documento permite silenciar um livro. O ntfy permite assinar vários topics. Custo: mais configuração. Decidir antes da FASE-1 (afeta settings e delivery).

### Q-016 — Pico de memória de PDF com figuras

**Contexto (2026-10-27):** `HTML(page)` do `go-fitz` embute as imagens da página como PNG em base64, mesmo que a extração de texto as descarte. No GPT-3 (75 páginas, 6,5 MB), o pico chegou a 325 MB; `fitz.MaxStore=32MiB` baixa para ~229 MB, mas `GOGC`/`GOMEMLIMIT` não mudam porque a alocação é do lado C. Ver [`spikes.md`](./spikes.md) (`SPIKE-04`, parcial).

Opções: (a) aceitar o pico e dimensionar o container para ~350 MB; (b) reduzir `fitz.MaxStore` no boot; (c) extrair texto sem `HTML()` (API própria sobre `fz_stext_page`, mais código CGO); (d) processar o PDF em subprocesso com limite de memória. A decisão depende da medição em ARM64 e de um PDF de 300 páginas; se as figuras de PDF entrarem na FASE-3, a opção (c) passa a ser necessária de qualquer forma.
