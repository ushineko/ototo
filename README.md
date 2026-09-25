# ototo

Keeps your audio on the output you want. `ototo` (音跳び, *oto-tobi*, sound-hop,
shortened) lives in the tray, switches to the highest-priority connected
output, follows it with the matching microphone, connects Bluetooth headsets,
reroutes JamesDSP so effects never drop out, and draws a volume indicator when
you press the keys. Native Go over the PulseAudio protocol: no `pactl`, no
Python, no Qt.

It is the port of `audio-source-switcher` from
[ag-scripts](https://github.com/ushineko/ag-scripts), a PyQt6 program that has
done this job on a KDE Plasma desktop through thirteen releases, and it
replaces it. The behaviour is that program's; the spec is
[`specs/001`](specs/001-port-audio-source-switcher.md).

*Nothing from your machine lives in this repository: no device name, no
address, no settings file.*

**Version**: 0.1.0

> **Status: scaffold.** The window opens, reads the sound server and lists the
> outputs; `--status` prints the same in a terminal. Switching, priorities,
> Bluetooth, JamesDSP, the microphone link, the headset, the indicator and the
> tray arrive with the spec's remaining requirements, in that order.

## Table of Contents

- [What it does](#what-it-does)
- [Requirements](#requirements)
- [Installing](#installing)
- [Using it](#using-it)
- [Where things live](#where-things-live)
- [Configuration](#configuration)
- [Project layout](#project-layout)
- [Testing](#testing)
- [Changelog](#changelog)
- [License](#license)

## What it does

When complete, the same things the original does:

- **Priority auto-switching.** Order your outputs; the highest connected one
  plays. Unplug it, or power a headset off, and the next one takes over, with
  a notification.
- **Microphone association.** When the output changes, the input follows:
  matched automatically by device, pinned by you, or left alone.
- **Bluetooth.** Headsets in the same list as wired outputs, connected and
  disconnected from the window, remembered while they are away.
- **JamesDSP routing.** With JamesDSP running, its output is rewired to the
  chosen device instead of bypassing it, so equalisation survives every switch.
  If JamesDSP dies, the switch falls back to the hardware and says so.
- **Headset.** Battery and idle timeout for a SteelSeries Arctis Nova Pro,
  through `headsetcontrol`.
- **Volume indicator.** A small frameless panel on the screen the pointer is
  on, for a second and a half, when the volume changes. One switch takes the
  volume keys over for it and gives them back; the original left that step
  to the user.
- **Tray.** Close hides to the tray; the switching continues.

What is here today is the foundation: the window, the settings document, the
native sound-server client and the `--status` report.

## Requirements

- Linux with PipeWire and `pipewire-pulse` (or PulseAudio). `ototo` speaks the
  PulseAudio native protocol to whichever is listening.
- A desktop that draws OpenGL windows. KDE Plasma on Wayland is where it is
  used; the window rule and the global shortcuts are Plasma's.
- Optional at run time: `pw-link` (from PipeWire, for JamesDSP routing),
  `bluez` (for headsets), `headsetcontrol` (for the Arctis battery). Each is
  reported when absent, and nothing else stops working.
- To build: Go 1.26 or newer, a C toolchain, and the OpenGL and X11/Wayland
  development headers (`make build` names the packages if they are missing).

## Installing

From a checkout:

```
./install.sh
```

puts `ototo`, its launcher entry and its icon under `~/.local`. `./uninstall.sh`
removes exactly those three files and prints where your settings are.

On Arch, `make pkg-arch` builds a package from the checkout, and the Release
page carries one built by CI.

## Using it

```
ototo                 # the window, in the tray
ototo --status        # the sound server, the default output and the devices, as text
ototo --version
```

A build that is not the release says so: `0.1.0-1a2b3c4-dev` unless HEAD is on
the version's tag with a clean tree. Packages stamp their own version and are
unaffected.

`--section` and `--scheme` open the window on a section in a colour scheme
without saving either, for the screenshot harness. `--config` and `--server`
point it at another settings file or another sound server.

## Where things live

| Path | What |
|---|---|
| `$XDG_CONFIG_HOME/ototo/config.json` | your device order, mic links and switches |
| `$XDG_CONFIG_HOME/io.ushineko.ototo/settings.json` | the window's colour scheme, font and layout |

`$OTOTO_CONFIG` overrides the first. A `config.json` written by the PyQt6
program is read as it is: the keys are the same.

## Configuration

| Key | Default | Effect |
|---|---|---|
| `device_priority` | `[]` | The auto-switch order, highest first. `bt:<MAC>` for a Bluetooth device, the sink name for anything else. |
| `auto_switch` | `false` | Switch to the highest-priority connected output automatically. |
| `mic_links` | `{}` | Priority id to `auto`, `default` or a source name. Absent means `auto`. |
| `arctis_idle_minutes` | `0` | Headset idle disconnect; 0 is never. |
| `osd_enabled` | `true` | Show the volume indicator. |
| `switch_notifications` | `true` | Notify on an automatic switch. Failures are always reported. |
| `loopback_enabled` | `false` | Play the line-in source through the current output. |
| `move_streams` | `true` | Move playing audio to the new output on a switch. |

## Project layout

```
cmd/ototo/            the binary: flags, --status, and the window
internal/core/        every operation, headless: request in, result out
internal/audio/       the PulseAudio native protocol client (pure Go)
internal/config/      the settings document
internal/gui/         the window: fynedesygn's shell, this program's sections
internal/buildinfo/   version and commit, stamped by the Makefile
packaging/            desktop entry, icon, Arch PKGBUILD
tools/                screenshot harness
specs/                the design of record
```

## Testing

```
make test     # headless; the audio test skips where no sound server listens
make lint     # golangci-lint, pinned and checksum-verified on first run
```

## Changelog

### 0.1.0

- **The scaffold.** The window on fynedesygn's shell with Outputs, Appearance
  and About; the settings document, read compatibly with the PyQt6 program's
  `config.json`; a pure-Go client for the PulseAudio native protocol, which
  PipeWire serves, so nothing shells out to `pactl`; `--status` for a bug
  report. Installer, Arch package, CI and the screenshot harness follow the
  sibling repositories.

## License

MIT. See [LICENSE](LICENSE).
