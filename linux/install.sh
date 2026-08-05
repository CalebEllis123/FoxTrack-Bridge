#!/bin/sh
# Single-command per-user installer for FoxTrack Bridge on Linux. No sudo,
# no system-wide writes — everything goes under $HOME. Run from a checkout
# of this repository (e.g. `./linux/install.sh` after cloning); only the
# platform binary and checksums.txt are downloaded, the icon, .desktop entry,
# and systemd unit are read from this checkout.
set -eu

REPO="FoxesRCool1/FoxTrack-Bridge"
RELEASE_BASE_URL="https://github.com/${REPO}/releases/latest/download"

BIN_DIR="$HOME/.local/bin"
BIN_PATH="$BIN_DIR/foxtrack-bridge"
ICON_DIR="$HOME/.local/share/icons/hicolor/512x512/apps"
ICON_PATH="$ICON_DIR/foxtrack-bridge.png"
DESKTOP_DIR="$HOME/.local/share/applications"
DESKTOP_PATH="$DESKTOP_DIR/foxtrack-bridge.desktop"
SERVICE_DIR="$HOME/.config/systemd/user"
SERVICE_PATH="$SERVICE_DIR/foxtrack-bridge.service"
CONFIG_DIR="$HOME/.config/FoxTrack-Bridge"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
SRC_ICON="$REPO_ROOT/assets/logo.png"
SRC_DESKTOP="$REPO_ROOT/linux/foxtrack-bridge.desktop"
SRC_SERVICE="$REPO_ROOT/linux/foxtrack-bridge-user.service"

info() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
err()  { printf 'error: %s\n' "$*" >&2; }

have_cmd() { command -v "$1" >/dev/null 2>&1; }

usage() {
	cat <<EOF
Usage: linux/install.sh [--uninstall]

Installs FoxTrack Bridge for the current user only, under \$HOME/.local and
\$HOME/.config. No sudo is used or required.

  (no arguments)  Install the binary, icon, desktop entry, and systemd
                  user unit.
  --uninstall     Remove everything the installer put in place. Your
                  configuration and print history in
                  \$HOME/.config/FoxTrack-Bridge are never touched.
  -h, --help      Show this help.
EOF
}

fetch() {
	# fetch <url> <dest>
	if have_cmd curl; then
		curl -fsSL -o "$2" "$1"
	elif have_cmd wget; then
		wget -q -O "$2" "$1"
	else
		err "need curl or wget to download files"
		exit 1
	fi
}

refresh_caches() {
	if have_cmd gtk-update-icon-cache; then
		gtk-update-icon-cache -f -t "$HOME/.local/share/icons/hicolor" >/dev/null 2>&1 || true
	fi
	if have_cmd update-desktop-database; then
		update-desktop-database "$DESKTOP_DIR" >/dev/null 2>&1 || true
	fi
}

detect_asset() {
	arch=$(uname -m)
	case "$arch" in
	x86_64)
		echo "FoxTrack-Bridge-Linux"
		;;
	aarch64)
		echo "FoxTrack-Bridge-Linux-ARM64"
		;;
	armv7l)
		echo "FoxTrack-Bridge-Linux-ARM32"
		;;
	armv6l)
		err "ARMv6 is not supported — the ARM32 release build targets ARMv7 and later"
		exit 1
		;;
	*)
		err "unrecognized architecture '$arch' — no FoxTrack Bridge build is available for it"
		exit 1
		;;
	esac
}

do_install() {
	info "FoxTrack Bridge installer"
	info "This will install to:"
	info "  binary:  $BIN_PATH"
	info "  icon:    $ICON_PATH"
	info "  desktop: $DESKTOP_PATH"
	info "  service: $SERVICE_PATH (as a systemd --user unit, not enabled)"
	info ""

	if [ ! -f "$SRC_ICON" ] || [ ! -f "$SRC_DESKTOP" ] || [ ! -f "$SRC_SERVICE" ]; then
		err "this script must be run from a FoxTrack Bridge checkout (missing assets/logo.png, linux/foxtrack-bridge.desktop, or linux/foxtrack-bridge-user.service under $REPO_ROOT)"
		exit 1
	fi

	asset=$(detect_asset)
	info "Detected architecture $(uname -m) -> release asset $asset"

	tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/foxtrack-install.XXXXXX")
	trap 'rm -rf "$tmp_dir"' EXIT INT TERM HUP

	info "Downloading $asset and checksums.txt from the latest release..."
	fetch "$RELEASE_BASE_URL/$asset" "$tmp_dir/$asset"
	fetch "$RELEASE_BASE_URL/checksums.txt" "$tmp_dir/checksums.txt"

	info "Verifying checksum..."
	checksum_line="$tmp_dir/checksum_line.txt"
	if ! awk -v f="$asset" '$2 == f { print; found=1 } END { exit !found }' "$tmp_dir/checksums.txt" >"$checksum_line"; then
		err "no checksum entry for $asset in checksums.txt — aborting, nothing installed"
		exit 1
	fi
	if ! (cd "$tmp_dir" && sha256sum -c checksum_line.txt); then
		err "checksum verification failed for $asset — aborting, nothing installed"
		exit 1
	fi

	info "Installing binary..."
	mkdir -p "$BIN_DIR"
	cp "$tmp_dir/$asset" "$BIN_PATH.new"
	chmod 755 "$BIN_PATH.new"
	mv "$BIN_PATH.new" "$BIN_PATH"

	case ":$PATH:" in
	*":$BIN_DIR:"*) ;;
	*)
		warn "$BIN_DIR is not on your PATH. Add it in your shell profile, e.g.:"
		warn "  export PATH=\"\$HOME/.local/bin:\$PATH\""
		;;
	esac

	info "Installing icon and desktop entry..."
	mkdir -p "$ICON_DIR"
	cp "$SRC_ICON" "$ICON_PATH"
	mkdir -p "$DESKTOP_DIR"
	cp "$SRC_DESKTOP" "$DESKTOP_PATH"
	refresh_caches

	info "Installing systemd user unit..."
	mkdir -p "$SERVICE_DIR"
	cp "$SRC_SERVICE" "$SERVICE_PATH"
	info "  installed as foxtrack-bridge.service (renamed from foxtrack-bridge-user.service"
	info "  to avoid clashing with the system template unit foxtrack-bridge@.service)"

	info ""
	info "Install complete."
	info "Dashboard: http://localhost:8080"
	info ""
	info "The service was installed but not enabled. To run it on login, run:"
	info "  systemctl --user enable --now foxtrack-bridge"
	info "  loginctl enable-linger $(id -un)"
}

do_uninstall() {
	info "Uninstalling FoxTrack Bridge..."

	if have_cmd systemctl; then
		systemctl --user stop foxtrack-bridge >/dev/null 2>&1 || true
		systemctl --user disable foxtrack-bridge >/dev/null 2>&1 || true
	fi

	rm -f "$BIN_PATH"
	rm -f "$ICON_PATH"
	rm -f "$DESKTOP_PATH"
	rm -f "$SERVICE_PATH"
	refresh_caches

	info "Removed the binary, icon, desktop entry, and systemd unit."
	info "Your configuration and print history were left untouched at:"
	info "  $CONFIG_DIR"
}

case "${1:-}" in
"")
	do_install
	;;
--uninstall)
	do_uninstall
	;;
-h | --help)
	usage
	;;
*)
	usage >&2
	exit 1
	;;
esac
