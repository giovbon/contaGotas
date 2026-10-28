---
title: Operações
status: ativo
updated: 2026-10-27
---

# Operações

Build, deploy, backup e diagnóstico. Alvo: uma instância ARM64 atrás de Cloudflare Tunnel, com no máximo 200 MiB de RAM.

## Pré-requisitos

| Item | Versão |
|---|---|
| Go | 1.26+ (templ exige) |
| Docker + buildx | 24+ |
| templ | CLI via `go run` (pinada em `v0.3.1070`) |
| ntfy | self-hosted, `message-size-limit: 64k` |
| Cloudflare | domínio + tunnel |

## Build local

```sh
# necessário apenas depois de editar arquivos .templ (os _templ.go são versionados)
go run github.com/a-h/templ/cmd/templ@v0.3.1070 generate ./internal/web/views

CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o bin/readingdrip ./cmd/app

APP_PASSWORD='senha-forte' \
APP_SECRET="$(head -c 32 /dev/urandom | base64)" \
APP_DATA_DIR=./data \
APP_BASE_URL=http://localhost:8080 \
./bin/readingdrip
```

Sem CGO, o binário compila mas retorna erro ao importar PDF. MD/EPUB funcionam.

### Execução local rápida (`run.sh`)

```sh
./run.sh            # http://localhost:8473
PORT=9000 ./run.sh  # outra porta
```

O script usa a porta dedicada **8473**, compila `./cmd/app` com `CGO_ENABLED=1` e guarda dados em `./data`. Na primeira execução gera `APP_PASSWORD` e `APP_SECRET` aleatórios em `.env.local` (chmod 600, fora do Git); as variáveis de ambiente têm prioridade e não são persistidas. `.env.local`, `./data` e `./bin` estão no `.gitignore`. Se a porta estiver ocupada, o script falha antes de compilar e sugere `PORT=...`.

O entrypoint HTTP é `cmd/app`; ele exige `APP_PASSWORD` e `APP_SECRET` (32+ bytes) e cria `{APP_DATA_DIR}/db/app.db` e `{APP_DATA_DIR}/documents`. A CLI headless continua em `cmd/readingdrip` (`plan`, `version`).

## Dockerfile (referência)

```dockerfile
# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS TARGETARCH
WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
      gcc-aarch64-linux-gnu libc6-dev-arm64-cross \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go run github.com/a-h/templ/cmd/templ@v0.3.1070 generate ./internal/web/views

RUN CGO_ENABLED=1 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    CC=aarch64-linux-gnu-gcc \
    go build -trimpath -buildvcs=false -tags "netgo,osusergo" \
      -ldflags "-s -w -linkmode external -extldflags '-static'" \
      -o /out/readingdrip ./cmd/app

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/readingdrip /readingdrip
ENV TZ=America/Sao_Paulo \
    GOMEMLIMIT=96MiB \
    GOGC=50 \
    GOMAXPROCS=2 \
    APP_DATA_DIR=/data \
    APP_ADDR=:8080
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD ["/readingdrip", "healthcheck"]
ENTRYPOINT ["/readingdrip"]
```

Notas:
- `-linkmode external -extldflags -static` com `netgo` evita dependência de libc no runtime.
- Preferir **build nativo em runner ARM64** (rápido e sem surpresas de cross). O cross acima é o caminho de referência; validar no CI (`SPIKE-03`).
- `templ` é pinado em `v0.3.1070`. CSS e JS próprios são embutidos via `go:embed` (`internal/web/assets`), então o build não precisa de Node ([ADR-0014](./adr/0014-ui-css-js-proprios.md)).
- Não há shell no runtime: healthcheck usa o próprio binário.

## Compose de referência

```yaml
services:
  app:
    image: ghcr.io/<user>/readingdrip:latest
    restart: unless-stopped
    read_only: true
    tmpfs:
      - /tmp
    volumes:
      - ./data:/data
    environment:
      APP_BASE_URL: "https://leitura.exemplo.com"
      APP_PASSWORD: "${APP_PASSWORD}"
      APP_SECRET: "${APP_SECRET}"          # 32+ bytes aleatórios
      BEHIND_CLOUDFLARE: "true"
      MAX_UPLOAD_BYTES: "99614720"          # 95 MiB
    mem_limit: 256m
    cpus: "1.5"
    ports:
      - "127.0.0.1:8080:8080"

  cloudflared:
    image: cloudflare/cloudflared:latest
    restart: unless-stopped
    command: tunnel --no-autoupdate run --token ${CF_TUNNEL_TOKEN}
    depends_on: [app]

  ntfy:
    image: binwiederhier/ntfy:latest
    restart: unless-stopped
    command: serve
    volumes:
      - ./ntfy/server.yml:/etc/ntfy/server.yml:ro
      - ./ntfy/cache:/var/cache/ntfy
    environment:
      TZ: America/Sao_Paulo
```

Permissões no volume (distroless nonroot, uid 65532):

```sh
sudo chown -R 65532:65532 ./data
```

## Cloudflare Tunnel

1. Criar o túnel no dashboard Zero Trust.
2. Public hostname: `leitura.exemplo.com` → `http://app:8080`.
3. Copiar o token para `CF_TUNNEL_TOKEN`.
4. Não abrir portas no firewall.
5. Em `APP_BASE_URL`, usar sempre `https://`.

