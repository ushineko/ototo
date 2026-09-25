#!/usr/bin/env bash
#
# Removes exactly what install.sh puts in place: the program, a launcher entry
# and an icon. Everything ototo has written for you stays, and this prints
# where it is. Idempotent.

set -euo pipefail

BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/.local/share/applications"
ICON_DIR="${HOME}/.local/share/icons/hicolor/scalable/apps"

APP_ID="io.ushineko.ototo"

DRY_RUN=0
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        -h|--help)
            cat <<'USAGE'
Usage: uninstall.sh [--dry-run]

  --dry-run   List what would be removed, change nothing

Removes only the three files install.sh placed. Your settings and the window's
appearance preferences are left alone, and their locations are printed so you
can remove them by hand if you want to. Anything the program installed into the
desktop on request (autostart entry, KWin rule, volume-key bindings) is undone
by the program's own command, named below.
USAGE
            exit 0
            ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

echo "Removing ototo ..."

removed=0
for f in "${BIN_DIR}/ototo" \
         "${APP_DIR}/${APP_ID}.desktop" \
         "${ICON_DIR}/ototo.svg"; do
    if [ -e "$f" ]; then
        removed=$((removed + 1))
        if [ "$DRY_RUN" -eq 1 ]; then
            echo "  would remove $f"
        else
            echo "  removing $f"
            rm -f "$f"
        fi
    fi
done
if [ "$removed" -eq 0 ]; then
    echo "  nothing to remove; install.sh has not run, or has already been undone"
fi

if [ "$DRY_RUN" -eq 0 ]; then
    command -v update-desktop-database >/dev/null 2>&1 && \
        update-desktop-database "${APP_DIR}" >/dev/null 2>&1 || true
    command -v gtk-update-icon-cache >/dev/null 2>&1 && \
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
fi

CONFIG_HOME="${XDG_CONFIG_HOME:-${HOME}/.config}"

echo
echo "Done."
echo
echo "Nothing you made was removed. It is in:"
echo
kept=(
    "${CONFIG_HOME}/ototo/config.json|your device order, mic links and switches"
    "${CONFIG_HOME}/${APP_ID}/settings.json|the window's colour scheme, font and layout"
)
width=0
for entry in "${kept[@]}"; do
    path="${entry%%|*}"
    if [ "${#path}" -gt "$width" ]; then
        width="${#path}"
    fi
done
for entry in "${kept[@]}"; do
    printf '    %-*s  %s\n' "$width" "${entry%%|*}" "${entry#*|}"
done
echo
echo "If you installed the autostart entry, the KWin rule for the volume indicator"
echo "or the volume-key bindings from inside ototo, undo them first with:"
echo "    ototo --desktop uninstall"
