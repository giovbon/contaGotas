---
title: Perguntas em aberto
status: ativo
updated: 2026-10-26
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
| `Q-010` | Critério objetivo de qualidade de PDF | testes, regressão | aberta | FASE-0 (`SPIKE-01`) |
| `Q-011` | Um topic ntfy por documento ou um único topic? | organização no celular | aberta | FASE-1 |
| `Q-012` | Trocar guloso por DP no segmentador? | qualidade da distribuição | aberta | pós-FASE-3 |
| `Q-013` | Suportar múltiplas réplicas? | lock distribuído, complexidade | aberta — não na v1 | pós-v1 |
| `Q-014` | Notificar o usuário quando um documento termina? | UX | aberta | FASE-2 |
| `Q-015` | Enviar “resumo da semana” ou streak de leitura? | engajamento | aberta | pós-v1 |

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

Proposta de critério: para uma fixture de 10 páginas com ground truth, ≥ 90% dos parágrafos com fronteiras corretas e ordem de leitura correta. Medir no `SPIKE-01` e congelar como teste de regressão.

### Q-011 — Topic

Um topic único (`leitura`) centraliza; um por documento permite silenciar um livro. O ntfy permite assinar vários topics. Custo: mais configuração. Decidir antes da FASE-1 (afeta settings e delivery).
