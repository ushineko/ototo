#!/usr/bin/env bash
# Capture ototo for the README, on KDE/Wayland.
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
# Nothing in these images comes from the person running the script. The window
# is pointed at a demo sound server (--server demo:<file>): a file this script
# writes with invented devices and documentation-range addresses, under a
# temporary HOME and a runtime directory of its own, so the tray instance is
# untouched and no device of anyone's appears in a committed screenshot.
#
# Requires kdotool (Wayland's xdotool), spectacle, and python3 with Pillow.
set -euo pipefail

CLASS="io.ushineko.ototo"
DEMO=""
BIN="${OTOTO_GUI:-$(command -v ototo || echo ./ototo)}"
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

  assets/screenshot-outputs.png     the device list, the playing device and its volume
  assets/screenshot-microphone.png  which input follows each output
  assets/screenshot-settings.png    the switches and the desktop steps

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
[ -x "$BIN" ] || { echo "ototo not found (set OTOTO_GUI, or run make build)" >&2; exit 1; }

# DEMO is a fixed path rather than a mktemp one, and deliberately: it can appear
# in a screenshot, and "/tmp/ototo-demo/..." reads as an example while
# "/tmp/tmp.4Xk9aP/..." reads as a mistake.
DEMO_ROOT="${TMPDIR:-/tmp}/ototo-demo"

