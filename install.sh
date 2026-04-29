#!/usr/bin/env bash
#
# DeployKit installer / upgrader.
#
# Idempotent: re-running this script upgrades an existing install in place.
# DeployKit itself can re-invoke this script via the deploykitd-upgrade
# systemd unit (file-watch trigger) when the user clicks "Update" in the UI.
#
# Usage:
#   curl -fsSL https://get.deploykit.dev | sudo bash
#   curl -fsSL https://get.deploykit.dev | sudo VERSION=v0.2.0 bash
#
# Environment variables:
#   VERSION         Release tag to install (default: latest).
#   INSTALL_DIR     Where to drop the binary (default: /usr/local/bin).
#   LIB_DIR         Where to stage helper files (default: /usr/local/lib/deploykit).
#   DATA_DIR        Where SQLite + state live (default: /var/lib/deploykit).
#   ADDR            HTTP listen address (default: :8080).
#   GITHUB_REPO     Source repo (default: deploykitdev/deploykit).
#   SKIP_DOCKER     Set to 1 to skip Docker installation.
#   SKIP_SERVICE    Set to 1 to skip systemd unit registration.
#   SKIP_VERIFY     Set to 1 to skip cosign signature verification (NOT recommended).
#   UPGRADE_STATUS  Set by the deploykitd-upgrade unit; writes JSON status.

set -euo pipefail

GITHUB_REPO="${GITHUB_REPO:-deploykitdev/deploykit}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
LIB_DIR="${LIB_DIR:-/usr/local/lib/deploykit}"
DATA_DIR="${DATA_DIR:-/var/lib/deploykit}"
ADDR="${ADDR:-:8080}"
SKIP_DOCKER="${SKIP_DOCKER:-0}"
SKIP_SERVICE="${SKIP_SERVICE:-0}"
SKIP_VERIFY="${SKIP_VERIFY:-0}"
UPGRADE_STATUS="${UPGRADE_STATUS:-}"

# Cosign keyless verification expects the artifacts to be signed by a
# specific workflow + tag. These pin both: the workflow file path and that
# the run was triggered by a tag push (refs/tags/v*).
COSIGN_VERSION="v2.4.1"
COSIGN_IDENTITY_REGEX="^https://github.com/${GITHUB_REPO}/\.github/workflows/release\.yml@refs/tags/v.+$"
COSIGN_OIDC_ISSUER="https://token.actions.githubusercontent.com"

SERVICE_USER="deploykit"
BINARY_NAME="deploykitd"
SYSTEMD_UNIT="/etc/systemd/system/deploykitd.service"
UPGRADE_SERVICE_UNIT="/etc/systemd/system/deploykitd-upgrade.service"
UPGRADE_PATH_UNIT="/etc/systemd/system/deploykitd-upgrade.path"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m  %s\n' "$*" >&2; }
err()  { printf '\033[1;31mxx\033[0m  %s\n' "$*" >&2; write_status "failed" "$*" || true; exit 1; }

# write_status writes a JSON status file when invoked under the upgrade unit
# (UPGRADE_STATUS points at the file). No-op for interactive installs.
write_status() {
    [ -n "$UPGRADE_STATUS" ] || return 0
    local state="$1"
    local error="${2:-}"
    local now
    now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    local started_at="${UPGRADE_STARTED_AT:-$now}"
    local finished_at=""
    case "$state" in
        done|failed) finished_at="$now" ;;
    esac
    local payload
    payload="$(printf '{"state":"%s","target_version":"%s","started_at":"%s","finished_at":"%s","error":%s}' \
        "$state" "$VERSION" "$started_at" "$finished_at" \
        "$(printf '%s' "$error" | jq -Rs . 2>/dev/null || printf '"%s"' "${error//\"/\\\"}")")"
    printf '%s\n' "$payload" > "${UPGRADE_STATUS}.tmp"
    mv "${UPGRADE_STATUS}.tmp" "$UPGRADE_STATUS"
}

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        err "this installer must be run as root (try: curl -fsSL ... | sudo bash)"
    fi
}

detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"

    case "$os" in
        linux) ;;
        *) err "unsupported OS: $os (only linux is supported)" ;;
    esac

    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) err "unsupported architecture: $arch" ;;
    esac

    PLATFORM="${os}_${arch}"
}

resolve_version() {
    if [ "$VERSION" = "latest" ]; then
        log "resolving latest release from github.com/${GITHUB_REPO}"
        # Buffer the response first: piping curl into `grep -m1` under
        # `set -o pipefail` makes grep close the pipe early, curl gets EPIPE,
        # and the script aborts with `curl: (23)`.
        local response
        response="$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest")" \
            || err "could not reach github.com/${GITHUB_REPO} releases API"
        VERSION="$(printf '%s' "$response" | sed -nE 's/.*"tag_name": *"([^"]+)".*/\1/p' | head -n1)"
        [ -n "$VERSION" ] || err "could not determine latest version"
    fi
}

# detect_existing reports whether deploykitd is already installed and, if
# so, captures the running version into CURRENT_VERSION.
detect_existing() {
    CURRENT_VERSION=""
    MODE="install"
    if [ -x "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        MODE="upgrade"
        CURRENT_VERSION="$(${INSTALL_DIR}/${BINARY_NAME} -version 2>/dev/null | awk '{print $2}' || echo "unknown")"
    fi
}

# refuse_downgrade aborts the upgrade if the requested version sorts before
# the currently installed version. "dev" or "unknown" current versions are
# treated as upgradeable to anything.
refuse_downgrade() {
    [ "$MODE" = "upgrade" ] || return 0
    case "$CURRENT_VERSION" in
        ""|dev|unknown) return 0 ;;
    esac
    # Strip leading "v" for comparison; sort -V handles semver correctly.
    local cur new top
    cur="${CURRENT_VERSION#v}"
    new="${VERSION#v}"
    if [ "$cur" = "$new" ]; then
        log "already on ${VERSION}; nothing to do"
        write_status "done"
        exit 0
    fi
    top="$(printf '%s\n%s\n' "$cur" "$new" | sort -V | tail -n1)"
    if [ "$top" != "$new" ]; then
        err "refusing to downgrade ${CURRENT_VERSION} -> ${VERSION}"
    fi
}

install_docker() {
    if command -v docker >/dev/null 2>&1; then
        return
    fi
    if [ "$SKIP_DOCKER" = "1" ]; then
        warn "docker not found and SKIP_DOCKER=1; deploykitd will fail to start until docker is available"
        return
    fi
    log "installing docker via get.docker.com"
    curl -fsSL https://get.docker.com | sh
    systemctl enable --now docker
}

ensure_user() {
    if id "$SERVICE_USER" >/dev/null 2>&1; then
        # Make sure it's still in the docker group (idempotent).
        usermod -aG docker "$SERVICE_USER" 2>/dev/null || true
        return
    fi
    log "creating system user '${SERVICE_USER}'"
    useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
    usermod -aG docker "$SERVICE_USER"
}

ensure_dirs() {
    mkdir -p "$DATA_DIR" "$LIB_DIR"
    chown -R "${SERVICE_USER}:${SERVICE_USER}" "$DATA_DIR"
    chmod 750 "$DATA_DIR"
}

# backup_db copies the SQLite file alongside itself before swapping the binary.
# Cheap insurance — the file is small and migrations may be irreversible.
backup_db() {
    [ "$MODE" = "upgrade" ] || return 0
    local db="${DATA_DIR}/deploykit.db"
    [ -f "$db" ] || return 0
    local stamp
    stamp="$(date -u +%Y%m%d-%H%M%S)"
    local backup="${db}.bak-${CURRENT_VERSION}-${stamp}"
    log "snapshotting database to ${backup}"
    cp -p "$db" "$backup"
    chown "${SERVICE_USER}:${SERVICE_USER}" "$backup"
}

# ensure_cosign installs the cosign binary into LIB_DIR if it isn't already
# on PATH. cosign is a single static binary from sigstore.
ensure_cosign() {
    if command -v cosign >/dev/null 2>&1; then
        COSIGN_BIN="$(command -v cosign)"
        return
    fi
    COSIGN_BIN="${LIB_DIR}/cosign"
    if [ -x "$COSIGN_BIN" ]; then
        return
    fi
    local arch_suffix
    case "$PLATFORM" in
        linux_amd64) arch_suffix="linux-amd64" ;;
        linux_arm64) arch_suffix="linux-arm64" ;;
        *) err "no cosign binary for ${PLATFORM}" ;;
    esac
    log "downloading cosign ${COSIGN_VERSION}"
    if ! curl -fsSL -o "$COSIGN_BIN" \
        "https://github.com/sigstore/cosign/releases/download/${COSIGN_VERSION}/cosign-${arch_suffix}"; then
        err "failed to download cosign"
    fi
    chmod +x "$COSIGN_BIN"
}

