---
title: Agendamento
status: ativo
updated: 2026-10-27
---

# Agendamento

Um ticker em processo verifica a cada 60 s se é hora de entregar. Sem cron externo, sem fila, sem dependência de relógio do host além de `time.Now`.

## Configuração

| Chave | Padrão | Descrição |
|---|---|---|
| `schedule.days` | `mon,tue,wed,thu,fri` | dias de leitura |
| `schedule.time` | `07:00` | horário local |
| `schedule.timezone` | `America/Sao_Paulo` | IANA |
| `schedule.catchup_days` | `1` | janela de recuperação, em dias de leitura |
| `schedule.max_docs_per_run` | `5` | documentos por run |
| `schedule.enabled` | `true` | liga/desliga o agendador |

As chaves do leitor são `reader.link_ttl_days` (padrão 180) e `reader.done_ttl_days` (padrão 30), em dias, aceitando 1..3650; `Settings.LinkTTLs()` devolve as duas como `time.Duration`. Elas alimentam `Runner.Links`, que monta as URLs assinadas de cada dose (ver [`delivery-ntfy.md`](./delivery-ntfy.md#links-assinados)).

As chaves de entrega (`ntfy.base_url`, `ntfy.topic`, `ntfy.token`, `ntfy.priority`, `ntfy.body_mode`, `ntfy.max_message_bytes`) estão em [`delivery-ntfy.md`](./delivery-ntfy.md). Todas são lidas do banco a cada tick por `schedule.LoadSettings`; valor inválido devolve erro citando a chave, sem correção silenciosa. O agendador não cria run enquanto `ntfy.base_url` ou `ntfy.topic` estiverem vazios. A UI de configurações usa o mesmo `schedule.Parse` antes de gravar, então nenhum valor que o tick recusaria chega ao banco.

## Ciclo

```go
func (r *Runner) Tick(ctx, now) {
    if !r.runMu.TryLock() { return "tick anterior em andamento" }
    st := LoadSettings()                 // schedule.* e ntfy.*
    if !st.Schedule.Enabled { return "agendador desligado" }
    pub := publisher(st.Ntfy)            // sem URL/topic → "ntfy não configurado"

    if run := Runs.Running(); run != nil {
        return resumeRun(run)            // só itens pending
    }

    local := now.In(st.Schedule.Location)
    if isReadingDay(local) && !local.Before(scheduledFor(local)) {
        createRun(local, late=false)
    }
    if dia, ok := diaPerdido(local); ok { // janela em dias de leitura
        createRun(dia, late=true)
    }
}
```

Cada `createRun`:

1. `INSERT INTO delivery_runs(run_date, scheduled_for, started_at, late)` com `ON CONFLICT DO NOTHING`.
2. Se a inserção não criou linha (outra instância/tick venceu), a run existente não é reexecutada.
3. Seleciona documentos `ready`, ordenados por `priority, id`, limitados a `max_docs_per_run`.
4. Para cada documento, pega o próximo chunk `pending` do plano ativo e cria um item `pending` na run (`UNIQUE (run_id, document_id)`).
5. Monta a dose com `Runner.Links(doc, chunk, readerTTL, doneTTL)`, se `Links` estiver definido; sem ele, a mensagem sai sem `click` e sem ações.
5. Monta as doses, compõe e publica (ver [`delivery-ntfy.md`](./delivery-ntfy.md)).
6. Fecha a run: `sent` se nenhum item falhou, `partial` se alguns falharam, `failed` se todos falharam; run sem documento elegível fecha `sent`.

O tick inteiro roda na goroutine do ticker, protegido por `runMu` (`TryLock`): um tick lento não sobrepõe o seguinte, e o próximo só começa depois que o anterior retorna. `Stop` cancela o contexto e espera a goroutine terminar.

## Retomada após restart

Uma run interrompida (shutdown, crash ou erro de contexto) fica `running`. No próximo `Start`, o primeiro tick — que roda imediatamente — encontra a run por `Runs.Running` e a retoma **antes** de criar qualquer run nova. A retomada também exige ntfy configurado; sem URL/topic, a run fica parada até a configuração voltar:

- só itens `pending` são reenviados; `sent` e `skipped` não são tocados;
- se o chunk do item já não está `pending` (entrega manual concorrente), o item vira `skipped`;
- se o documento sumiu, o item vira `skipped`;
- falha de composição ou publicação marca o item `failed`; o chunk continua `pending`;
- ao final, a run é fechada com o mesmo cálculo de `sent`/`partial`/`failed`.

## Falhas e estados

| Situação | Item | Chunk | Run |
|---|---|---|---|
| Publicação 2xx | `sent` + `bytes` + `attempts` | `sent` + `sent_at` | segue |
| 4xx/timeout após as tentativas | `failed` + erro | `pending` | `partial`/`failed` |
| Contexto cancelado no envio | `pending` | `pending` | fica `running` para retomada |
| Chunk já entregue por outro caminho | `skipped` | inalterado | `sent`/`partial` |
| Documento removido na retomada | `skipped` | inalterado | `sent`/`partial` |
| Erro ao montar ou publicar a run | pendentes `failed` | `pending` | `failed` |

`delivery_items.bytes` guarda o tamanho da mensagem que levou o item; quando a mensagem é agrupada, o mesmo valor é repetido em cada item. Os `attempts` vêm do cliente ntfy. Ao fechar a run por erro (não por cancelamento), itens ainda `pending` são marcados `failed` com o motivo, para não ficarem órfãos no histórico; o chunk continua `pending`.

## Catch-up

Na inicialização e a cada tick:

- olha para trás até `catchup_days` **dias de leitura** (não dias corridos);
- se existia dia de leitura sem run e o horário já passou, cria a run agora com `late=1`;
- envia **somente a run perdida mais recente** (não enfileira uma por dia);
- depois que existe uma run `late=1` dentro da janela, ticks seguintes não criam as mais antigas: o catch-up acontece **uma vez** por janela;
- entrega única por documento (o próximo chunk pendente), não “os chunks de todos os dias perdidos”;
- a run do dia é criada primeiro; o catch-up é independente e não é bloqueado por ela.

Exemplo: container desligado de sexta 07:00 a segunda 09:00, dias úteis:

| Dia | Resultado |
|---|---|
| sexta | run criada na segunda, `late=1` |
| sábado/domingo | não são dias de leitura |
| segunda | run normal, no horário (se ainda não passou) |

Se a run de segunda já existir, o catch-up de sexta ainda é criado (uma vez), com `late=1`. O histórico deixa claro o que aconteceu. Um tick posterior não repete nenhuma das duas porque `run_date` é única.

## Mudança de dias/horário

O agendador **não** conhece o plano. Ele só consome o próximo chunk `pending`. Portanto:

- mudar dias da semana → nenhum re-corte, nenhuma perda;
- mudar horário → vale a partir do próximo tick;
- desligar o agendador → nenhuma run nova; nada é perdido;
- religar → retoma do cursor.

## Mudança de plano no meio do caminho

| Ação | Efeito |
|---|---|
| Alterar `minutes_per_day` | oferece regenerar a partir do cursor |
| Alterar `finish_by_days` | idem |
| Alterar `min_fill`/`max_overshoot` | idem |
| Editar fronteira de um chunk pendente | plano ativo mutado (permitido só em `pending`) |
| Editar chunk já enviado | proibido |
| Reprocessar arquivo | novo plano; hashes divergentes → `stale` |

## Cursor

O cursor é derivado, não armazenado:

```sql
SELECT MIN(seq) FROM chunks
WHERE plan_id = ? AND status = 'pending';
```

Consequências:
- pausar/retomar é só mudar `documents.status`;
- chunk `skipped` (pulado manualmente) não trava a fila;
- regenerar plano não exige migrar cursor.

## Modos de ritmo no calendário

| Modo | Efeito no calendário |
|---|---|
| `minutes_per_day` | término emerge: `chunks ÷ dias de leitura por semana` |
| `finish_by_days` | horizonte de N dias de leitura; dias perdidos **empurram** o término |

Projeção exibida na UI: para os próximos 14 dias de leitura, calcula a data de cada chunk restante. É estimativa, não compromisso.

## Fuso e DST

- Todo cálculo usa `time.Location` carregada de `schedule.timezone`.
- `run_date` é `local.Format("2006-01-02")`.
- Em transição de horário de verão:
  - horário inexistente (ex.: 02:30 que pulou para 03:30) → dispara no primeiro tick após o horário;
  - horário duplicado → a `run_date` única garante um envio só.
- `TZ` do container é irrelevante para a regra; afeta apenas logs.

## Concorrência e múltiplas instâncias

O desenho assume **uma instância**. Se duas subirem:

- `run_date UNIQUE` impede runs duplicadas;
- o lock é local ao processo, então há risco de duas tentativas no mesmo item em cenário de split-brain;
- suporte oficial: 1 réplica. Escalar horizontalmente exigiria lock no banco (`BEGIN IMMEDIATE` + lease), fora do escopo (`Q-013`).

## Envio manual

`POST /documents/{id}/send-now` entrega o próximo trecho do documento fora do horário. Regras (implementadas em `Runner.SendNow`):

- usa a run do dia (`run_date` local); se ela não existe, é criada com `late=0` e `scheduled_for` = instante do clique;
- a run criada aqui **é** a run do dia: o tick seguinte não entrega de novo, e um documento enviado manualmente não recebe outra dose no mesmo dia;
- se a run do dia já está fechada (`sent`/`partial`/`failed`), nada é reenviado e a UI responde `ja-enviado`; o próximo trecho sai na próxima run;
- item `failed` de uma tentativa anterior é reenviado (o chunk continua `pending`, RF-064), desde que a run do dia ainda esteja `running`;
- run `running` de um tick interrompido é reaproveitada, mas **não** é fechada: o próximo tick a retoma e finaliza;
- sem trechos pendentes, sem ntfy configurado, com tick/envio concorrente (`TryLock`) ou documento fora de `ready`, nada é criado nem enviado;
- a publicação usa **uma** tentativa (sem backoff): o clique vê o resultado na hora e repetir é o retry;
- falha de publicação antes de qualquer envio **remove** a run criada por esse clique, para não consumir a run programada do dia; o chunk continua `pending`;
- registra o evento `delivery.manual` (info no sucesso, warn na falha) e a run aparece no histórico como qualquer outra.

O mesmo `TryLock` dos ticks serializa o envio manual: um clique durante uma entrega responde “entrega em andamento” em vez de esperar os retries do tick.

## Envio de teste

`Runner.SendTest` publica a mensagem de teste com a configuração salva, também em uma única tentativa. É síncrono, com `context` de 20 s aplicado pelo handler (`POST /settings/test`), e não cria run, item nem evento de entrega: só loga status, bytes, latência e tentativas. O contrato da tela está em [`web-ui.md`](./web-ui.md).

## Relógio

`internal/platform.Clock` injetável:

```go
type Clock interface{ Now() time.Time }
```

Testes usam `FakeClock`; produção usa `time.Now`. Nenhum código de agendamento chama `time.Now` diretamente.

## Testes

`internal/schedule` usa banco real em `t.TempDir()`, `platform.FakeClock` e um publisher falso que só registra mensagens.

| Teste | Cenário |
|---|---|
| `TestTickAntesDoHorario` | antes do horário não cria run |
| `TestTickDiaNaoConfigurado` | domingo com dias úteis não cria run |
| `TestTickCompletaRun` | cria run, envia um chunk, fecha `sent`; segundo tick não repete |
| `TestTickAgrupaDocumentos` | duas doses viram uma mensagem com dois chunks |
| `TestTickCatchupCriaRunPerdida` | segunda 08:00 cria a run de hoje e a de sexta com `late=1`, uma vez |
| `TestTickCatchupForaDaJanela` | janela de 1 dia de leitura não alcança segunda |
| `TestTickCatchupSoUmaVezNaJanela` | cinco dias perdidos geram só um catch-up, mesmo em ticks seguintes |
| `TestTickDesligado` | `schedule.enabled=false` não cria run |
| `TestTickSemNtfy` | sem URL/topic não cria run e devolve `ntfy não configurado` |
| `TestTickPulaDocumentoPausado` | documento `paused` não entra; o `ready` segue |
| `TestTickLimitaDocumentosPorRun` | `max_docs_per_run=1` respeitado |
| `TestTickFalhaPublicacaoNaoAvancaChunk` | 401 → item `failed`, chunk `pending`, run `failed` |
| `TestTickRetomaRunInterrompida` | run `running` reenvia só o item `pending` e fecha |
| `TestTickRetomaPulaChunkJaEnviado` | chunk já `sent` → item `skipped` |
| `TestTickConfiguracaoInvalida` | chave inválida devolve erro |
| `TestRunnerStartStop` | primeiro tick imediato, `Stop` espera e é idempotente |
| `TestSendNowCriaRunDoDia` | envio manual cria a run do dia, marca item/chunk e registra `delivery.manual`; tick seguinte não repete |
| `TestSendNowRunFechadaNaoReenvia` | segundo clique no mesmo dia devolve `ErrJaEnviadoHoje` |
| `TestSendNowSemTrechosNaoCriaRun` | sem chunk pendente, nenhuma run é criada |
| `TestSendNowFalhaRemoveRunCriada` | falha de publicação descarta a run criada e permite nova tentativa |
| `TestSendNowSemNtfy` | sem URL/tópico devolve `ErrNtfyNaoConfigurado` sem criar run |
| `TestSendNowDocumentosInvalidos` | id inexistente, id zero e documento em `processing` |
| `TestSendNowReaproveitaItemFalho` | item `failed` é reenviado; run de tick anterior segue `running` |
| `TestSendNowComLockOcupado` | `TryLock` ocupado devolve `ErrEnvioEmAndamento` |
| `TestSendTest` / `TestSendTestSemNtfy` | mensagem de teste publicada e erro propagado |

Configuração: `TestLoadSettingsPadroes`, `TestLoadSettingsLeValores` e uma tabela de valores inválidos por chave. `Parse` valida o mesmo mapa que a UI grava (`TestConfiguracoesSalva`, `TestConfiguracoesValorInvalidoNaoGrava` em `internal/web`).
