#!/usr/bin/env bash
# Sobe o Reading Drip em modo local (desenvolvimento).
#
# - porta dedicada 8473 (evita as portas comuns de dev e do ntfy);
# - dados em ./data (ignorado pelo Git);
# - APP_PASSWORD/APP_SECRET geradas uma vez e guardadas em .env.local;
# - compila ./cmd/app (CGO ligado para extração de PDF) e executa o binário.
#
# Uso:
#   ./run.sh                 # porta 8473
#   PORT=9000 ./run.sh       # outra porta
#   ./run.sh 9000            # idem
#   APP_PASSWORD=... APP_SECRET=... ./run.sh
set -euo pipefail

RAIZ="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$RAIZ"

PORTA_PADRAO=8473
PORTA="${1:-${PORT:-$PORTA_PADRAO}}"
ARQUIVO_ENV="$RAIZ/.env.local"
DIR_DADOS="${APP_DATA_DIR:-$RAIZ/data}"
BINARIO="$RAIZ/bin/readingdrip"

uso() {
	cat <<'EOF'
Uso: ./run.sh [porta]

Variáveis:
  PORT                  porta HTTP (padrão: 8473)
  APP_DATA_DIR          diretório de dados (padrão: ./data)
  APP_PASSWORD          senha de login (padrão: gerada em .env.local)
  APP_SECRET            segredo de assinatura (padrão: gerado em .env.local)
  APP_BASE_URL          URL pública (padrão: http://localhost:8473)
  CGO_ENABLED           padrão: 1 (necessário para PDF)
  TZ                    padrão: America/Sao_Paulo

Credenciais geradas ficam em .env.local (chmod 600, fora do Git).
EOF
}

case "${1:-}" in
-h | --help)
	uso
	exit 0
	;;
esac

if [[ ! "$PORTA" =~ ^[0-9]+$ ]] || ((PORTA < 1 || PORTA > 65535)); then
	echo "run.sh: porta inválida: $PORTA" >&2
	exit 1
fi

if ! command -v go >/dev/null 2>&1; then
	echo "run.sh: Go não encontrado no PATH" >&2
	exit 1
fi

# Gera hex a partir de /dev/urandom; openssl é só um atalho.
gerar_hex() {
	local bytes="$1"
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex "$bytes"
	else
		head -c "$bytes" /dev/urandom | od -An -tx1 | tr -d ' \n'
	fi
}

ler_env() {
	local chave="$1" valor
	[[ -f "$ARQUIVO_ENV" ]] || return 1
	valor="$(sed -n "s/^${chave}=//p" "$ARQUIVO_ENV" | tail -n 1)"
	[[ -n "$valor" ]] || return 1
	printf '%s' "$valor"
}

senha_do_ambiente=0
segredo_do_ambiente=0
[[ -n "${APP_PASSWORD:-}" ]] && senha_do_ambiente=1
[[ -n "${APP_SECRET:-}" ]] && segredo_do_ambiente=1

if [[ "$senha_do_ambiente" -eq 0 ]]; then
	APP_PASSWORD="$(ler_env APP_PASSWORD || true)"
fi
if [[ "$segredo_do_ambiente" -eq 0 ]]; then
	APP_SECRET="$(ler_env APP_SECRET || true)"
fi

gerou_senha=0
gerou_segredo=0
if [[ -z "${APP_PASSWORD:-}" ]]; then
	APP_PASSWORD="$(gerar_hex 12)"
	gerou_senha=1
fi
if [[ -z "${APP_SECRET:-}" ]]; then
	APP_SECRET="$(gerar_hex 32)"
	gerou_segredo=1
fi

# Só grava quando nenhuma credencial veio do ambiente, para não persistir
# segredos que o usuário passou de fora.
if [[ "$senha_do_ambiente" -eq 0 && "$segredo_do_ambiente" -eq 0 ]]; then
	if [[ "$gerou_senha" -eq 1 || "$gerou_segredo" -eq 1 || ! -f "$ARQUIVO_ENV" ]]; then
		(
			umask 077
			cat >"$ARQUIVO_ENV" <<EOF
# Gerado por ./run.sh. Não versionar.
APP_PASSWORD=$APP_PASSWORD
APP_SECRET=$APP_SECRET
EOF
		)
		chmod 600 "$ARQUIVO_ENV"
	fi
fi

export APP_PASSWORD APP_SECRET
export APP_ADDR=":$PORTA"
export APP_BASE_URL="${APP_BASE_URL:-http://localhost:$PORTA}"
export APP_DATA_DIR="$DIR_DADOS"
export CGO_ENABLED="${CGO_ENABLED:-1}"
export TZ="${TZ:-America/Sao_Paulo}"

mkdir -p "$APP_DATA_DIR"

porta_ocupada() {
	local porta="$1"
	if command -v ss >/dev/null 2>&1; then
		ss -ltn 2>/dev/null | awk -v alvo=":$porta" 'NR > 1 && $4 ~ alvo "$" { achou = 1 } END { exit !achou }'
	else
		(exec 3<>"/dev/tcp/127.0.0.1/$porta") 2>/dev/null
	fi
}

if porta_ocupada "$PORTA"; then
	echo "run.sh: a porta $PORTA já está em uso." >&2
	echo "        use outra: PORT=9000 ./run.sh" >&2
	exit 1
fi

echo "Reading Drip"
echo "  URL:   $APP_BASE_URL"
echo "  dados: $APP_DATA_DIR"
if [[ "$gerou_senha" -eq 1 ]]; then
	echo "  senha: $APP_PASSWORD (gerada agora; salva em .env.local)"
elif [[ "$senha_do_ambiente" -eq 0 ]]; then
	echo "  senha: em .env.local"
else
	echo "  senha: definida por APP_PASSWORD"
fi
if [[ "$gerou_segredo" -eq 1 && ( "$senha_do_ambiente" -eq 1 || "$segredo_do_ambiente" -eq 1 ) ]]; then
	echo "  aviso: APP_SECRET gerado não será persistido; defina-o no ambiente." >&2
fi

echo "compilando ./cmd/app..."
mkdir -p "$(dirname -- "$BINARIO")"
go build -trimpath -o "$BINARIO" ./cmd/app

echo "iniciando..."
exec "$BINARIO"
