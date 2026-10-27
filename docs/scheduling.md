---
title: Agendamento
status: ativo
updated: 2026-10-26
---

# Agendamento

Um ticker em processo verifica a cada 60 s se é hora de entregar. Sem cron externo, sem fila, sem dependência de relógio do host além de `time.Now`.

## Configuração

| Chave | Padrão | Descrição |
|---|---|---|
| `schedule.days` | `mon,tue,wed,thu,fri` | dias de leitura |
| `schedule.time` | `07:00` | horário local |
| `schedule.timezone` | `America/Sao_Paulo` | IANA |
| `schedule.catchup_days` | `1` | janela de recuperação |
| `schedule.max_docs_per_run` | `5` | documentos por run |
| `schedule.enabled` | `true` | liga/desliga o agendador |

## Ciclo

```go
func (s *Scheduler) tick(now time.Time) {
    loc, _ := time.LoadLocation(s.cfg.Timezone)
    local := now.In(loc)

    if !s.cfg.Enabled || !s.isReadingDay(local.Weekday()) {
        return
    }
    if local.Format("15:04") < s.cfg.Time {
        return
    }
    if s.runExists(local.Format("2006-01-02")) {
        return
    }
    s.startRun(local)
}
```

`startRun`:

1. `INSERT INTO delivery_runs(run_date, scheduled_for, started_at, late)` com `ON CONFLICT DO NOTHING`.
2. Se a inserção não criou linha (outra instância/tick venceu), retorna.
3. Seleciona documentos `ready`, ordenados por `priority, id`, limitados a `max_docs_per_run`.
4. Para cada documento, pega o próximo chunk `pending` do plano ativo.
5. Compõe e envia (ver [`delivery-ntfy.md`](./delivery-ntfy.md)).
6. Fecha a run: `sent` se todos os itens ok, `partial` se alguns falharam, `failed` se todos.

O tick é protegido por mutex; a run em si roda em goroutine para não segurar o ticker.

## Catch-up

Na inicialização e a cada tick:

- olha para trás até `catchup_days` dias;
- se existia dia de leitura sem run e o horário já passou, cria a run agora com `late=1`;
- envia **somente a run perdida mais recente** (não enfileira uma por dia);
- entrega única por documento (o próximo chunk pendente), não “os chunks de todos os dias perdidos”.

Exemplo: container desligado de sexta 07:00 a segunda 09:00, dias úteis:

| Dia | Resultado |
|---|---|
| sexta | run criada na segunda, `late=1` |
| sábado/domingo | não são dias de leitura |
| segunda | run normal, no horário (se ainda não passou) |

Se a run de segunda já existir, o catch-up de sexta ainda é criado (uma vez), com `late=1`. O histórico deixa claro o que aconteceu.

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

`POST /documents/{id}/send-now`:

- cria/adere a run do dia (se já enviada, cria uma run manual com `run_date = "manual:{timestamp}"`? **Não** — usa `run_date` normal e envia os itens restantes);
- não altera o cronograma;
- registra no histórico com `late=0` e um evento `delivery.manual`.

## Relógio

`internal/platform.Clock` injetável:

```go
type Clock interface{ Now() time.Time }
```

Testes usam `FakeClock`; produção usa `time.Now`. Nenhum código de agendamento chama `time.Now` diretamente.

## Testes

| Teste | Cenário |
|---|---|
| Dia de leitura no horário | cria run |
| Dia não configurado | não cria |
| Antes do horário | não cria |
| Duas execuções no mesmo dia | uma run |
| Container off por 1 dia | catch-up com `late=1` |
| Off por 5 dias | uma única run de catch-up |
| DST spring forward | dispara após o salto |
| DST fall back | um envio só |
| Pausa de documento | não entra na run, demais seguem |
| Falha de um documento | run `partial`, cursor do doc falho intacto |
| Mudança de dias | próximo chunk continua |
