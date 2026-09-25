# Spec 001: Port audio-source-switcher to Go on fynedesygn

**Issue**: #1

## Status: INCOMPLETE

## Executive Summary

*(Populated before the PR opens.)*

## Context

`ag-scripts/audio-source-switcher` is a PyQt6 tray application that has run a
KDE Plasma/Wayland desktop's audio for thirteen releases: priority
auto-switching between outputs, a microphone that follows the output,
Bluetooth headsets in the same list, JamesDSP rewired so effects survive a
switch, an Arctis headset's battery and idle timeout, a volume indicator on
the volume keys, and a tray icon. It is 2,400 lines of Python over `pactl`,
`pw-link`, `headsetcontrol`, `notify-send` and python-dbus.

`ototo` replaces it with one Go binary on fynedesygn, the design system the
sibling programs share. The reasons are the ones that hold for the siblings:
one static-ish artifact instead of an interpreter and a Qt stack, a window
that follows the same rules as the other tools, and behaviour in a headless
core that can be tested without a desktop.

The behavioural reference is the Python program. Where this spec says
"as the original", the original's code is the definition; the domain map in
the survey that preceded this spec (magic strings, algorithms, config keys)
is reproduced in the requirements below so that the port does not depend on
reading Python.

## Decisions

These are the design choices that are not obvious from the original. Each is
open to argument; each is recorded so that the argument is with the spec and
not with the code.

- **D1. One binary, no separate CLI.** The original is a tray application
  with three flags. The tray application is the program: the auto-switch
  loop, the indicator and the notifications live in the resident process.
  `ototo` therefore has no cobra command tree and no CLI/GUI parity test.
  What the original's flags did survives as flags on the same binary:
  `--connect NAME`, `--vol-up`, `--vol-down` are forwarded to the running
  instance and exit, and `--status` prints the machine's state for a bug
  report. Core stays headless (D2) so this costs nothing in testability.
  *Considered:* a `cmd/ototo` CLI beside `cmd/ototo-gui`, as nmsbonker and
  angou have; rejected because nothing here is worth running without the
  desktop, and two binaries plus a parity guard is overhead for a tray app.
- **D2. Core is headless.** Every operation is a request-in, result-out
  function in `internal/core`. The window renders; the flag paths render.
  Tests drive core with a throwaway `HOME`, a throwaway config and a sound
  server that is either the live one (skipped when absent) or none.
- **D3. Native PulseAudio protocol, no `pactl`.** `internal/audio` speaks the
  PulseAudio native protocol through `github.com/jfreymuth/pulse/proto` (pure
  Go). PipeWire serves that protocol on the same socket, and it is what
  `pactl` speaks. Everything the original used `pactl` for is a request:
  server info, sinks, sources, sink inputs, default sink and source, volume,
  mute, move-sink-input, and the change subscription. Verified in the
  scaffold against PipeWire 1.6 (`ototo --status`).
- **D4. `pw-link` stays a subprocess.** The JamesDSP rewiring is PipeWire's
  own graph, which the PulseAudio protocol cannot reach. A native PipeWire
  protocol client in Go does not exist at a quality worth depending on, and
  the graph work is three commands run at a switch. It is optional at run
  time: absent `pw-link` means "JamesDSP routing unavailable", reported, and
  hardware switching continues.
- **D5. D-Bus for BlueZ, notifications and KWin**, through
  `github.com/godbus/dbus/v5`, which is already in the module graph as a
  Fyne dependency. No `bluetoothctl`, no `notify-send`, no `qdbus`.
  Notifications go to `org.freedesktop.Notifications` with the same hints
  the original passed (`sound-name`, and `value`/`synchronous:volume` for
  the CLI volume fallback).
- **D6. `headsetcontrol` stays a subprocess**, optional. The alternative is
  a HID driver for one headset model. Absent tool means the Headset group
  reads "not detected", as the original.
- **D7. The settings document is the original's `config.json`**, same path
  (`$XDG_CONFIG_HOME/ototo/config.json` rather than
  `audio-source-switcher/`; a copy from the old path is read as it is), same
  keys, same defaults, backfilled on load. The window's appearance lives in
  fynedesygn's own store under the app ID, as in every sibling.
- **D8. The volume indicator is a glance window.** See R8, which holds the
  analysis of the fit and the gaps.
- **D9. The desktop is written only on request, and then completely.**
  Autostart, the KWin rule and the volume-key binding are steps the program
  offers (`--desktop install` / `--desktop uninstall`, and switches in
  Settings), each reversible. The original's `install.sh` did the first two
  silently and left the third half done: it freed the kmix keys and told the
  user to create the custom shortcuts by hand. R11 finishes that: the program
  binds the keys itself and puts them back itself, so the indicator is a
  switch a user can turn on and off, not a procedure.
