---
title: Operações
status: ativo
updated: 2026-10-30
---

# Operações

Build, deploy, backup e diagnóstico. Alvo: uma instância ARM64 atrás de Cloudflare Tunnel, com no máximo 200 MiB de RAM.

## Pré-requisitos

| Item | Versão |
|---|---|
| Go | 1.26+ (templ exige) |
| Docker + buildx | 24+ (Docker Compose **v2** para o `compose.yaml`) |
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

## Imagem (`Dockerfile`)

O `/Dockerfile` implementa o [ADR-0008](./adr/0008-build-arm64-distroless.md): build multi-stage com CGO (MuPDF), link estático e runtime `distroless/static-debian12:nonroot`. Os `_templ.go` são gerados no build para impedir que uma view alterada sem regenerar entre na imagem.

O estágio de build usa `golang:${GO_VERSION}-trixie` (glibc 2.41). **Não volte para bookworm**: os objetos pré-compilados do `go-fitz` referenciam `__isoc23_strtol`/`__isoc23_strtoul`, que não existem na libc 2.36 e quebram o link com `undefined reference`. O runtime continua distroless static, sem libc.

```sh
# cross-compile ARM64 a partir de amd64 (caminho de referência)
docker buildx build --platform linux/arm64 -t readingdrip:arm64 .

# runner ARM64 nativo (recomendado no CI; dispensa o toolchain de cross)
docker buildx build --platform linux/arm64 --build-arg NATIVE=1 -t readingdrip:arm64 .

# com versão na label da imagem
docker buildx build --platform linux/arm64 --build-arg VERSION=1.0.0 -t readingdrip:1.0.0 .
```

Em host amd64, o cross exige o emulador de binfmt. O build em si roda nativo (o Go só usa o `aarch64-linux-gnu-gcc`), mas o smoke test do container ARM64 precisa dele:

```sh
docker run --privileged --rm tonistiigi/binfmt --install arm64
```

Argumentos de build:

| Argumento | Padrão | Uso |
|---|---|---|
| `GO_VERSION` | `1.27` | tag da imagem `golang` |
| `NATIVE` | `0` | `1` em runner ARM64 nativo; não instala o gcc de cross |
| `VERSION` | `dev` | label `org.opencontainers.image.version` |

Verificação depois do build:

```sh
docker image inspect readingdrip:arm64 --format '{{.Architecture}} {{.Size}}'
docker run -d --name rd-smoke --platform linux/arm64 \
  -e APP_PASSWORD=teste -e APP_SECRET="$(head -c 32 /dev/urandom | base64)" \
  -e APP_BASE_URL=http://localhost:8080 -v "$PWD/data:/data" readingdrip:arm64
docker inspect --format '{{.State.Health.Status}}' rd-smoke   # healthy
docker rm -f rd-smoke
```

Notas:
- `-linkmode external -extldflags '-static'` com `netgo,osusergo` evita dependência de libc no runtime.
- Preferir **build nativo em runner ARM64** (rápido e sem surpresas de cross). O cross acima é o caminho de referência; validado em 2026-10-29 (`SPIKE-03`).
- `cmd/app` importa `time/tzdata` e responde ao subcomando `healthcheck` (consulta `GET /readyz`), porque a imagem não tem shell, `curl` nem `wget`.
- CSS e JS são embutidos via `go:embed`; o build não precisa de Node ([ADR-0014](./adr/0014-ui-css-js-proprios.md)).

### Validação da imagem (2026-10-29)

O build cross ARM64 e o smoke test foram executados num host amd64 com Docker 29 + BuildKit e QEMU/binfmt:

| Verificação | Resultado |
|---|---|
| `docker buildx build --platform linux/arm64 --load .` | concluiu |
| arquitetura e tamanho | `arm64 linux`, **23.965.482 bytes (~22,9 MiB)** |
| container com `read_only` + `tmpfs /tmp` | `healthy` |
| `/healthz` e `/readyz` | `200` |
| upload `.md` → extração → plano | `processing → ready` em ~0,3 s; 6 blocos, 1 chunk, plano v1 |
| original preservado | `/data/documents/1/source.md`, owner `65532:65532` |

O primeiro build **falhou** com bookworm (glibc 2.36) por causa dos símbolos `__isoc23_*` do MuPDF; a troca do builder para trixie resolveu. O container rodou sob emulação ARM64, então os tempos acima não valem como medição de performance nativa (`SPIKE-04`).

## CI/CD (GitHub Actions)

`.github/workflows/docker.yml` testa, compila e publica a imagem ARM64 no Docker Hub a cada push na `main`, em tag `v*` e em PR (em PR só valida, sem push):

- **test** — `go test ./... -race`;
- **image** — buildx com `--platform linux/arm64` e push (cache GHA). Tags geradas: `latest` (main), `<branch>`, `<sha>` e a versão da tag `v*`; `VERSION` vira label da imagem.

