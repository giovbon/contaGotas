---
title: Operações
status: ativo
updated: 2026-10-26
---

# Operações

Build, deploy, backup e diagnóstico. Alvo: uma instância ARM64 atrás de Cloudflare Tunnel, com no máximo 200 MiB de RAM.

## Pré-requisitos

| Item | Versão |
|---|---|
| Go | 1.24+ |
| Docker + buildx | 24+ |
| templ | CLI via `go run` (pinada) |
| Node | 22 (apenas build do CSS) |
| ntfy | self-hosted, `message-size-limit: 64k` |
| Cloudflare | domínio + tunnel |

## Build local

```sh
go run github.com/a-h/templ/cmd/templ@v0.3.900 generate
npx @tailwindcss/cli@4 -i web/static/input.css -o web/static/app.css --minify
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o bin/readingdrip ./cmd/app
./bin/readingdrip -config ./config.yml
```

Sem CGO, o binário compila mas retorna erro ao importar PDF (`SPIKE-01`). MD/EPUB funcionam.

## Dockerfile (referência)

```dockerfile
# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM node:22-alpine AS css
WORKDIR /app
COPY web/ ./web/
RUN npx --yes @tailwindcss/cli@4 -i ./web/static/input.css -o ./web/static/app.css --minify

FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS build
ARG TARGETOS TARGETARCH
WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
      gcc-aarch64-linux-gnu libc6-dev-arm64-cross \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=css /app/web/static/app.css ./web/static/app.css

RUN go run github.com/a-h/templ/cmd/templ@v0.3.900 generate

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
- `templ` e Tailwind são pinados. Alpine.js é vendorizado em `web/static/alpine.min.js` (versão + SHA no repositório).
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