# demo builds the world the captures are taken in: a throwaway HOME, a demo
# sound server of invented devices, and a settings file with an order.
demo() {
    DEMO="$DEMO_ROOT"
    # A demo window left by an interrupted run would be a second window with
    # this title; it is stopped by its command line, read from /proc so that
    # this script's own command line cannot match.
    for p in $(pgrep -x ototo 2>/dev/null); do
        if tr '\0' ' ' < "/proc/$p/cmdline" 2>/dev/null | grep -q -- "--server demo:$DEMO/"; then
            kill "$p" 2>/dev/null || true
        fi
    done
    rm -rf "$DEMO"
    mkdir -p "$DEMO/home/.config" "$DEMO/home/.local/share" "$DEMO/home/.cache" "$DEMO/run"
    cat > "$DEMO/devices.json" <<'JSON'
{
  "default_sink": "alsa_output.usb-Example_Audio_DAC-00.analog-stereo",
  "default_source": "alsa_input.usb-Example_Audio_DAC-00.analog-stereo-linein",
  "sinks": [
    {"name": "alsa_output.usb-Example_Audio_DAC-00.analog-stereo", "description": "Example DAC",
     "properties": {"device.vendor.name": "Example Audio", "device.product.name": "DAC", "device.bus_path": "usb-1"},
     "ports": [{"name": "analog-output-headphones", "description": "Headphones", "available": "yes"}],
     "active_port": "analog-output-headphones", "volume": 40},
    {"name": "alsa_output.usb-Example_Wireless_Headset-00.analog-stereo", "description": "Example Wireless Headset",
     "properties": {"device.vendor.name": "Example", "device.product.name": "Arctis Nova Headset", "device.serial": "HS-1"},
     "volume": 55},
    {"name": "alsa_output.pci-0000_01_00.1.hdmi-stereo", "description": "Monitor",
     "properties": {"device.description": "HDMI Audio"},
     "ports": [{"name": "hdmi-output-0", "description": "HDMI / DisplayPort", "available": "yes"}],
     "active_port": "hdmi-output-0", "volume": 100},
    {"name": "alsa_output.pci-0000_00_1f.3.analog-stereo", "description": "Built-in Audio",
     "properties": {"device.description": "Built-in Audio"},
     "ports": [{"name": "analog-output-lineout", "description": "Line Out", "available": "no"}],
     "active_port": "analog-output-lineout", "volume": 85}
  ],
  "sources": [
    {"name": "alsa_input.usb-Example_Audio_DAC-00.analog-stereo-linein", "description": "Example DAC Line In",
     "properties": {"device.description": "Example DAC Line In", "device.bus_path": "usb-1"},
     "ports": [{"name": "analog-input-linein", "description": "Line In", "available": "yes"}],
     "active_port": "analog-input-linein"},
    {"name": "alsa_input.usb-Example_Wireless_Headset-00.mono", "description": "Example Wireless Headset Microphone",
     "properties": {"device.description": "Example Wireless Headset Microphone", "device.serial": "HS-1"}},
    {"name": "alsa_input.usb-Example_Desk_Mic-00.mono", "description": "Example Desk Mic",
     "properties": {"device.description": "Example Desk Mic"}}
  ],
  "bluetooth": [
    {"mac": "AA:BB:CC:DD:EE:FF", "name": "Example Earbuds"},
    {"mac": "00:11:22:33:44:55", "name": "Living room speaker"}
  ],
  "headset": {"detected": true, "battery": "87%"}
}
JSON
    cat > "$DEMO/config.json" <<'JSON'
{
  "device_priority": [
    "bt:AA:BB:CC:DD:EE:FF",
    "alsa_output.usb-Example_Wireless_Headset-00.analog-stereo",
    "alsa_output.usb-Example_Audio_DAC-00.analog-stereo",
    "alsa_output.pci-0000_01_00.1.hdmi-stereo"
  ],
  "auto_switch": true,
  "mic_links": {"alsa_output.pci-0000_01_00.1.hdmi-stereo": "alsa_input.usb-Example_Desk_Mic-00.mono"},
  "osd_enabled": true,
  "switch_notifications": true,
  "switch_in_osd": true,
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

    # HOME and the XDG directories are redirected so the window cannot reach a
    # remembered setting. XDG_RUNTIME_DIR is deliberately NOT: that is where the
    # Wayland display socket and the sound server's socket live.
    HOME="$DEMO/home" XDG_CONFIG_HOME="$DEMO/home/.config" \
        XDG_DATA_HOME="$DEMO/home/.local/share" XDG_CACHE_HOME="$DEMO/home/.cache" \
        OTOTO_RUNTIME_DIR="$DEMO/run" \
        "$BIN" --config "$DEMO/config.json" --server "demo:$DEMO/devices.json" --section "$sect" \
        ${scheme:+--scheme "$scheme"} >/dev/null 2>&1 &
    local pid=$!
    # shellcheck disable=SC2064  # pid is captured deliberately, at trap-set time
    trap "kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true" RETURN

    # The window is the one this binary just opened, and not any other ototo
    # on this desktop: the tray instance has a window of the same class, and a
    # capture of it would be a capture of the developer's own devices. kdotool
    # 0.3 ignores --pid, so the match is the class and the exact title, which
    # carries this binary's version; and if that still finds more than one
    # window, nothing is captured, because a guess here is the wrong picture.
    local title; title="ototo $("$BIN" --version | awk '{print $2}')"
    local wid="" waited=0
    while [ "$waited" -lt 40 ]; do
        local matches=""
        for candidate in $(timeout 10 kdotool search --class "$CLASS" 2>/dev/null || true); do
            if [ "$(timeout 10 kdotool getwindowname "$candidate" 2>/dev/null || true)" = "$title" ]; then
                matches="$matches $candidate"
            fi
        done
        set -- $matches
        if [ $# -gt 1 ]; then
            echo "more than one window is titled '$title'; close the others and try again:$matches" >&2
            return 1
        fi
        wid="${1:-}"
        [ -n "$wid" ] && break
        sleep 0.25
        waited=$((waited + 1))
    done
    [ -n "$wid" ] || { echo "the window never appeared (no window of class $CLASS titled '$title')" >&2; return 1; }

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
    # The geometry the check below compares against is read now, before the
    # grab: read afterwards, it once described a window that had grown while
    # the image was being written, and a correct capture was refused.
    local geo_check; geo_check=$(timeout 10 kdotool getwindowgeometry "$wid" 2>/dev/null || true)

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
    for s in Outputs Microphone Settings; do
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