# verify_signature downloads checksums.txt + checksums.txt.bundle for the
# release, verifies the bundle against the GitHub Actions OIDC identity that
# signed it, then verifies the asset's sha256 matches its checksums entry.
# Bypassed when SKIP_VERIFY=1 (loud warning).
verify_signature() {
    local tmp="$1" asset="$2" base="$3"

    if [ "$SKIP_VERIFY" = "1" ]; then
        warn "SKIP_VERIFY=1: skipping signature check for ${asset}"
        return
    fi

    log "verifying ${asset} signature"
    if ! curl -fsSL -o "${tmp}/checksums.txt" "${base}/checksums.txt"; then
        err "failed to download checksums.txt"
    fi
    if ! curl -fsSL -o "${tmp}/checksums.txt.bundle" "${base}/checksums.txt.bundle"; then
        err "failed to download checksums.txt.bundle (release may not be signed; re-run with SKIP_VERIFY=1 to bypass)"
    fi

    ensure_cosign

    if ! "$COSIGN_BIN" verify-blob \
        --bundle "${tmp}/checksums.txt.bundle" \
        --certificate-identity-regexp "$COSIGN_IDENTITY_REGEX" \
        --certificate-oidc-issuer "$COSIGN_OIDC_ISSUER" \
        "${tmp}/checksums.txt" >/dev/null 2>&1; then
        err "cosign signature on checksums.txt did NOT verify; aborting"
    fi

    # checksums.txt is "<sha256>  <filename>" per line. Pull the expected
    # digest for our asset and confront it with the local file.
    local expected actual
    expected="$(awk -v f="$asset" '$2 == f {print $1}' "${tmp}/checksums.txt")"
    if [ -z "$expected" ]; then
        err "no checksum entry found for ${asset} in checksums.txt"
    fi
    actual="$(sha256sum "${tmp}/${asset}" | awk '{print $1}')"
    if [ "$expected" != "$actual" ]; then
        err "sha256 mismatch for ${asset}: expected ${expected}, got ${actual}"
    fi

    log "signature ok (cosign + sha256)"
}

