#!/usr/bin/env bash
# Capture ototo-gui for the README, on KDE/Wayland.
#
# Adapted from angou's tools/screenshot.sh (MIT, same author) by way of
# nmsbonker's; the window-finding, focus-checking and aspect-ratio machinery is
# angou's and the reasons below are its comments, kept because they are still
# the reasons.
#
# The window takes --section, --scheme and --config, so the script starts a fresh
# window on the section it wants, grabs it, and kills it. Refreshing the whole
# set is one command with nothing to click.
#
# Two things make this less trivial than "take a screenshot":
#
#   1. The active window is almost never the one we want. The window is
#      therefore raised first, and found by window *class* -- searching by name
#      also matches a browser sitting on the project's GitHub page.
#   2. When a dialog is open the dialog *is* the active window, so an active-
#      window grab returns the dialog alone. For those shots pass --with-dialog:
#      it captures the whole desktop and crops to the window's geometry.
#
# The window is pointed at a settings file this script writes and throws away,
# under a temporary HOME, so no path of anyone's appears in a committed
# screenshot. What it cannot fake is the sound server: the outputs in the image
# are the outputs of the machine the script runs on. Check that the device
# names in a capture are ones you are happy to publish.
#
# Requires kdotool (Wayland's xdotool), spectacle, and python3 with Pillow.
set -euo pipefail

CLASS="io.ushineko.ototo"
DEMO=""
BIN="${OTOTO_GUI:-$(command -v ototo-gui || echo ./ototo-gui)}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
    cat <<'USAGE'
usage: tools/screenshot.sh [--with-dialog] [--scheme NAME] --section NAME <output.png>
       tools/screenshot.sh --all

  --section NAME  which section to open on (Outputs, Appearance, About)
  --scheme NAME   colour scheme for this run; not saved over the user's choice
  --with-dialog   a dialog is open: capture the desktop and crop, rather than grabbing
                  the active window (which would be the dialog on its own)
  --all           refresh the README set into assets/, then print the alt-text reminder

  assets/screenshot-outputs.png    the outputs the sound server can see

The alt text in README.md describes what is actually in each image. It is the only
description a screen-reader user gets, and a stale one is worse than none -- check it
still matches before committing a new capture.
USAGE
}

with_dialog=0
section=""
scheme=""
out=""
all=0
while [ $# -gt 0 ]; do
    case "$1" in
        --with-dialog) with_dialog=1 ;;
        --section) shift; section="${1:-}" ;;
        --scheme) shift; scheme="${1:-}" ;;
        --all) all=1 ;;
        -h|--help) usage; exit 0 ;;
        -*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
        *) out="$1" ;;
    esac
    shift
done

for tool in kdotool spectacle python3; do
    command -v "$tool" >/dev/null || { echo "$tool is not installed" >&2; exit 1; }
done
python3 -c "import PIL" 2>/dev/null || { echo "python3 Pillow is not installed" >&2; exit 1; }
[ -x "$BIN" ] || { echo "ototo-gui not found (set OTOTO_GUI, or run make build-gui)" >&2; exit 1; }

# DEMO is a fixed path rather than a mktemp one, and deliberately: it can appear
# in a screenshot, and "/tmp/ototo-demo/..." reads as an example while
# "/tmp/tmp.4Xk9aP/..." reads as a mistake.
DEMO_ROOT="${TMPDIR:-/tmp}/ototo-demo"

# demo builds the world the captures are taken in: a throwaway HOME and a
# settings file with an example priority order.
demo() {
    DEMO="$DEMO_ROOT"
    rm -rf "$DEMO"
    mkdir -p "$DEMO/home/.config" "$DEMO/home/.local/share" "$DEMO/home/.cache"
    cat > "$DEMO/config.json" <<'JSON'
{
  "device_priority": [],
  "auto_switch": true,
  "osd_enabled": true,
  "switch_notifications": true,
  "move_streams": true
}
JSON
}

demo_cleanup() {
    [ -n "$DEMO" ] || return 0
    rm -rf "$DEMO"
}