- **D10. Single instance over a Unix socket in `$XDG_RUNTIME_DIR`**, with an
  `flock` on a lock file beside it (terrariabonker's pattern). A second
  `ototo` with no flags asks the first to show its window and exits, as the
  original's `SHOW` message did. The hotkey flags use the same socket.

## Requirements

Requirements carry the original's semantics. A requirement marked *(as the
original)* has its definition in the survey's domain map and in the Python
source, and the port's behaviour must match it.

### R1. Program shape (scaffold: done)

- R1.1 One binary `ototo`, a Fyne window on fynedesygn's shell, app ID
  `io.ushineko.ototo`, desktop entry of the same basename.
- R1.2 `internal/core` holds every operation; the window and the flag paths
  render only.
- R1.3 Flags: `--section`, `--scheme`, `--config`, `--server`, `--status`,
  `--version` (scaffold); `--connect`, `--vol-up`, `--vol-down`, `--desktop`
  (later requirements).
- R1.4 Installer, uninstaller, Arch package, CI with release-from-changelog,
  screenshot harness: as nmsbonker.

### R2. Settings (scaffold: done)

- R2.1 Keys and defaults exactly: `device_priority` `[]`, `auto_switch`
  `false`, `arctis_idle_minutes` `0`, `mic_links` `{}`, `osd_enabled` `true`,
  `switch_notifications` `true`, `loopback_enabled` `false`, `move_streams`
  `true` (the last was unpersisted in the original and always started on;
  persisting it is the one addition).
- R2.2 A file lacking a key gets the default; a missing file is the default
  document; a damaged file is an error naming the path.
- R2.3 Writes are atomic (temp file and rename).

### R3. Sound server client (done)

- R3.1 Connect over the native protocol; `--server` and `$PULSE_SERVER`
  override the session socket.
- R3.2 Sinks and sources with properties, ports, active port, mute and the
  loudest channel as a percentage; sources exclude monitors.
- R3.3 Set default sink and source; set volume absolute (clamped 0..150) and
  relative (step 5); set mute; move every sink input to a sink.
- R3.4 Subscribe to sink change events and deliver them on a channel; the
  subscription survives a server restart by reconnecting with backoff.
- R3.5 Connected-ness: a device whose active port reports "not available"
  is disconnected; a device with no ports is connected *(as the original's
  `_append_port_info`)*.

### R4. Device model *(as the original)* (done)

- R4.1 Priority id: `bt:<MAC upper, colons>` when the sink name carries a
  MAC (`([0-9A-F]{2}[:_]){5}[0-9A-F]{2}`, case-insensitive), else the sink
  name. This is the key for `device_priority`, `mic_links` and `--connect`.
- R4.2 Display name resolution in order: BlueZ alias for a bluez sink (by
  MAC, from the D-Bus device cache), else `bluez.alias`/`device.alias`; then
  `device.vendor.name` + `device.product.name`/`device.model`; then
  `device.description`; strip a literal `(null)`; then the sink name. Append
  ` - <port description>` unless it is exactly `Analog Output`; append
  ` [Disconnected]` when unavailable; append ` [<battery>]` or
  ` [Disconnected]` for a sink whose name contains `Arctis Nova` or
  `SteelSeries`.
- R4.3 Offline devices: every id in `device_priority` with no live sink is a
  row, named from the BlueZ cache for `bt:` ids, marked disconnected. A
  `jamesdsp_sink` never appears in the list.
- R4.4 The list is one list: online sinks, offline remembered devices, and
  paired Bluetooth audio devices (A2DP sink, audio source, headset,
  handsfree, AADP UUIDs, or an `audio-` icon), in priority order then
  appearance order.

### R5. Switching and auto-switching *(as the original)* (done)

- R5.1 `Switch(target)`: if JamesDSP outputs exist in the graph and the
  breaker is closed, set default to `jamesdsp_sink`, move streams there if
  `move_streams`, relink JamesDSP to the target (R6); on relink failure trip
  the breaker and fall back to setting the target as default. Otherwise set
  the target as default and move streams.
- R5.2 After the sink change, the microphone follows (R7); then one
  notification "Audio Switched" with output and input, only when the
  physical sink changed since the last switch, and only if
  `switch_notifications`.
