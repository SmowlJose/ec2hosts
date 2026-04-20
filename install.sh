#!/usr/bin/env bash
# ec2hosts installer for Linux and macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/SmowlJose/ec2hosts/main/install.sh | bash
#
# Environment overrides:
#   EC2HOSTS_VERSION    release tag to install (default: latest)
#   EC2HOSTS_BIN_DIR    force the install directory
#   EC2HOSTS_NO_PATH    set to any value to skip PATH wiring
#   EC2HOSTS_NO_CONFIG  set to any value to skip config seeding

set -euo pipefail

REPO="SmowlJose/ec2hosts"
BINARY_NAME="ec2hosts"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/ec2hosts"
CONFIG_EXAMPLE_URL="https://raw.githubusercontent.com/${REPO}/main/config.yaml.example"

if [ -t 1 ]; then
  C_BOLD=$'\033[1m'
  C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'
  C_CYAN=$'\033[36m'
  C_RESET=$'\033[0m'
else
  C_BOLD=""
  C_RED=""
  C_GREEN=""
  C_YELLOW=""
  C_CYAN=""
  C_RESET=""
fi

log_info()  { printf '%s==>%s %s\n' "$C_CYAN"   "$C_RESET" "$*"; }
log_ok()    { printf '%sOK %s %s\n' "$C_GREEN"  "$C_RESET" "$*"; }
log_warn()  { printf '%sWARN%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
log_error() { printf '%sERR %s %s\n' "$C_RED"   "$C_RESET" "$*" >&2; }
die() { log_error "$*"; exit 1; }

# Globals populated as we progress — consumed by print_summary.
# shellcheck disable=SC2034
TMP_DIR=""
TMP_BINARY=""
INSTALL_DIR=""
INSTALL_PATH=""
USED_SUDO=0
PATH_RC_FILE=""
PATH_RC_ADDED=0
CONFIG_PATH=""
CONFIG_SEEDED=0

cleanup() {
  # Guarded: only remove TMP_DIR if it was set by download_binary and still
  # exists. Never operate on an empty or unset variable.
  if [ -n "${TMP_DIR:-}" ] && [ -d "$TMP_DIR" ]; then
    rm -rf -- "$TMP_DIR"
  fi
}
trap cleanup EXIT

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

detect_os() {
  case "$(uname -s)" in
    Linux)  echo linux ;;
    Darwin) echo darwin ;;
    *) die "Unsupported OS: $(uname -s). This installer targets Linux and macOS." ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo amd64 ;;
    arm64|aarch64) echo arm64 ;;
    *) die "Unsupported architecture: $(uname -m)." ;;
  esac
}

download_binary() {
  local os arch asset version url magic
  os=$(detect_os)
  arch=$(detect_arch)
  asset="${BINARY_NAME}-${os}-${arch}"
  version="${EC2HOSTS_VERSION:-latest}"
  if [ "$version" = "latest" ]; then
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
  else
    url="https://github.com/${REPO}/releases/download/${version}/${asset}"
  fi

  TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t ec2hosts)
  TMP_BINARY="${TMP_DIR}/${BINARY_NAME}"

  log_info "Downloading ${asset} (${version})"
  if ! curl -fsSL --retry 3 -o "$TMP_BINARY" "$url"; then
    die "Failed to download ${url}. Confirm the asset exists on the release page."
  fi
  if [ ! -s "$TMP_BINARY" ]; then
    die "Downloaded file is empty: ${url}"
  fi
  chmod +x "$TMP_BINARY"

  # Magic bytes: ELF or any Mach-O (fat or thin, BE or LE).
  magic=$(head -c 4 "$TMP_BINARY" | od -An -tx1 | tr -d ' \n')
  case "$magic" in
    7f454c46) ;;                                   # ELF
    feedface|feedfacf|cefaedfe|cffaedfe|cafebabe) ;; # Mach-O
    *) die "Downloaded asset is not a recognized Linux/macOS binary (magic: ${magic})." ;;
  esac

  log_ok "Binary downloaded and verified."
}

