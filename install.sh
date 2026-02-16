#!/bin/sh
# Oculo Installer
# https://github.com/Mr-Dark-debug/Oculo
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Mr-Dark-debug/Oculo/main/install.sh | sh
#
# Environment variables:
#   OCULO_INSTALL_DIR  — installation directory (default: ~/.local/bin)
#   OCULO_VERSION      — version to install     (default: latest)

set -e

# ──────────────────────────────────────────────
# Configuration
# ──────────────────────────────────────────────

REPO="Mr-Dark-debug/Oculo"
BINARY_NAME="oculo"
INSTALL_DIR="${OCULO_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${OCULO_VERSION:-latest}"
GITHUB_API="https://api.github.com/repos/${REPO}"
GITHUB_RELEASES="https://github.com/${REPO}/releases"

# ──────────────────────────────────────────────
# Colors (only when stdout is a terminal)
# ──────────────────────────────────────────────

if [ -t 1 ]; then
    BOLD="\033[1m"
    DIM="\033[2m"
    BLUE="\033[38;5;75m"
    GREEN="\033[38;5;78m"
    RED="\033[38;5;203m"
    YELLOW="\033[38;5;214m"
    RESET="\033[0m"
else
    BOLD="" DIM="" BLUE="" GREEN="" RED="" YELLOW="" RESET=""
fi

# ──────────────────────────────────────────────
# Helpers
# ──────────────────────────────────────────────

info()  { printf "${BLUE}${BOLD}info${RESET}  %s\n" "$1"; }
ok()    { printf "${GREEN}${BOLD}  ok${RESET}  %s\n" "$1"; }
warn()  { printf "${YELLOW}${BOLD}warn${RESET}  %s\n" "$1"; }
fail()  { printf "${RED}${BOLD}fail${RESET}  %s\n" "$1"; exit 1; }

# ──────────────────────────────────────────────
# Detect platform
# ──────────────────────────────────────────────

detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"

    case "$OS" in
        linux)   OS="linux" ;;
        darwin)  OS="darwin" ;;
        mingw*|msys*|cygwin*) OS="windows" ;;
        *)       fail "Unsupported operating system: $OS" ;;
    esac

    case "$ARCH" in
        x86_64|amd64)    ARCH="amd64" ;;
        aarch64|arm64)   ARCH="arm64" ;;
        armv7l|armv6l)   ARCH="arm" ;;
        *)               fail "Unsupported architecture: $ARCH" ;;
    esac

    PLATFORM="${OS}_${ARCH}"
    info "Detected platform: ${BOLD}${PLATFORM}${RESET}"
}

# ──────────────────────────────────────────────
# Check dependencies
# ──────────────────────────────────────────────

check_deps() {
    for cmd in curl tar; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            fail "Required command not found: ${BOLD}$cmd${RESET}"
        fi
    done
}

# ──────────────────────────────────────────────
# Resolve version
# ──────────────────────────────────────────────

