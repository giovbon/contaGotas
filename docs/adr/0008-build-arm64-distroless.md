---
title: ADR-0008 — Imagem ARM64 distroless
status: aceito
date: 2026-10-26
---

# ADR-0008 — Imagem ARM64 distroless

## Contexto

O alvo é ARM64 (SBC ou VPS Ampere). O container precisa ser pequeno, não-root, somente leitura e sem shell, mantendo CGO para PDF.

## Decisão

- Build multi-stage: Tailwind (Node) → Go + templ + CGO → runtime **distroless static** non-root.
- `CGO_ENABLED=1`, `GOOS=linux`, `GOARCH=arm64`, link estático (`-linkmode external -extldflags '-static'`).
- Flags: `-trimpath -buildvcs=false -tags netgo,osusergo -ldflags "-s -w"`.
- Runtime: `read_only: true`, `tmpfs: /tmp`, volume `/data` com owner 65532, `GOMEMLIMIT=96MiB`, `GOGC=50`, `GOMAXPROCS=2`.
- Healthcheck via subcomando do próprio binário (`/readingdrip healthcheck`), já que não há shell nem `curl`.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| `alpine` + musl | libs do MuPDF são glibc; risco de incompatibilidade |
| `debian-slim` | +30 MB e superfície de pacotes desnecessária |
| `scratch` | sem CA certs/tzdata; exige copiar manualmente |
| Build sem CGO | perde PDF |
| qemu para build ARM64 | lento; preferir runner nativo |

## Consequências

### Positivas
- Imagem ~60–90 MB, superfície mínima, sem shell para atacante.
- Rootfs imutável; estado só em `/data`.
- Start rápido e consumo previsível.

### Negativas / riscos
- Cross-compile CGO é sensível (versão de glibc/toolchain).
- Sem shell dificulta debug dentro do container.
- Distroless exige copiar `tzdata` ou importar `time/tzdata`.

### Mitigações
- Runner ARM64 nativo no CI; cross como fallback documentado (`SPIKE-03`).
- `import _ "time/tzdata"` no binário.
- Debug por logs estruturados e `docker cp` do banco.
- Teste de fumaça do healthcheck no pipeline.
