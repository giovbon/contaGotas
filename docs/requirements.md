---
title: Requisitos
status: ativo
updated: 2026-10-26
---

# Requisitos

Prioridade: **M** (must, v1), **S** (should, v1 se couber), **C** (could, pós-v1). Todo requisito é verificável; o critério de aceite é a fonte da verdade para testes.

## Funcionais — documentos

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-001 | Importar `.md`, `.pdf` e `.epub` por upload na UI, um ou vários por vez | M | Upload de 3 arquivos cria 3 documentos em `processing` |
| RF-002 | Processar extração de forma assíncrona, com status visível | M | UI mostra `processing → ready` sem bloquear a request; upload responde < 1 s |
| RF-003 | Manter vários documentos ativos em paralelo, cada um com plano e pacing próprios | M | 3 docs ativos recebem 1 chunk cada na mesma run |
| RF-004 | Pausar, retomar, excluir e reprocessar documento | M | Pausado não aparece na run; retomar continua do cursor; excluir remove blocos/planos/arquivos |
| RF-005 | Editar metadados (título, autor, idioma, WPM, dificuldade, prioridade) | S | Alteração persiste e reflete na próxima run |
| RF-006 | Preservar o arquivo original e permitir download | M | Original em `/data/documents/{id}/source.*`, inalterado, baixável |
| RF-007 | Evitar duplicidade por conteúdo | S | Mesmo `sha256` não cria segundo documento (aviso com link para o existente) |
| RF-008 | Suportar upload até 95 MiB (limite Cloudflare) | M | Arquivo de 90 MiB é aceito; acima é rejeitado com erro claro |

## Funcionais — extração

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-010 | Gerar blocos tipados com `ord`, `kind`, `level`, `text`, `html`, `words`, `seconds`, `hash` | M | `ord` contíguo de 0 a n-1; nenhum bloco vazio |
| RF-011 | Construir hierarquia de seções a partir dos headings | M | Todo bloco tem `heading_id` (ou NULL para conteúdo antes do primeiro heading) |
| RF-012 | Servir imagens de EPUB/PDF no leitor | S | `<img>` resolve para `/media/{docID}/...` com cache imutável |
| RF-013 | Detectar PDF sem camada de texto | M | Documento fica `needs_ocr` com mensagem acionável, sem plano |
| RF-014 | Normalizar PDF: hifenização, cabeçalho/rodapé repetido, colunas, ordem de leitura | M | Fixtures de 1 e 2 colunas produzem parágrafos na ordem correta |
| RF-015 | Preservar Markdown semântico (listas, código, tabelas, citações) no `html` | M | Golden test compara HTML renderizado por tipo de bloco |
| RF-016 | Extrair metadados (título, autor, idioma, capa) quando disponíveis | S | EPUB preenche título/autor/idioma sem digitação |

## Funcionais — ritmo e plano

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-020 | Modo `minutes_per_day`: alvo fixo em minutos por dia de leitura | M | Documento de 100 min com 10 min/dia gera ~10 chunks |
| RF-021 | Modo `finish_by_days`: alvo = tempo restante ÷ dias de leitura restantes | M | Documento de 100 min com 10 dias gera ~10 chunks de ~10 min |
| RF-022 | Preview do plano com duração estimada por chunk e projeção de datas | M | UI lista chunks com minutos e datas previstas |
| RF-023 | Editar fronteiras: mesclar, dividir e mover blocos entre chunks | S | Edição persiste, revalida invariantes e não afeta chunks já enviados |
| RF-024 | Regenerar plano a partir do cursor, preservando histórico | M | Chunks enviados permanecem; nova versão cobre apenas o restante |
| RF-025 | Versionar planos; apenas um ativo por documento | M | `UNIQUE(document_id) WHERE status='active'` garantido no store |
| RF-026 | Avisar quando o pacing é incompatível com o conteúdo | S | Ex.: 30 dias para 20 min de texto gera alerta, não plano inválido |

## Funcionais — segmentação

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-030 | Respeitar `min_fill` e `max_overshoot` salvo impossibilidade estrutural | M | Nenhum chunk acima de `max` sem `oversized`; chunks curtos são permitidos quando inevitáveis |
| RF-031 | Não cortar blocos indivisíveis | M | Nenhum corte dentro de código/tabela/lista/citação |
| RF-032 | Nunca terminar chunk em heading | M | Nenhum chunk com `last_block.kind LIKE 'h%'`, exceto heading órfão no fim do documento |
| RF-033 | Corte de parágrafo apenas em fronteira de frase | M | Parágrafos > 300 palavras são pré-divididos em blocos `p`; o chunk nunca corta dentro de um bloco |
| RF-034 | Determinismo total | M | Duas execuções com o mesmo input produzem planos byte a byte iguais |
| RF-035 | Cobertura total e sem sobreposição | M | Soma dos intervalos = todos os blocos, exatamente uma vez |
| RF-036 | Preferir fim de seção a fim de parágrafo, e parágrafo a frase | M | Nós que cabem no alvo são preservados inteiros; pontuação de fronteira fica para a otimização futura (`Q-012`) |

## Funcionais — continuidade

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-040 | Toda mensagem abre com breadcrumb do documento e seção | M | Primeira linha contém `documento › seção` |
| RF-041 | Recap extrativo do chunk anterior (2 frases, ≤ 240 caracteres) | M | Mensagem do dia 2+ contém “Onde paramos” |
| RF-042 | Rodapé “continua amanhã” quando o corte é no meio de seção | M | Chunk com `ends_mid_section=1` traz próximo heading |
| RF-043 | Link para o leitor com contexto (seção inteira, anterior/próximo) | M | Link assinado abre a página correta sem login |
| RF-044 | Indicador de progresso (percentual e capítulo atual) | C | Presente no corpo ou no título |