Se usar Cloudflare Access:
- proteger `/`, `/documents/*`, `/settings`, `/history`;
- **bypass** em `/r/*`, `/api/actions/*`, `/media/*`, `/healthz`, `/readyz`;
- ou não usar Access e ficar só com a auth do app (`Q-003`).

## ntfy

`ntfy/server.yml`:

```yaml
base-url: "https://ntfy.exemplo.com"
listen-http: ":8080"
cache-file: "/var/cache/ntfy/cache.db"
cache-duration: "48h"
message-size-limit: "64k"
behind-proxy: true
```

Publicar `ntfy.exemplo.com` pelo mesmo túnel (hostname separado) ou por outro túnel. O app usa `http://ntfy:8080` internamente.

## Variáveis de ambiente

| Variável | Padrão | Obrigatória em prod |
|---|---|---|
| `APP_ADDR` | `:8080` | — |
| `APP_DATA_DIR` | `/data` | — |
| `APP_BASE_URL` | `http://localhost:8080` | **sim** |
| `APP_PASSWORD` | — | **sim** |
| `APP_SECRET` | — | **sim** (32+ bytes) |
| `BEHIND_CLOUDFLARE` | `false` | recomendado |
| `MAX_UPLOAD_BYTES` | `99614720` | — |
| `TZ` | `UTC` | recomendado |
| `GOMEMLIMIT` | `96MiB` | — |
| `GOGC` | `50` | — |
| `GOMAXPROCS` | `2` | — |
| `LOG_LEVEL` | `info` | — |

`APP_SECRET` assina o cookie de sessão `rd_session` e os links do leitor; trocar `APP_SECRET` **ou** `APP_PASSWORD` invalida todas as sessões e links. O rate limit de login fica em memória (5 falhas/min por IP) e zera quando o app reinicia; com `BEHIND_CLOUDFLARE=true`, o IP vem de `CF-Connecting-IP`. Ver [ADR-0013](./adr/0013-sessao-hmac-argon2id.md).

## Backup

Diário (cron do host ou sidecar):

```sh
sqlite3 /data/db/app.db "VACUUM INTO '/data/backup/app-$(date +%F).db'"
tar -C /data -czf /data/backup/documents-$(date +%F).tar.gz documents
find /data/backup -type f -mtime +14 -delete
```

- `VACUUM INTO` é seguro com WAL e não bloqueia leituras.
- Testar restore trimestralmente.

## Restore

```sh
docker compose stop app
cp /data/backup/app-2026-10-25.db /data/db/app.db
rm -f /data/db/app.db-wal /data/db/app.db-shm
tar -C /data -xzf /data/backup/documents-2026-10-25.tar.gz
docker compose start app
```

Migrações pendentes rodam no start. Conferir `/history` e `/documents`.

## Upgrade

1. Backup.
2. `docker compose pull && docker compose up -d`.
3. O app copia o banco para `backup/pre-migrate-{versao}.db` e aplica migrações.
4. Se o start falhar, o log indica a migração; restaurar o backup e abrir issue.

Migrações são **forward-only**; downgrade não é suportado.

## Diagnóstico

| Sintoma | Causa provável | Ação |
|---|---|---|
| 502 na Cloudflare | app fora ou túnel caído | `docker compose ps`, logs do `cloudflared` |
| 413 no upload | arquivo > limite Cloudflare/app | reduzir `MAX_UPLOAD_BYTES` e avisar |
| Mensagem chega cortada | `message-size-limit` do ntfy em 4k | elevar para 64k e reiniciar ntfy |
| `needs_ocr` | PDF escaneado | converter com OCR antes de subir |
| PDF sem sentido | 3+ colunas / layout complexo | reportar fixture; ajustar heurística |
| Nada é enviado | dia não configurado / agendador off / hora | conferir `/settings` e `/history` |
| Envio duplicado | restart entre 2xx e commit | aceitável; documentado em `RNF-012` |
| `database is locked` | backup sem WAL / duas instâncias | usar `VACUUM INTO`; garantir 1 réplica |
| Link expirado | TTL ou `APP_SECRET` rotacionado | regenerar pelo leitor autenticado |
| Login 429 | 5 falhas no último minuto | aguardar `Retry-After`; conferir `APP_PASSWORD` |
| Sessão invalidada | `APP_SECRET`/`APP_PASSWORD` trocados ou cookie expirado | entrar novamente |
| Hora errada | timezone da agenda | conferir `schedule.timezone` (não `TZ`) |

## Checklist de segurança

- [ ] `APP_PASSWORD` forte e exclusiva.
- [ ] `APP_SECRET` aleatório (32+ bytes), fora do Git.
- [ ] Token do ntfy com permissão só de publish no topic.
- [ ] Cloudflare Access ou auth do app ativa; bypass só nas rotas necessárias.
- [ ] `read_only: true` + `tmpfs` no container.
- [ ] Volume `/data` com owner 65532.
- [ ] Backup criptografado se sair do host.
- [ ] Logs sem corpo de mensagem e sem tokens.
- [ ] `X-Robots-Tag: noindex` nas rotas privadas.
- [ ] Atualização de dependências revisada (CVE).

## Limites de recurso

| Recurso | Limite |
|---|---|
| `mem_limit` | 256 MiB (deixa margem para pico de PDF) |
| `GOMEMLIMIT` | 96 MiB |
| `cpus` | 1.5 |
| Disco | ~1,2× o tamanho dos originais + mídia + backups |