download_binary() {
    local tmp asset url base
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT

    asset="deploykitd_${VERSION#v}_${PLATFORM}.tar.gz"
    base="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"
    url="${base}/${asset}"

    log "downloading ${asset}"
    if ! curl -fsSL -o "${tmp}/${asset}" "$url"; then
        err "failed to download ${url}"
    fi

    verify_signature "$tmp" "$asset" "$base"

    tar -xzf "${tmp}/${asset}" -C "$tmp"

    # Sanity check before swapping anything: make sure the new binary at
    # least runs and reports a version.
    chmod +x "${tmp}/${BINARY_NAME}"
    if ! "${tmp}/${BINARY_NAME}" -version >/dev/null 2>&1; then
        err "downloaded binary failed -version check; aborting"
    fi

    if [ "$MODE" = "upgrade" ] && [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        log "preserving previous binary at ${INSTALL_DIR}/${BINARY_NAME}.previous"
        cp -p "${INSTALL_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}.previous"
    fi

    log "installing ${BINARY_NAME} -> ${INSTALL_DIR}/${BINARY_NAME}"
    install -m 0755 "${tmp}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
}

# stage_self copies the running script into LIB_DIR so the upgrade systemd
# unit can re-invoke a stable, on-disk copy (the user's `curl | sh` invocation
# is gone after the script returns).
stage_self() {
    local src="${BASH_SOURCE[0]:-}"
    if [ -z "$src" ] || [ ! -f "$src" ]; then
        # Running from a pipe — re-download the script. We can't read /dev/stdin
        # twice, so pull it from the same release tag for self-consistency.
        log "staging install.sh from release ${VERSION}"
        curl -fsSL -o "${LIB_DIR}/install.sh" \
            "https://raw.githubusercontent.com/${GITHUB_REPO}/${VERSION}/install.sh"
    else
        cp -p "$src" "${LIB_DIR}/install.sh"
    fi
    chmod 0755 "${LIB_DIR}/install.sh"
}

write_units() {
    if [ "$SKIP_SERVICE" = "1" ]; then
        log "skipping systemd units (SKIP_SERVICE=1)"
        return
    fi

    log "writing systemd unit ${SYSTEMD_UNIT}"
    cat > "$SYSTEMD_UNIT" <<EOF
[Unit]
Description=DeployKit self-hosted PaaS
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
WorkingDirectory=${DATA_DIR}
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -addr ${ADDR} -db ${DATA_DIR}/deploykit.db
Environment=DEPLOYKIT_DATA_DIR=${DATA_DIR}
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

    log "writing systemd unit ${UPGRADE_SERVICE_UNIT}"
    cat > "$UPGRADE_SERVICE_UNIT" <<EOF
[Unit]
Description=DeployKit upgrade runner (oneshot)
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target

[Service]
Type=oneshot
# Read VERSION from the trigger file written by the deploykitd backend.
ExecStart=/bin/bash -c 'VERSION="\$(cat ${DATA_DIR}/upgrade.requested)" UPGRADE_STATUS=${DATA_DIR}/upgrade.status UPGRADE_STARTED_AT="\$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" ${LIB_DIR}/install.sh >> ${DATA_DIR}/upgrade.log 2>&1'
ExecStartPost=/bin/rm -f ${DATA_DIR}/upgrade.requested
EOF

    log "writing systemd unit ${UPGRADE_PATH_UNIT}"
    cat > "$UPGRADE_PATH_UNIT" <<EOF
[Unit]
Description=Watch for DeployKit upgrade requests

[Path]
PathExists=${DATA_DIR}/upgrade.requested
Unit=deploykitd-upgrade.service

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload

    if [ "$MODE" = "upgrade" ]; then
        log "restarting deploykitd.service"
        systemctl restart deploykitd.service
    else
        systemctl enable --now deploykitd.service
    fi
    systemctl enable --now deploykitd-upgrade.path
}

print_summary() {
    if [ -n "$UPGRADE_STATUS" ]; then
        # Running inside the upgrade unit — keep stdout terse, the JSON file
        # is the real status surface.
        log "upgrade to ${VERSION} complete"
        write_status "done"
        return
    fi

    local host port
    port="${ADDR##*:}"
    host="$(hostname -I 2>/dev/null | awk '{print $1}')"
    [ -n "$host" ] || host="<your-server-ip>"

    if [ "$MODE" = "upgrade" ]; then
        cat <<EOF

  DeployKit upgraded ${CURRENT_VERSION} -> ${VERSION}.

    systemctl status deploykitd
    journalctl -u deploykitd -f

EOF
    else
        cat <<EOF

  DeployKit ${VERSION} installed.

    binary:      ${INSTALL_DIR}/${BINARY_NAME}
    data dir:    ${DATA_DIR}
    service:     deploykitd.service
    upgrades:    deploykitd-upgrade.path (file-watch)
    listen:      ${ADDR}

  next steps:
    systemctl status deploykitd
    journalctl -u deploykitd -f

  open http://${host}:${port} in your browser to create the first admin user.

EOF
    fi
}

main() {
    require_root
    detect_platform
    resolve_version
    detect_existing
    refuse_downgrade

    if [ "$MODE" = "upgrade" ]; then
        log "upgrading DeployKit ${CURRENT_VERSION} -> ${VERSION}"
        write_status "running"
    else
        log "installing DeployKit ${VERSION}"
    fi

    install_docker
    ensure_user
    ensure_dirs
    stage_self
    backup_db
    download_binary
    write_units
    print_summary
}

main "$@"
