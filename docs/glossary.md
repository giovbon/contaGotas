---
title: Glossário
status: ativo
updated: 2026-10-26
---

# Glossário

Termos usados com significado preciso neste projeto. Se um termo aparecer em código, ele deve usar o nome em inglês indicado (snake_case/camelCase conforme a camada).

| Termo | Código | Definição |
|---|---|---|
| **Documento** | `document` | Obra importada (`.md`, `.pdf`, `.epub`) com metadados e um plano ativo. |
| **Bloco** | `block` | Unidade atômica de conteúdo: heading, parágrafo, item de lista, código, tabela, citação, figura, nota. |
| **Seção** | `section` | Nó da árvore delimitado por headings. Um heading “possui” os blocos seguintes até o próximo heading de nível igual ou superior. |
| **Árvore** | `tree` | Hierarquia `documento → capítulo → seção → bloco`, derivada dos headings. |
| **Chunk** | `chunk` | Dose diária: intervalo contíguo de blocos com metadados de tempo e continuidade. |
| **Plano** | `plan` | Lista ordenada e completa de chunks de um documento, versionada e imutável após geração. |
| **Cursor** | `cursor` | Posição de leitura de um documento = menor `seq` de chunk `pending` do plano ativo. |
| **Pacing** | `pacing` | Ritmo de leitura de um documento: modo + valor (`10 min/dia` ou `terminar em 20 dias`). |
| **Alvo** | `target` | Duração pretendida de um chunk, em segundos. |
| **Mínimo** | `min_fill` | Fração do alvo abaixo da qual um chunk não deve ser fechado (padrão 0.60). |
| **Overshoot** | `max_overshoot` | Fração do alvo acima da qual um chunk não deve crescer (padrão 1.40). |
| **Fronteira** | `boundary` | Ponto permitido de corte entre dois blocos, com um custo associado. |
| **Oversized** | `oversized` | Chunk cuja duração excede o máximo: bloco indivisível maior que `max` ou fechamento de seção que exigiu ultrapassar `max`. Marcado e sinalizado na UI. |
| **Recap** | `recap` | Trecho extrativo do fim do chunk anterior, repetido no início do próximo para retomar contexto. |
| **Breadcrumb** | `section_path` | Caminho hierárquico do chunk (`Cap. 32 › 32.4 — A carta`). |
| **Run** | `delivery_run` | Execução de entrega de um dia, idempotente por `run_date`. |
| **Item de entrega** | `delivery_item` | Par `(run, documento)` → chunk enviado. |
| **Catch-up** | — | Recuperação de uma run perdida (container desligado) na próxima inicialização. |
| **Leitor** | `reader` | Página web que mostra o chunk com contexto, navegação e ações. |
| **Link assinado** | `signed link` | URL com token HMAC de curta/ média duração que dispensa login no celular. |
| **Corpo completo** | `full body` | Modo de entrega em que o texto do chunk vai inteiro no corpo da mensagem ntfy. |
| **Preview** | `preview body` | Modo de entrega em que o corpo traz só breadcrumb + recap + primeiras linhas + link. |
| **WPM** | `wpm` | Palavras por minuto usadas na estimativa de tempo. |
| **Fator de dificuldade** | `difficulty` | Multiplicador de tempo por documento (calibração). |
