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

**Version**: 0.1.2

[Screenshots](https://github.com/ushineko/ototo/blob/main/docs/screenshots.md)
of the three sections, over invented devices.

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
- **A sound on a switch**, if you want one: a short built-in chime, or a WAV
  file of your own, played on the device that just became the output.
- **The README, in the window.** About holds this document, with the routing
  diagram; the screenshots are a page of their own, linked above.
- **The desktop, on request.** Autostart, the indicator's window rule and the
  volume keys are switches in Settings. Nothing is written to the desktop
  until you turn one on, and each one says how it is undone.

## How it routes audio

Applications play into the default output. Without JamesDSP, that is the
device you chose. With JamesDSP running, the default stays on its sink and
`ototo` moves the filter's output to the device you chose, so the effects
apply to everything and survive every switch.

```mermaid
flowchart LR
    subgraph apps [Applications]
        A1[Browser]
        A2[Player]
        A3[Game]
    end
    subgraph server [Sound server]
        D[jamesdsp_sink<br/>the default output]
        F[JamesDSP filter]
        H1[Headphones]
        H2[Speakers]
        H3[Headset]
    end
    A1 --> D
    A2 --> D
    A3 --> D
    D --> F
    F -. "linked by ototo" .-> H1
    F -. "or" .-> H2
    F -. "or" .-> H3
    O((ototo)) -- "sets the default,<br/>moves the links" --> D
    O -- "connects, follows<br/>with the microphone" --> H3
```

The choice comes from your order: every five seconds the highest device in
it that can play becomes the output, and a Bluetooth device is connected
first. Volume keys act on the device that is playing, behind the filter.

## Requirements

- **A PulseAudio server, mandatory.** PipeWire with `pipewire-pulse`, or
  PulseAudio itself. `ototo` speaks the PulseAudio native protocol to
  whichever is listening, and does nothing without one.
- A desktop that draws OpenGL windows.
- Optional at run time: `pw-link` (from PipeWire, for JamesDSP routing),
  `bluez` (for headsets), `headsetcontrol` (for the Arctis battery). Each is
  reported when absent, and nothing else stops working.
- To build: Go 1.26 or newer, a C toolchain, and the OpenGL and X11/Wayland
  development headers (`make build` names the packages if they are missing).

**Tested on** CachyOS with KDE Plasma 6 on Wayland, PipeWire 1.6 serving the
PulseAudio protocol, and JamesDSP. That is the one machine it was written on
and runs on. The window rule, the global shortcuts and the autostart entry are
Plasma's; on another desktop the indicator keeps its titlebar and the volume
keys stay with whatever holds them. Other desktops and distributions may work
and are untested. If you try it and it does not work for you, please file an
issue with the output of `ototo --status`.

### JamesDSP, or no effects at all

JamesDSP is not required. Without it, a switch sets the chosen device as the
default output and moves the playing streams there, and that is all.

With JamesDSP running, `ototo` recognises its sink and its output ports by
name, `jamesdsp_sink` and the `jdsp_` ports in the PipeWire graph, keeps the
default output on that sink, and rewires its output to the chosen device, so
the effects apply whatever plays. If the rewiring fails, the switch falls back
to the device itself and says so.

Another effects sink, such as EasyEffects, is not recognised. It appears in
the list as an ordinary device, and a switch to a hardware device sets that
device as the default, which bypasses it. If you use one and want it routed
the way JamesDSP is, please file an issue with its sink and port names from
`pw-link -o` and `ototo --status`.

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

Keys of your own, on KDE Plasma:

```
ototo --desktop bind Meta+A --connect "AirPods Pro"   # a key that switches to a device
ototo --desktop bind "Meta+Num++" --vol-up             # a key that steps the volume
ototo --desktop unbind Meta+A
```

The key is spelled as System Settings spells it. A custom shortcut that held
the key is released and named; another program's own shortcut is left alone
and the binding is refused until you release it in System Settings.

`--section` and `--scheme` open the window on a section in a colour scheme
without saving either, for the screenshot harness. `--config` and `--server`
point it at another settings file or another sound server; `--server
demo:<file>` is an in-memory server over the devices the file describes,
which is what the screenshots are taken over.

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
make test         # headless; the audio test skips where no sound server listens
make lint         # golangci-lint, pinned and checksum-verified on first run
make screenshots  # refresh docs/screenshots.md's images (KDE/Wayland; kdotool and spectacle)
```

The screenshots are taken over a demo sound server of invented devices, in a
throwaway home and runtime directory, so nothing of the machine that takes
them appears in the repository. They live on a page of their own,
[docs/screenshots.md](docs/screenshots.md), so the README embedded in the
window does not carry pictures of the window.

## Changelog

### Unreleased

- The switch sound waits for the route: it plays once the server reports
  the new default and, through JamesDSP, once the graph reports the filter
  linked to the new device, and it plays into the device's own sink, so it
  is no longer lost in a sink that is still being rewired. For a device
  that just appeared, such as headphones that just connected, the sound is
  scheduled for later instead, "After a device connects, wait seconds" in
  Settings, 15 by default: their Bluetooth transport is not active until a
  second after the sink exists, a stream played meanwhile goes by unheard,
  and headphones such as the Sony WH-1000XM6 render nothing for about ten
  seconds more, with nothing on the bus to mark the moment they do.
- The sound plays to its end. Playback asked the server to drain before the
  clip had been handed over, so it returned after about a second whatever
  the length and closed the stream on the rest; nothing was lost while the
  chime was the whole clip, and everything was once silence went in front.
- A switch holds the indicator for 3 s, twice a key press's 1.5 s.

### 0.1.2

- **A sound on a switch.** "Play a sound when the output switches" in
  Settings: a short built-in chime, or a WAV file of your own, played on the
  device that just became the output, over the sound server and without a
  subprocess.
- **About holds the README**, with a diagram of how the audio is routed,
  rendered at development time and embedded. The screenshots are a page of
  their own, `docs/screenshots.md`, linked from the README by its GitHub
  address, so the copy in the window links there too.

### 0.1.1

- **The launcher entry runs ototo.** It said `Exec=ototo-gui`, the binary from
  before the two became one, so the menu entry and a taskbar pin ran nothing.
  The 0.1.0 tarball and Arch package carried it.
- **Keys of your own.** `ototo --desktop bind Meta+A --connect "AirPods Pro"`
  registers a command shortcut that runs ototo and releases a custom shortcut
  that held the key; `--desktop unbind` removes it. The original's "Copy
  Hotkey Command" left the shortcut for you to make.
- **Connect brings a Bluetooth device up without switching.** It did what
  Switch to does; now it connects and waits for the sink, and the order or
  Switch to decides what plays.
- **Screenshots** over a demo sound server, `--server demo:<file>`, so the
  README shows invented devices. The README says what is mandatory, where
  this was tested, and that JamesDSP is not required.

### 0.1.0

The port, complete enough to replace the original on the desktop it was
written for. The behaviour is the original's, function for function; the
differences are the ones below.

- **One binary, in the tray.** The window on fynedesygn's shell: Outputs,
  Microphone, Settings, Appearance and About. `--vol-up`, `--vol-down` and
  `--connect` are answered by the running window; `--status` prints the
  machine for a bug report.
- **No `pactl`, no Python, no Qt.** The sound server is reached over the
  PulseAudio native protocol, which PipeWire serves; BlueZ, notifications and
  KWin over D-Bus. `pw-link` and `headsetcontrol` are the two subprocesses,
  both optional.
- **Switching as the original did it.** Priority auto-switching every five
  seconds, JamesDSP rewired so effects survive a switch with the circuit
  breaker and the floating repair, the microphone following by device, a
  Bluetooth device connected and waited for before the switch.
- **The indicator is one panel.** The icon, the level, the meter and the
  device name, placed on the screen the pointer is on, gone after a moment.
  A switch can show there instead of a notification. The text size and the
  font are settings, and the font chooser previews the indicator itself.
- **The desktop, on request.** Autostart, the indicator's window rule and
  the volume keys are switches in Settings and `--desktop install`. The keys
  are taken through kglobalaccel with what held them recorded, and given back
  when turned off; the original left that step to the user.
- **The line-in loopback**, through the systemd unit when it exists, else a
  `pw-loopback` child, with the source selectable when there is more than
  one.

## License

MIT. See [LICENSE](LICENSE).
