# 002: A Hotkeys section

**Issue**: #30

## Status: INCOMPLETE

## Executive Summary

(populated before the PR)

## Context

Spec 001 R11 gave the program the volume keys (a switch in Settings, what
held them recorded and given back) and, in R11.6, keys of the person's own
through `--desktop bind KEY ARGS` and `--desktop unbind KEY`. The bindings
are command shortcuts registered with kglobalaccel, each a desktop entry
named for its key. The program forgets them the moment the command returns:
the settings file does not carry them, nothing in the window lists them,
and a command shortcut a binding replaced is released and not recorded, so
nothing can give it back.

The person has four such keys on their desktop, made by hand: Meta+A
connects the AirPods, Meta+Z the WH-1000XM6, Meta+Num++ and Meta+Num+-
step the volume. The AirPods key matters because AirPods do not always
auto-connect when an iOS device is in range; the key is how they are
reached.

This spec makes the keys a setting: listed, edited, saved, applied, and
undone from the window, with one click back to the desktop's stock keys and
one click back to the person's own. On a desktop that is not KDE Plasma the
program cannot bind anything, and says how the person can.

## Decisions

- **D1. The settings file is the record.** `Config.Hotkeys` lists every key
  of the person's own, with what it runs. The desktop is made to match the
  file, never read as the truth, except once: bindings made by the command
  line before this spec are adopted into the file the first time the
  section reads them, so nothing the person made is lost.
- **D2. One switch turns them all on and off.** `Config.HotkeysEnabled`
  says whether the keys in the file are bound on this desktop. Off, every
  binding is removed and what each replaced is given back; the list is
  kept. On, every binding is made again. "Restore stock keys" is this
  switch off together with the volume keys off; "Use my keys" is both on.
- **D3. What a key replaced is recorded and given back**, as the volume
  keys do: a command shortcut released by a binding goes into
  `hotkeys.json` beside `volume-keys.json`, and unbinding registers it
  again. A foreign component's shortcut is not taken (unchanged from R11.6:
  a program's own keys are its to give up in System Settings) and the
  binding is refused with the reason.
- **D4. A key is typed, not captured.** The entry takes the key as System
  Settings writes it, "Meta+A", checked by `ParseKey` as it is typed and
  committed on Enter. Capturing a key press in the window cannot work for
  the keys that matter: a key kglobalaccel already holds never reaches the
  window on Wayland.
- **D5. Actions are three.** `connect` with a device, `vol-up`, `vol-down`.
  The device is the priority id (`bt:<MAC>` or the sink name), which is
  stable; the command line's free-form `ARGS` narrows to these, since a
  key that runs anything else is not a hotkey of this program's.
- **D6. Plasma or not.** kglobalaccel answering on the session bus is the
  test, not the desktop's name: it is what the bindings need. Without it the
  section is a page: the three commands with the person's device ids filled
  in, and a sentence that the desktop's own shortcut settings bind them.
- **D7. Core first.** Every operation is a core function over the config
  and a `keyBinder` the tests replace; the window and `--desktop bind|unbind`
  render it.

## Requirements

### R1. Settings

- R1.1 `Config.Hotkeys []Hotkey{Key, Action, Device}`, `Config.HotkeysEnabled bool`
  (default true: a key added is a key wanted).
- R1.2 A key appears once; setting a key that another hotkey holds moves it.
- R1.3 Adoption (D1): a `io.ushineko.ototo.bind-*.desktop` entry whose
  `Exec` is one of the three actions and whose key is not in the file is
  added to the file; one whose `Exec` is anything else is reported and left.

### R2. Core operations

- R2.1 `Hotkeys(ctx, req)` → the list with each key's state: bound, unbound,
  refused (with the reason), plus `Supported bool` (D6) and the volume keys'
  state.
- R2.2 `SetHotkey(ctx, req{Action, Device, Key})`: saves; when enabled and
  supported, unbinds the old key for that action/device and binds the new;
  empty key removes. Errors from the desktop leave the file changed and
  come back with the state.
- R2.3 `SetHotkeysEnabled(ctx, req{On})`: saves; binds or unbinds every
  hotkey, restoring holders on unbind (D3); every item is done even when
  one fails, errors joined.
- R2.4 `RestoreStockKeys(ctx, req)`: R2.3 off, then the volume keys off.
  `UseMyKeys(ctx, req)`: R2.3 on, then the volume keys on.
- R2.5 `--desktop bind KEY --connect DEV | --vol-up | --vol-down` and
  `--desktop unbind KEY` are R2.2.

### R3. Desktop layer

- R3.1 `desktop.Bind` records the released holders per key in
  `hotkeys.json`; `desktop.Unbind` gives them back and drops the record.
- R3.2 `desktop.ListBindings()` reads the entries under applications/ and
  the shortcuts file: key, args, and whether kglobalaccel has the key.
- R3.3 `desktop.HotkeysSupported(ctx)`: kglobalaccel answers on the bus.

### R4. The section

- R4.1 "Hotkeys" between Microphone and Settings. Supported: a card per
  group. **Devices**: every device in the priority order (online or not),
  name, an entry for its key, its state. **Volume**: rows for Volume up
  and Volume down keys of the person's own, and the volume-keys switch
  moved here from Settings. **Everything**: "Use my keys" and "Restore
  stock keys", and a line saying what is bound now.
- R4.2 Not supported: the page of D6, with the device ids from the list.
- R4.3 Each change runs through `Perform` and refreshes the section's
  state in place; the entry's validator shows a bad key before Enter.

## Acceptance Criteria

- [ ] R1: the config carries hotkeys and the switch; a key set twice moves; bindings made before this spec are adopted once, tested with a fake applications dir.
- [ ] R2: core operations over a fake binder, with tests for set/move/remove, all-off restoring holders in order, all-on, the two one-click operations, and errors joined and reported.
- [ ] R3: `Bind` records what it released and `Unbind` gives it back (tested over a fake shortcuts file for the record; the bus calls exercised on this desktop); `ListBindings` parses the person's four entries; `HotkeysSupported` false without the bus.
- [ ] R4: the section over the demo server in both states, screenshot in the gallery; the volume-keys switch is in Hotkeys and not in Settings.
- [ ] `--desktop bind/unbind` go through core and land in the file.
- [ ] Verified on the desktop: the four existing keys adopted and listed; a key changed, removed and put back; Restore stock gives Volume Up/Down back to the mixer and Meta+A to nothing; Use my keys puts all back; the AirPods key connects them.
- [ ] README: the Hotkeys section, the generic commands, the changelog.

## Risks & Assumptions

- **Assumption: kglobalaccel keeps the registrations across logins**, as it
  did for R11: they are in kglobalshortcutsrc, written by kglobalaccel.
  The program does not re-register at start.
- **Risk: adoption misreads an entry** made by hand with an `Exec` this
  spec does not know. It is reported, never rewritten or removed.
- **Risk: a holder that no longer exists** when a key is given back
  (an uninstalled program's entry). Registering it fails; the failure is
  reported and the record dropped, since there is nothing to give it to.
- **Rollback**: revert the PR; the settings file gains keys older versions
  ignore; bindings on the desktop are the same entries as before.
- **Query budget, migrations, shared code**: none.

## Alternatives Considered

- Capturing the key by pressing it in the window; rejected, D4.
- Reading the desktop as the truth every time; rejected, D1: it cannot say
  which keys are the person's intent once one is refused or removed.