# capture starts a window on the requested section, grabs it, and stops it again.
capture() {
    local sect="$1" dest="$2"

    # Wait for any previous instance to be gone before starting the next.
    local gone=0
    while [ "$gone" -lt 40 ]; do
        [ -z "$(timeout 10 kdotool search --class "$CLASS" 2>/dev/null || true)" ] && break
        sleep 0.25
        gone=$((gone + 1))
    done

    # HOME and the XDG directories are redirected so the window cannot reach a
    # remembered setting. XDG_RUNTIME_DIR is deliberately NOT: that is where the
    # Wayland display socket and the sound server's socket live.
    HOME="$DEMO/home" XDG_CONFIG_HOME="$DEMO/home/.config" \
        XDG_DATA_HOME="$DEMO/home/.local/share" XDG_CACHE_HOME="$DEMO/home/.cache" \
        "$BIN" --config "$DEMO/config.json" --section "$sect" \
        ${scheme:+--scheme "$scheme"} >/dev/null 2>&1 &
    local pid=$!
    # shellcheck disable=SC2064  # pid is captured deliberately, at trap-set time
    trap "kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true" RETURN

    local wid="" waited=0
    while [ "$waited" -lt 40 ]; do
        wid=$(timeout 10 kdotool search --class "$CLASS" 2>/dev/null | head -1 || true)
        [ -n "$wid" ] && break
        sleep 0.25
        waited=$((waited + 1))
    done
    [ -n "$wid" ] || { echo "the window never appeared (no window of class $CLASS)" >&2; return 1; }

    # Activating is asynchronous, and `spectacle -a` grabs whatever is active at
    # the moment it fires. So: activate, then confirm we have focus before grabbing.
    local active="" tries=0
    while [ "$tries" -lt 12 ]; do
        timeout 10 kdotool windowactivate "$wid" >/dev/null 2>&1 || true
        sleep 0.5
        active=$(timeout 10 kdotool getactivewindow 2>/dev/null || true)
        [ "$active" = "$wid" ] && break
        tries=$((tries + 1))
    done
    [ "$active" = "$wid" ] || { echo "could not focus the window (active=$active want=$wid)" >&2; return 1; }
    sleep 1.5                   # let it repaint after the raise, and let its loads land

    rm -f "$dest"
    if [ "$with_dialog" -eq 0 ]; then
        # -S drops the compositor's drop shadow, which otherwise pads the image unevenly
        timeout 30 spectacle -a -b -n -S -o "$dest" >/dev/null 2>&1 || true
        sleep 1.5
    else
        local tmp; tmp=$(mktemp --suffix=.png)
        timeout 30 spectacle -f -b -n -o "$tmp" >/dev/null 2>&1 || true
        sleep 1.5
        local geo; geo=$(timeout 10 kdotool getwindowgeometry "$wid")
        python3 "${REPO_DIR}/tools/crop.py" "$tmp" "$dest" \
            "$(printf '%s' "$geo" | awk '/Position/{print $2}')" \
            "$(printf '%s' "$geo" | awk '/Geometry/{print $2}')"
        rm -f "$tmp"
    fi

    [ -s "$dest" ] || { echo "capture produced nothing" >&2; return 1; }

    # A last sanity check on the geometry: an image wildly wider or taller than
    # the window we asked for is not a screenshot of it.
    local geo_check; geo_check=$(timeout 10 kdotool getwindowgeometry "$wid" 2>/dev/null || true)
    python3 - "$dest" "$(printf '%s' "$geo_check" | awk '/Geometry/{print $2}')" <<'PY'
import sys
from PIL import Image

path, dim = sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else ""
im = Image.open(path)
if dim and "x" in dim:
    w, h = (float(v) for v in dim.split("x"))
    want, got = w / h, im.width / im.height
    if abs(want - got) / want > 0.05:
        sys.exit("captured %dx%d, but the window is %gx%g -- wrong window grabbed"
                 % (im.width, im.height, w, h))
PY
    python3 - "$dest" <<'PY'
import os, sys
from PIL import Image
p = sys.argv[1]
im = Image.open(p)
print("  %s  %dx%d  %.0fK" % (os.path.basename(p), im.width, im.height,
                              os.path.getsize(p) / 1024))
PY
}

if [ "$all" -eq 1 ]; then
    # Force a scheme unless one was asked for, so two refreshes on two machines
    # produce the same colours.
    : "${scheme:=Breeze Dark}"
    mkdir -p "${REPO_DIR}/assets"
    demo
    trap demo_cleanup EXIT
    for s in Outputs; do
        low=$(printf '%s' "$s" | tr '[:upper:]' '[:lower:]')
        capture "$s" "${REPO_DIR}/assets/screenshot-${low}.png"
    done
    echo
    echo "Now check the alt text in README.md still describes what is in each image,"
    echo "and that no device name in any of them is one you would rather not publish."
    exit 0
fi

[ -n "$out" ] || { usage >&2; exit 2; }
[ -n "$section" ] || { echo "--section is required (or use --all)" >&2; exit 2; }
demo
trap demo_cleanup EXIT
capture "$section" "$out"