- R5.3 A tick every 5 s while `auto_switch` is on: refresh Bluetooth, sinks,
  volume, loopback; then decide. Target = first id in `device_priority`,
  skipping `jamesdsp_sink`, that is present and connected. Switch when the
  default differs from the target; or when JamesDSP outputs exist and the
  default is the target but not `jamesdsp_sink` and the breaker is closed
  (enforcement); or when the default is `jamesdsp_sink` and its target is
  wrong or floating with outputs present. When no priority target exists and
  the current default is disconnected, switch to the first connected sink.
- R5.4 The breaker is closed by a successful relink, by any user-initiated
  switch, and by the enforcement branch seeing outputs return.
- R5.5 `--connect NAME` matches exact priority id, then case-insensitive
  substring of display name, sink name or id, then the Bluetooth cache
  (stripping a `bt:` prefix). An offline Bluetooth match connects first and
  polls for the sink every 500 ms for up to 10 s. Exit 0 on switch, 1 on
  failure, each failure notified.

### R6. JamesDSP routing *(as the original)* (done)

- R6.1 Outputs: lines of `pw-link -o` containing `jdsp_`, `JamesDsp` and
  `:output_`. Target inputs: lines of `pw-link -i` containing the sink name
  and `:playback_`. Current target: from `pw-link -l`, the `  |->` lines
  under each output, split at `:playback_`.
- R6.2 Relink: unlink every existing playback link of every output, then
  link sorted outputs to sorted inputs pairwise. Empty outputs or empty
  inputs is a failure.
- R6.3 The volume the window shows and the keys change is the hardware
  sink's when the default is `jamesdsp_sink`.
- R6.4 Absent `pw-link` is reported once and disables R5.1's JamesDSP path.

### R7. Microphone association *(as the original)* (done)

- R7.1 `mic_links[id]` is `auto` (absent), `default` (leave alone) or a
  source name.
- R7.2 Auto matches the first source sharing, in order, `api.bluez5.address`,
  `device.serial`, `device.bus_path`, `device.name`, `alsa.card` with the
  sink; empty sink values are skipped; no match means no change.

### R8. Volume indicator (OSD)

The original's OSD is a 380 x 132 frameless, translucent, always-on-top,
non-activating window centred on the screen the pointer is on, showing a
volume icon, a level bar (amber above 100 %, grey when muted) and the
percentage, for 1.5 s after the last change, coalescing rapid presses. It
appears on the volume keys (which run the program with `--vol-up`/`--vol-down`)
and on any sink volume change the subscription reports, debounced 80 ms and
deduplicated against the last shown state. Its position and its lack of a
frame came from one KWin rule per screen, matched by window title
(`ass-volume-osd@<x>_<y>`), with `position`, `above`, `noborder`,
`skiptaskbar`, `skipswitcher` and `skippager` all forced.

**Fit with fynedesygn.** This is a glance window (`docs/glance.md`): frameless,
fixed-size, always on top, read without touching, opaque with contrast
separating it from the desktop, the KWin rule supplying what the toolkit
cannot. `glance.NewWindow` with `Options{OnTop: true}` and a `glance.Meter`
cover the window and the bar. What the shape as written does not cover, and
what this spec proposes as library work under "Gaps found":

- **Transient.** A glance window is resident; the OSD shows for 1.5 s and
  hides. Show and hide on a timer is program logic, but "show without taking
  focus" is not something Fyne offers, and the original relied on Qt's
  `WA_ShowWithoutActivating`. The KWin rule key `acceptfocus=false` (forced)
  is the compositor's answer; `glance/kwin.Rule` has no field for it.
- **Placement per screen.** The original positioned by rule, one rule per
  screen, matched by title. `glance/kwin.Rule` matches by app ID only and
  writes no `position`. `docs/glance.md` says a `position` rule on Wayland
  moves the window to the screen's origin and recommends the Scripting D-Bus
  API instead; the original shipped four releases on a `position` rule with
  coordinates and one rule per screen. One of these observations is out of
  date, and R8.4 settles it before the design commits.
- **Skip lists.** `skiptaskbar`, `skipswitcher`, `skippager`: not in the
  library's rule, and an indicator that appears in Alt-Tab is a defect.

- R8.1 The indicator is a `glance` window: one per screen, or one moved per
  event, decided by R8.4. It draws icon, bar and percentage from a snapshot
  (`Show(percent int, muted bool)`), and nothing else.
- R8.2 It appears for 1,500 ms after the last change; a change while visible
  restarts the timer without re-showing. It never takes focus.
- R8.3 It appears on the hotkey flags and on subscription events, debounced
  80 ms and deduplicated on (percent, muted). It records the state even when
  `osd_enabled` is false, and then does not show.
