# syntax=docker/dockerfile:1.7
#
# Imagem de produção do Reading Drip (ADR-0008): binário único com CGO
# (MuPDF/go-fitz), runtime distroless non-root e rootfs somente leitura.
#
# Build ARM64 a partir de amd64 (cross):
#   docker buildx build --platform linux/arm64 -t readingdrip:arm64 .
#
# Build nativo em runner ARM64 (recomendado no CI):
#   docker buildx build --platform linux/arm64 --build-arg NATIVE=1 -t readingdrip:arm64 .

ARG GO_VERSION=1.27
# trixie (glibc 2.41): os objetos pré-compilados do go-fitz usam símbolos
# __isoc23_* e não linkam com a libc 2.36 do bookworm.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-trixie AS build
ARG TARGETOS
ARG TARGETARCH
ARG NATIVE=0

WORKDIR /src

# Toolchain de cross para ARM64. Em runner ARM64 nativo (NATIVE=1) o gcc do
# próprio build é usado e o pacote de cross não é necessário.
RUN if [ "$NATIVE" != "1" ] && [ "$TARGETARCH" = "arm64" ]; then \
      apt-get update && apt-get install -y --no-install-recommends \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross \
      && rm -rf /var/lib/apt/lists/*; \
    fi

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Os _templ.go são versionados, mas gerar no build garante que uma view
# alterada sem regenerar não entre na imagem.
RUN go run github.com/a-h/templ/cmd/templ@v0.3.1070 generate ./internal/web/views

# Link estático para o runtime distroless static. O binário embute tzdata
# (cmd/app importa time/tzdata) e não depende de shell, curl ou libc.
RUN if [ "$TARGETARCH" = "arm64" ] && [ "$NATIVE" != "1" ]; then \
      export CC=aarch64-linux-gnu-gcc; \
    fi; \
    CGO_ENABLED=1 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -tags "netgo,osusergo" \
      -ldflags "-s -w -linkmode external -extldflags '-static'" \
      -o /out/readingdrip ./cmd/app

FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
LABEL org.opencontainers.image.title="Reading Drip" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/giovani/readingdrip"

COPY --from=build /out/readingdrip /readingdrip

ENV TZ=America/Sao_Paulo \
    GOMEMLIMIT=96MiB \
    GOGC=50 \
    GOMAXPROCS=2 \
    APP_DATA_DIR=/data \
    APP_ADDR=:8080

VOLUME ["/data"]
EXPOSE 8080

# Sem shell na imagem: o healthcheck é um subcomando do próprio binário e
# consulta /readyz em 127.0.0.1:${APP_ADDR}.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/readingdrip", "healthcheck"]

ENTRYPOINT ["/readingdrip"]