pick_install_dir() {
  local candidate="/usr/local/bin"

  if [ -n "${EC2HOSTS_BIN_DIR:-}" ]; then
    INSTALL_DIR="$EC2HOSTS_BIN_DIR"
    mkdir -p -- "$INSTALL_DIR" || die "Cannot create ${INSTALL_DIR}"
    INSTALL_PATH="${INSTALL_DIR}/${BINARY_NAME}"
    return
  fi

  if [ -w "$candidate" ]; then
    INSTALL_DIR="$candidate"
    INSTALL_PATH="${INSTALL_DIR}/${BINARY_NAME}"
    return
  fi

  if command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    INSTALL_DIR="$candidate"
    INSTALL_PATH="${INSTALL_DIR}/${BINARY_NAME}"
    USED_SUDO=1
    return
  fi

  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p -- "$INSTALL_DIR"
  INSTALL_PATH="${INSTALL_DIR}/${BINARY_NAME}"
}

install_binary() {
  log_info "Installing to ${INSTALL_PATH}"
  if [ "$USED_SUDO" -eq 1 ]; then
    sudo mv -f -- "$TMP_BINARY" "$INSTALL_PATH"
    sudo chmod 0755 "$INSTALL_PATH"
  else
    mv -f -- "$TMP_BINARY" "$INSTALL_PATH"
    chmod 0755 "$INSTALL_PATH"
  fi
  log_ok "Binary installed."
}

path_contains_dir() {
  case ":${PATH}:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

wire_path() {
  if [ -n "${EC2HOSTS_NO_PATH:-}" ]; then
    return
  fi
  if path_contains_dir "$INSTALL_DIR"; then
    return
  fi

  local shell_name rc marker
  shell_name=$(basename -- "${SHELL:-}")
  case "$shell_name" in
    bash) rc="$HOME/.bashrc" ;;
    zsh)  rc="$HOME/.zshrc"  ;;
    *)
      log_warn "Unrecognized shell '${shell_name}'. Add ${INSTALL_DIR} to PATH manually."
      return
      ;;
  esac

  marker="# added by ec2hosts installer"
  if [ -f "$rc" ] && grep -Fq "$marker" "$rc"; then
    PATH_RC_FILE="$rc"
    return
  fi

  {
    printf '\n%s\n' "$marker"
    # Deliberate: $PATH must stay literal so it expands when the rc file runs.
    # shellcheck disable=SC2016
    printf 'export PATH="%s:$PATH"\n' "$INSTALL_DIR"
  } >> "$rc"
  PATH_RC_FILE="$rc"
  PATH_RC_ADDED=1
}

seed_config() {
  if [ -n "${EC2HOSTS_NO_CONFIG:-}" ]; then
    return
  fi
  CONFIG_PATH="${CONFIG_DIR}/config.yaml"
  if [ -e "$CONFIG_PATH" ]; then
    return
  fi
  if ! mkdir -p -- "$CONFIG_DIR"; then
    log_warn "Could not create ${CONFIG_DIR} — skipping config seed."
    return
  fi
  if curl -fsSL --retry 2 -o "$CONFIG_PATH" "$CONFIG_EXAMPLE_URL"; then
    chmod 0644 "$CONFIG_PATH"
    CONFIG_SEEDED=1
  else
    log_warn "Could not fetch ${CONFIG_EXAMPLE_URL} — skipping config seed."
    # Remove the partial file if curl left one behind. Guarded: CONFIG_PATH
    # is non-empty because it was set above.
    if [ -n "$CONFIG_PATH" ] && [ -f "$CONFIG_PATH" ]; then
      rm -f -- "$CONFIG_PATH"
    fi
  fi
}

print_summary() {
  printf '\n%s%s installed.%s\n' "$C_BOLD$C_GREEN" "$BINARY_NAME" "$C_RESET"
  printf '  binary : %s\n' "$INSTALL_PATH"
  if [ -n "$CONFIG_PATH" ]; then
    if [ "$CONFIG_SEEDED" -eq 1 ]; then
      printf '  config : %s (seeded from example)\n' "$CONFIG_PATH"
    else
      printf '  config : %s (kept existing)\n' "$CONFIG_PATH"
    fi
  fi
  if [ "$PATH_RC_ADDED" -eq 1 ]; then
    printf '\n%sNext step:%s reload your shell so %s is on PATH:\n' \
      "$C_BOLD" "$C_RESET" "$INSTALL_DIR"
    printf '  source %s\n' "$PATH_RC_FILE"
  fi
  printf '\nRun %s%s --help%s to get started.\n' "$C_BOLD" "$BINARY_NAME" "$C_RESET"
}

main() {
  require_cmd curl
  require_cmd uname
  download_binary
  pick_install_dir
  install_binary
  wire_path
  seed_config
  print_summary
}

main "$@"
