#!/usr/bin/env bash
#
# Installs ototo, its launcher entry and its icon. Idempotent: safe to re-run.
#
# Nothing here touches your sound server, your Bluetooth pairings, your KWin
# rules or any settings ototo has written. It builds from this checkout and
# copies three files into ~/.local.

set -euo pipefail

BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/.local/share/applications"
ICON_DIR="${HOME}/.local/share/icons/hicolor/scalable/apps"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

APP_ID="io.ushineko.ototo"

DRY_RUN=0
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        -h|--help)
            cat <<'USAGE'
Usage: install.sh [--dry-run]

  --dry-run   Show what would be installed, change nothing

Installs:
  ~/.local/bin/ototo                                      the program
  ~/.local/share/applications/io.ushineko.ototo.desktop   the launcher entry
  ~/.local/share/icons/hicolor/scalable/apps/ototo.svg    its icon

Building needs Go, CGO and a C toolchain with the OpenGL and X11 or Wayland
development headers; the message on failure names the packages.

Autostart, the KWin rule for the volume indicator and the volume-key bindings
are not installed here. They are explicit steps the program offers, because
each one changes the desktop, and uninstall.sh names how each is undone.
USAGE
            exit 0
            ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

run() {
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: $*"
    else
        "$@"
    fi
}

echo "Installing ototo from ${REPO_DIR} ..."

if ! command -v go >/dev/null 2>&1; then
    echo "Error: go is not installed. ototo needs Go 1.26 or newer to build." >&2
    echo "       Arch: pacman -S go   Debian/Ubuntu: apt install golang-go" >&2
    exit 1
fi

echo "Building ototo ..."
if [ "$DRY_RUN" -eq 1 ]; then
    echo "  would run: make -C $REPO_DIR build"
elif ! make -C "$REPO_DIR" build; then
    echo
    echo "ototo did not build. It is a Fyne window and needs CGO, OpenGL and the X11 or" >&2
    echo "Wayland development headers:" >&2
    echo "    Arch:          base-devel libgl libxi libxcursor libxrandr libxinerama" >&2
    echo "    Debian/Ubuntu: build-essential libgl1-mesa-dev xorg-dev" >&2
    echo "    Fedora:        gcc mesa-libGL-devel libXi-devel libXcursor-devel libXrandr-devel libXinerama-devel" >&2
    exit 1
fi

echo "Installing to ${BIN_DIR} ..."
run install -Dm755 "${REPO_DIR}/ototo" "${BIN_DIR}/ototo"
run install -Dm644 "${REPO_DIR}/packaging/${APP_ID}.desktop" "${APP_DIR}/${APP_ID}.desktop"
run install -Dm644 "${REPO_DIR}/packaging/ototo.svg" "${ICON_DIR}/ototo.svg"

if command -v update-desktop-database >/dev/null 2>&1; then
    run update-desktop-database "${APP_DIR}"
fi
# The icon cache is per theme directory and only some desktops need it poked;
# a failure here costs nothing but a stale icon until the next login.
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: gtk-update-icon-cache -f -t ${HOME}/.local/share/icons/hicolor"
    else
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
    fi
fi

echo
echo "Done."
case ":${PATH}:" in
    *":${BIN_DIR}:"*) ;;
    *) echo "Note: ${BIN_DIR} is not on your PATH." ;;
esac
echo
echo "Next:"
echo
echo "  ototo --status        # the sound server, the default output and what it can see"
echo "  ototo                 # open the window, or \"ototo\" in your application launcher"