- R8.4 **Placement experiment, before R8.1 is built.** On this Plasma 6
  Wayland session, test (a) a KWin `position` rule with coordinates and a
  title match, as the original did, and (b) the Scripting D-Bus API moving a
  window by `resourceClass`. Record which works in this spec and update
  `docs/glance.md` in fynedesygn if (a) does, since that page says it cannot.
- R8.5 The KWin rule is installed by `--desktop install` and from Settings,
  never at first run, and removed by `--desktop uninstall`. It is written
  through `glance/kwin` once that package carries the keys R8 needs.
- R8.6 Without KWin, the indicator still works: frameless where GLFW
  manages it, opaque, wherever the compositor puts it.

### R9. Bluetooth, headset, loopback, tray *(as the original)*

- R9.1 BlueZ over the system bus: enumerate `Device1` objects, filter audio
  by UUID or icon, `Connect`/`Disconnect` off the UI thread, connect on
  selecting an offline device, error text from the D-Bus error.
- R9.2 Arctis: `headsetcontrol -b -c` battery, negative or failure is "not
  detected"; `headsetcontrol -i N` idle timeout applied when the setting
  changes, 0 disables, else 1..90.
- R9.3 Line-in loopback: `systemctl --user` `audio-loopback.service` when
  installed, else a `pw-loopback -C <source> -P <target>` child, target the
  first sink containing `jamesdsp` else `@DEFAULT_SINK@`; restored at start
  from `loopback_enabled`.
- R9.4 Tray: icon, menu Show / About / Quit; close hides to the tray and the
  loop continues; left click toggles. Without a tray (no `desktop.App`),
  close quits.

### R10. Window sections

- R10.1 Outputs: the unified list (R4.4) with drag-to-reorder writing
  `device_priority`, the playing device marked, connect/disconnect and
  switch actions, and the volume with a mute toggle.
- R10.2 Microphone: per-device link editor (R7).
- R10.3 Headset: battery and idle timeout, disabled with "not detected".
- R10.4 Settings: the switches (`auto_switch`, `osd_enabled`,
  `switch_notifications`, `move_streams`, `loopback_enabled`) and the
  desktop steps (D9, R11): autostart, the indicator's KWin rule and the
  volume keys, each a switch that says what it changes and how it is undone.
- R10.5 Appearance and About: the library's.

### R11. Volume keys, bound and unbound by the program

The original's `install.sh --bind-volume-keys` set the kmix `increase_volume`
and `decrease_volume` shortcuts to `none` in `kglobalshortcutsrc`, backed the
old values up, and printed instructions for creating two custom shortcuts in
System Settings that run the program with `--vol-up` and `--vol-down`. That is
the half that users skip, and the half that makes the indicator optional in
practice rather than by design.

- R11.1 "Use the volume keys for ototo's indicator" is one switch, in
  Settings and as `--desktop install --volume-keys`. On: record kmix's
  current bindings for `increase_volume` and `decrease_volume` in ototo's
  config directory, clear them, and register `Volume Up` and `Volume Down` to
  run `ototo --vol-up` and `ototo --vol-down`. Off: unregister ototo's two
  shortcuts and restore kmix's from the record. Both are idempotent and both
  report what they did.
- R11.2 Registration goes through `org.kde.kglobalaccel` over the session
  bus, never by editing `kglobalshortcutsrc`: kglobalaccel keeps the table
  in memory and writes the file itself, so an edit is overwritten (hotaru
  learned this; its `internal/desktop/claims.go` is the tested D-Bus
  pattern for reading claims and releasing them). On Plasma 6 a command
  shortcut is a desktop entry under `$XDG_DATA_HOME/kglobalaccel/` carrying
  `X-KDE-GlobalAccel-CommandShortcut=true` and an `Exec`, with its sequence
  set through `setShortcut` on that entry's component. **Verify this on the
  development machine before building it**, and record here which of the
  two shapes (`kglobalaccel/` entry, or `applications/` entry) this Plasma
  version honours.
- R11.3 A key that another component holds is reported, not taken: "Volume
  Up is held by <component>. Release it in System Settings, or turn the
  indicator off." A claim that is a leftover (hotaru's `Claim`) is offered
  for release with a confirmation.
- R11.4 `uninstall.sh` names the switch as the way to put the keys back, and
  `--desktop uninstall` turns it off along with the autostart entry and the
  KWin rule.
- R11.5 With the switch off, the indicator still appears on volume changes
  that the subscription reports (R8.3), because Plasma's own keys change the
  sink volume and the change is what the indicator listens for. The switch
  changes who shows the indicator, not whether volume works.

## Acceptance Criteria

Scaffold (this PR):