## Funcionais — entrega

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-050 | Configurar ntfy self-hosted: base URL, topic, token, prioridade | M | Envio de teste chega no celular |
| RF-051 | Enviar corpo completo em Markdown (`body_mode=full`) | M | Chunk de 10 min chega inteiro com formatação |
| RF-052 | Modo `preview`: breadcrumb + recap + primeiras linhas + link | S | Corpo ≤ 1 KB e link funcional |
| RF-053 | Agrupar chunks de vários documentos em uma mensagem | M | 3 docs = 1 mensagem com 3 seções `## 📖 …` |
| RF-054 | Respeitar limite de bytes; dividir por documento ou truncar com link | M | Corpo acima de `max_message_bytes` nunca é enviado |
| RF-055 | Botões “Abrir leitor” e “Marcar como lido” | S | Ação HTTP do ntfy retorna 2xx e marca o chunk |
| RF-056 | Retry com backoff em falha transitória; sem retry em 4xx | M | 500 → 3 tentativas; 404 → falha imediata |
| RF-057 | Histórico de runs com status, tamanho e erro | M | UI lista últimas 30 runs e detalhe por documento |
| RF-058 | Envio manual (“enviar agora”) de um documento ou de todos | S | Botão dispara run fora do horário |

## Funcionais — agendamento

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-060 | Configurar dias da semana, horário e timezone IANA | M | Run dispara no dia/hora locais corretos, inclusive após DST |
| RF-061 | Idempotência por data local | M | Duas execuções no mesmo dia geram uma única run |
| RF-062 | Catch-up de run perdida na janela configurada | M | Container desligado às 07:00 envia ao subir às 09:00, marcado `late` |
| RF-063 | Mudar dias/horário não re-segmenta nem perde conteúdo | M | Cursor intacto; próxima run segue do chunk pendente |
| RF-064 | Avançar cursor apenas em caso de sucesso | M | Falha mantém chunk `pending` para a próxima tentativa |
| RF-065 | Pausar documento sem afetar os demais | M | Doc pausado não entra na run; os outros seguem |
| RF-066 | Limitar documentos por run (`max_docs_per_run`) | C | Excesso fica para a próxima run, na ordem de prioridade |

## Funcionais — leitor e UI

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-070 | Página do chunk com conteúdo renderizado e contexto expansível | M | Abre em < 1 s; seção completa a um clique |
| RF-071 | Navegação anterior/próximo e “voltar ao documento” | S | Links corretos respeitando status do chunk |
| RF-072 | Links assinados com expiração configurável | M | Token inválido/expirado → 401 com mensagem |
| RF-073 | Marcar chunk como lido no leitor | S | Atualiza `done_at`, reflete no histórico |
| RF-074 | Autenticação por senha única + sessão assinada | M | Sem sessão, rotas de gestão redirecionam para `/login` |
| RF-075 | UI responsiva, utilizável no celular | M | Leitor e configurações funcionam em 360 px |
| RF-076 | Healthcheck `/healthz` e `/readyz` | M | 200 com banco acessível; 503 caso contrário |

## Funcionais — operação

| ID | Requisito | Prio | Critério de aceite |
|---|---|---|---|
| RF-090 | Binário único com templ, CSS e JS embutidos | M | Container sem Node no runtime |
| RF-091 | Imagem `linux/arm64` publicável | M | `docker buildx build --platform linux/arm64` conclui e roda |
| RF-092 | Migrações automáticas no start, com backup prévio | M | Upgrade de versão preserva dados |
| RF-093 | Backup e restore documentados e testados | M | Restore em container limpo reproduz o estado |
| RF-094 | Exposição via Cloudflare Tunnel + domínio próprio | M | `APP_BASE_URL` https; links e cookies corretos atrás do proxy |

## Não funcionais

| ID | Requisito | Alvo |
|---|---|---|
| RNF-001 | Memória em idle | ≤ 60 MiB RSS |
| RNF-002 | Pico de memória na extração | ≤ 200 MiB (1 extração por vez) |
| RNF-003 | Imagem do container | ≤ 90 MiB |
| RNF-004 | Cold start | ≤ 500 ms |
| RNF-005 | Extração de PDF 300 páginas | ≤ 15 s em ARM64 |
| RNF-006 | Latência de entrega | ≤ 5 s p95 |
| RNF-007 | Funcionar sem internet | Exceto a chamada ao ntfy |
| RNF-008 | Sem dependência de serviço externo | SQLite local, sem fila, sem cache externo |
| RNF-009 | Segurança HTTP | CSP, `X-Content-Type-Options`, `Referrer-Policy`, `no-store` em HTML |
| RNF-010 | Acessibilidade | WCAG AA nas telas de leitura |
| RNF-011 | Determinismo | Segmentador com testes de propriedade |
| RNF-012 | Robustez a restart | Nenhuma entrega duplicada ou perdida silenciosamente |
| RNF-013 | Upgrade | Migrações forward-only, sem perda de dados |
| RNF-014 | Logs | Estruturados, sem conteúdo de leitura nem tokens |
| RNF-015 | Compatibilidade de upload | ≤ 95 MiB (margem do limite Cloudflare) |