resolve_version() {
    if [ "$VERSION" = "latest" ]; then
        info "Fetching latest release..."
        VERSION=$(curl -fsSL "${GITHUB_API}/releases/latest" 2>/dev/null | \
            grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

        if [ -z "$VERSION" ]; then
            # Fallback: try listing tags
            VERSION=$(curl -fsSL "${GITHUB_API}/tags" 2>/dev/null | \
                grep '"name"' | head -1 | sed 's/.*"name": *"\([^"]*\)".*/\1/')
        fi

        if [ -z "$VERSION" ]; then
            fail "Could not determine latest version. Set OCULO_VERSION manually."
        fi
    fi

    # Strip leading 'v' for consistency in asset naming
    VERSION_NUM="${VERSION#v}"
    info "Installing version: ${BOLD}${VERSION}${RESET}"
}

# ──────────────────────────────────────────────
# Download and install
# ──────────────────────────────────────────────

download_and_install() {
    # Construct asset name
    EXT="tar.gz"
    if [ "$OS" = "windows" ]; then
        EXT="zip"
    fi

    ASSET_NAME="oculo_${VERSION_NUM}_${PLATFORM}.${EXT}"
    DOWNLOAD_URL="${GITHUB_RELEASES}/download/${VERSION}/${ASSET_NAME}"

    info "Downloading: ${DIM}${DOWNLOAD_URL}${RESET}"

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    HTTP_CODE=$(curl -fsSL -w "%{http_code}" -o "${TMPDIR}/${ASSET_NAME}" "$DOWNLOAD_URL" 2>/dev/null || true)

    if [ ! -f "${TMPDIR}/${ASSET_NAME}" ] || [ "$HTTP_CODE" = "404" ]; then
        warn "Pre-built binary not found for ${PLATFORM}."
        warn ""
        warn "To build from source instead:"
        warn "  git clone https://github.com/${REPO}.git"
        warn "  cd Oculo"
        warn "  make build"
        warn ""
        warn "Or install with Go:"
        warn "  go install -tags fts5 github.com/${REPO}/cmd/oculo@${VERSION}"
        warn "  go install -tags fts5 github.com/${REPO}/cmd/oculo-daemon@${VERSION}"
        warn "  go install -tags fts5 github.com/${REPO}/cmd/oculo-tui@${VERSION}"
        exit 1
    fi

    info "Extracting..."
    cd "$TMPDIR"

    if [ "$EXT" = "tar.gz" ]; then
        tar xzf "$ASSET_NAME"
    else
        unzip -q "$ASSET_NAME"
    fi

    # Create install directory
    mkdir -p "$INSTALL_DIR"

    # Find and install all binaries
    INSTALLED=0
    for bin in oculo oculo-daemon oculo-tui; do
        FOUND=$(find "$TMPDIR" -name "$bin" -o -name "${bin}.exe" 2>/dev/null | head -1)
        if [ -n "$FOUND" ]; then
            chmod +x "$FOUND"
            mv "$FOUND" "${INSTALL_DIR}/"
            ok "Installed ${BOLD}${bin}${RESET} → ${DIM}${INSTALL_DIR}/${bin}${RESET}"
            INSTALLED=$((INSTALLED + 1))
        fi
    done

    if [ "$INSTALLED" -eq 0 ]; then
        fail "No binaries found in the release archive."
    fi
}

# ──────────────────────────────────────────────
# Post-install
# ──────────────────────────────────────────────

post_install() {
    echo ""

    # Check if install dir is in PATH
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            warn "${INSTALL_DIR} is not in your PATH."
            echo ""
            printf "  Add it by running:\n"
            echo ""
            SHELL_NAME=$(basename "$SHELL" 2>/dev/null || echo "sh")
            case "$SHELL_NAME" in
                zsh)
                    printf "    ${DIM}echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc${RESET}\n"
                    printf "    ${DIM}source ~/.zshrc${RESET}\n"
                    ;;
                fish)
                    printf "    ${DIM}fish_add_path ~/.local/bin${RESET}\n"
                    ;;
                *)
                    printf "    ${DIM}echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc${RESET}\n"
                    printf "    ${DIM}source ~/.bashrc${RESET}\n"
                    ;;
            esac
            echo ""
            ;;
    esac

    printf "${GREEN}${BOLD}Oculo installed successfully.${RESET}\n"
    echo ""
    printf "  ${DIM}Start the daemon:${RESET}    oculo-daemon\n"
    printf "  ${DIM}Open the debugger:${RESET}   oculo-tui\n"
    printf "  ${DIM}Run analysis:${RESET}        oculo analyze <trace-id>\n"
    printf "  ${DIM}Install Python SDK:${RESET}  pip install oculo-sdk\n"
    echo ""
}

# ──────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────

main() {
    echo ""
    printf "${BOLD}Oculo Installer${RESET}\n"
    printf "${DIM}The Glass Box for AI Agents${RESET}\n"
    echo ""

    check_deps
    detect_platform
    resolve_version
    download_and_install
    post_install
}

main