- [x] `make build` produces `ototo`; `--version` and `--status` answer; `--status` lists the live server's outputs with the default marked (verified against PipeWire 1.6).
- [x] `make test` passes headless; the audio test skips with no server; the config tests cover defaults, backfill, atomic save, env override and a parse error naming the file.
- [x] `make lint` passes with the pinned golangci-lint.
- [x] The window opens on fynedesygn's shell with Outputs, Appearance and About; a machine with no sound server draws the reason, not a blank section.
- [x] Installer, uninstaller, PKGBUILD, CI workflow and screenshot harness present, adapted from nmsbonker, for one binary.

Port (later PRs, one per requirement group):

- [x] R3.3, R3.4: write side and subscription, tested against the live server where present (PR: feat/audio-write-and-watch; the reconnect path is exercised only by the no-server test, since restarting the developer's sound server from a test is not acceptable).
- [x] R4: device model, with table-driven tests over synthetic sink property sets (no real MACs). (PR: feat/device-model. The Bluetooth cache and the headset battery are inputs the model takes; they are supplied by R9, so until then a Bluetooth device is named by its address and the Arctis reads as off.)
- [x] R5, R6, R7: switching, auto-switching, JamesDSP and microphone, with the algorithm tested over a fake server snapshot and a fake graph. (PR: feat/switching. `--connect`, `--vol-up` and `--vol-down` act on the server directly until D10 forwards them to the running instance. One deviation from the original, on purpose: the fallback that picks any connected sink never picks the JamesDSP sink itself, which the original's sink order could.)
- [ ] R8: the indicator, after the R8.4 experiment is recorded here.
- [ ] R9: Bluetooth, headset, loopback, tray.
- [ ] R10: the sections, with headless tests naming the defect each prevents.
- [ ] D9, D10, R11: desktop steps, the volume keys bound and restored by the program, and single instance.
- [ ] The original is retired from ag-scripts' README with a pointer here.

## Risks & Assumptions

- **Assumption: the PulseAudio protocol over PipeWire is complete for R3.**
  Verified for the read side. `MoveSinkInput`, `SetSinkVolume`, `SetSinkMute`,
  `SetDefaultSink/Source` and `Subscribe` exist in the proto package and are
  the same commands `pactl` sends; untested until R3.3.
- **Risk: KWin rule placement on Wayland** (R8.4). Two sources disagree.
  Settled by experiment before code.
- **Risk: a splash window may take focus when shown.** If Fyne/GLFW gives the
  OSD focus, a game loses input for a frame on every volume key. The KWin
  `acceptfocus` rule is the mitigation; without KWin, this is a limitation
  to document.
- **Risk: the Plasma 6 command-shortcut mechanism** (R11.2) is verified by
  experiment before code, as R8.4 is. If kglobalaccel refuses to register a
  command shortcut for a non-KDE component, the fallback is the original's
  half step (free the keys, print the instructions), stated as such.
- **Risk: fynedesygn changes.** R8 needs `glance/kwin.Rule` to grow fields.
  Those land in fynedesygn first, by its own spec; ototo pins the release.
- **Assumption: `pw-link` output format is stable** across PipeWire 1.x. The
  parser is the original's, which has run since PipeWire 1.0.
- **Rollback**: the original stays in ag-scripts and installable until the
  last checkbox above; a user who prefers it runs its `install.sh`. This
  repository adds nothing to the desktop without D9's explicit step, and each
  step has its inverse.
- **Query budget, migrations, shared code**: none.

## Gaps found (fynedesygn)

Candidate library changes, to be specced in fynedesygn:

1. `glance/kwin.Rule`: `TitleMatch`, `Position` (if R8.4 finds it works),
   `SkipTaskbar`, `SkipSwitcher`, `SkipPager`, `AcceptFocus`. Keyed by title
   as well as app ID, so a program can keep one rule per screen.
2. A transient glance window: show-for-a-duration with coalescing, and a
   documented answer to focus. Possibly `glance.Options.Transient` or a
   section in `docs/glance.md` describing the OSD as a variant of the shape.
3. `glance.Meter` in the OSD's proportions: a large icon, a bar and a number
   on one row, rather than a card's label-caption-bar stack.

## Alternatives Considered

- A daemon plus a client window (hotaru's shape); rejected because the
  original runs fine as one resident process and a socket protocol between
  two halves is more code than the whole switcher.
- A native PipeWire protocol client for the graph (D4); rejected for now,
  revisit if `pw-link` becomes the only subprocess left.
- `wpctl` instead of the native protocol; rejected because it is still a
  subprocess and still text parsing.
