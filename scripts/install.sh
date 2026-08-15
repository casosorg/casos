#!/usr/bin/env bash
# CasOS one-step installer, adapted from OpenAgent's Apache-2.0 installer.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash
#
# Optional environment variables:
#   CASOS_VERSION  release tag such as v1.32.0 (default: latest)
#   CASOS_REPOSITORY GitHub release repository (default: casosorg/casos)
#   INSTALL_DIR    binary directory (default: $HOME/.local/bin)

set -euo pipefail

REPO="${CASOS_REPOSITORY:-casosorg/casos}"
CASOS_VERSION="${CASOS_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

info() { printf '%s\n' "$*"; }
die() { printf '[casos] %s\n' "$*" >&2; exit 1; }

[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "invalid GitHub repository: $REPO"

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

need_cmd curl

[[ "$INSTALL_DIR" != *:* ]] || die "INSTALL_DIR must not contain a PATH separator (:)"

if [[ "$CASOS_VERSION" == "latest" ]]; then
	RELEASE_URL="https://github.com/${REPO}/releases/latest/download"
else
	[[ "$CASOS_VERSION" =~ ^v[0-9A-Za-z._-]+$ ]] || die "invalid release version: $CASOS_VERSION"
	RELEASE_URL="https://github.com/${REPO}/releases/download/${CASOS_VERSION}"
fi

case "$(uname -s)" in
	Linux) OS_NAME="linux" ;;
	*) die "unsupported operating system; download manually from https://github.com/${REPO}/releases" ;;
esac

case "$(uname -m)" in
	x86_64|amd64) ARCH_NAME="amd64" ;;
	aarch64|arm64) ARCH_NAME="arm64" ;;
	*) die "unsupported architecture; download manually from https://github.com/${REPO}/releases" ;;
esac

FILENAME="casos_${OS_NAME}_${ARCH_NAME}"
TEMP_DIR="$(mktemp -d)"
PENDING=""
cleanup() {
	rm -rf "$TEMP_DIR"
	[[ -z "$PENDING" ]] || rm -f "$PENDING"
}
trap cleanup EXIT

info "Downloading CasOS ${CASOS_VERSION}..."
curl -fsSL -o "$TEMP_DIR/$FILENAME" "$RELEASE_URL/$FILENAME"
curl -fsSL -o "$TEMP_DIR/SHA256SUMS" "$RELEASE_URL/SHA256SUMS"

EXPECTED="$(grep -E "^[0-9a-fA-F]{64}[[:space:]]+\\*?${FILENAME}$" "$TEMP_DIR/SHA256SUMS" | head -1 | awk '{print tolower($1)}' || true)"
[[ -n "$EXPECTED" ]] || die "release checksum for $FILENAME was not found"
if command -v sha256sum >/dev/null 2>&1; then
	ACTUAL="$(sha256sum "$TEMP_DIR/$FILENAME" | awk '{print tolower($1)}')"
elif command -v shasum >/dev/null 2>&1; then
	ACTUAL="$(shasum -a 256 "$TEMP_DIR/$FILENAME" | awk '{print tolower($1)}')"
else
	die "required command not found: sha256sum or shasum"
fi
[[ "$ACTUAL" == "$EXPECTED" ]] || die "download checksum verification failed"

mkdir -p "$INSTALL_DIR"
INSTALL_DIR="$(cd "$INSTALL_DIR" && pwd -P)"
[[ -w "$INSTALL_DIR" ]] || die "installation directory is not writable: $INSTALL_DIR"

PENDING="$INSTALL_DIR/.casos.new.$$"
cp "$TEMP_DIR/$FILENAME" "$PENDING"
chmod 755 "$PENDING"
mv -f "$PENDING" "$INSTALL_DIR/casos"
PENDING=""

SHELL_RC=""
LOGIN_SHELL="${SHELL:-}"
if command -v getent >/dev/null 2>&1; then
	DETECTED_LOGIN_SHELL="$(getent passwd "$(id -un)" | cut -d: -f7 || true)"
	[[ -z "$DETECTED_LOGIN_SHELL" ]] || LOGIN_SHELL="$DETECTED_LOGIN_SHELL"
fi
case "$LOGIN_SHELL" in
	*/zsh) SHELL_RC="$HOME/.zshrc" ;;
	*/bash) SHELL_RC="$HOME/.bashrc" ;;
esac
PATH_LINE="$(printf 'export PATH=%q:"$PATH"' "$INSTALL_DIR")"
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]] && [[ -n "$SHELL_RC" ]] && \
	! grep -Fq "$PATH_LINE" "$SHELL_RC" 2>/dev/null; then
	printf '\n%s\n' "$PATH_LINE" >> "$SHELL_RC"
	info "Added $INSTALL_DIR to PATH in $SHELL_RC."
fi

info "CasOS ${CASOS_VERSION} installed at $INSTALL_DIR/casos"
if [[ ":$PATH:" == *":$INSTALL_DIR:"* ]]; then
	info "Run 'casos' and open http://localhost:9000"
elif [[ -n "$SHELL_RC" ]]; then
	SOURCE_COMMAND="$(printf 'source %q' "$SHELL_RC")"
	info "Open a new shell or run: $SOURCE_COMMAND"
	info "Then run 'casos' and open http://localhost:9000"
	info "You can also run '$INSTALL_DIR/casos' directly."
else
	info "Add $INSTALL_DIR to PATH for your shell, or run '$INSTALL_DIR/casos' directly."
	info "Then open http://localhost:9000"
fi