Segredos do repositório: `DOCKERHUB_USERNAME` e `DOCKERHUB_TOKEN` (PAT com permissão de write no Docker Hub). Sem eles o login falha em push na `main`/tag; em PR o build usa o nome padrão `readingdrip` e não publica.

Para subir o que foi publicado, use o template [`/deploy/compose.template.yml`](../deploy/compose.template.yml): copie para o servidor, aponte `RD_IMAGE=<usuario>/readingdrip:latest` no `.env` e rode `docker compose up -d` (roteiro completo no cabeçalho do arquivo).

## Compose (`compose.yaml`)

O `/compose.yaml` sobe `app`, `cloudflared` e `ntfy`; `/deploy/ntfy/server.yml` é a configuração de referência do ntfy e `/.env.example` lista as variáveis. O `.env` fica fora do Git.

```sh
cp .env.example .env          # preencha APP_BASE_URL, APP_PASSWORD, APP_SECRET e CF_TUNNEL_TOKEN
mkdir -p data deploy/ntfy/cache
sudo chown -R 65532:65532 data
docker compose up -d --build
docker compose ps
docker compose logs -f app
```

O app fica em `127.0.0.1:8080` (não exposto na internet) e o `cloudflared` só inicia depois de o healthcheck do app ficar `healthy`. Para usar uma imagem publicada em vez de compilar:

```sh
RD_IMAGE=<usuario-dockerhub>/readingdrip:latest docker compose up -d --no-build
```

O arquivo usa a chave de topo `name:` e a sintaxe do Compose **v2**. O `docker-compose` v1 (Python) não o aceita; confira com `docker compose version` antes do deploy. A validação de 2026-10-29 cobriu a imagem e o container isolado (`read_only`, `tmpfs`, volume, healthcheck); o `compose.yaml` completo ainda depende do host com Compose v2 e das credenciais do Cloudflare.

Permissões no volume (distroless nonroot, uid 65532):

```sh
sudo chown -R 65532:65532 ./data
```

## Cloudflare Tunnel

1. Criar o túnel no dashboard Zero Trust.
2. Public hostname: `leitura.exemplo.com` → `http://app:8080`.
3. Public hostname do ntfy: `ntfy.exemplo.com` → `http://ntfy:8080` (opcional, se for expor o ntfy).
4. Copiar o token para `CF_TUNNEL_TOKEN` no `.env`.
5. Não abrir portas no firewall.
6. Em `APP_BASE_URL`, usar sempre `https://`.

Se usar Cloudflare Access:
- proteger `/`, `/documents/*`, `/settings`, `/history`;
- **bypass** em `/r/*`, `/api/actions/*`, `/media/*`, `/healthz`, `/readyz` e `/static/*` (senão a tela de login fica sem CSS/JS);
- ou não usar Access e ficar só com a auth do app (`Q-003`).

## ntfy

O arquivo de referência é [`deploy/ntfy/server.yml`](../deploy/ntfy/server.yml), montado pelo `compose.yaml`:

```yaml
base-url: "https://ntfy.exemplo.com"
listen-http: ":8080"
cache-file: "/var/cache/ntfy/cache.db"
cache-duration: "48h"
message-size-limit: "64k"
behind-proxy: true
# auth-file: "/var/cache/ntfy/auth.db"
# auth-default-access: "deny-all"
```

Publicar `ntfy.exemplo.com` pelo mesmo túnel (hostname separado) ou por outro túnel. O app usa `http://ntfy:8080` internamente; o cache fica em `deploy/ntfy/cache` (fora do Git).

Com autenticação ligada, crie o usuário e dê permissão de escrita só no tópico do app:

```sh
ntfy user add leitor
ntfy access leitor leitura write-only
```

No app, preencha `ntfy.username`/`ntfy.password` em `/settings` (HTTP Basic) **ou** `ntfy.token` (Bearer). Basic tem precedência; o app nunca loga nem renderiza nenhum dos dois. Detalhes em [`delivery-ntfy.md`](./delivery-ntfy.md#publicação).

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
- O app também grava um backup automático antes de qualquer migração pendente: `backup/pre-migrate-{versão-alvo}.db`. Banco novo não gera arquivo; se o mesmo upgrade for repetido, o backup existente é preservado.
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

1. Backup (recomendado mesmo com o backup automático; cobre também os originais).
2. `docker compose pull && docker compose up -d`.
3. Com migrações pendentes num banco existente, o app grava `backup/pre-migrate-{versão-alvo}.db` via `VACUUM INTO`, loga “backup pré-migração gravado” e só então aplica as migrações. Falha no backup aborta o start sem tocar no schema.
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
- [ ] Credencial do ntfy (token ou usuário/senha) com permissão só de publish no topic.
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
