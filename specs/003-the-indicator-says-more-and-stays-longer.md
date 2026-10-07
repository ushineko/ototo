# 003: The indicator says more, and stays longer

**Issue**: #33

## Status: COMPLETE

## Executive Summary

Two things the indicator cannot do. A hotkey connect still goes to the
desktop's notification service for "Connecting..." and for a failure, even
when the person has asked for switches in the indicator, so a key press that
is meant to be a glance at the corner of the screen is a notification popup
with the desktop's own lifetime. And the indicator's hold is 1.5 s, which is
short enough that the value is gone before the eye reaches it. This spec adds
a second routing choice, for what a hotkey says, and makes the hold a setting.

## Context

Spec 001 R8 gave the program its own volume indicator, and D5 gave it desktop
notifications. Settings offers one choice over the two — "On an automatic
switch": say nothing, send a notification, or show it in the indicator — kept
as `switch_notifications` and `switch_in_osd`, the keys the PyQt6 program
wrote.

That choice covers one message, `KindSwitched`. A hotkey connect (spec 002)
sends two more: `KindConnecting` ("Connecting to X", because a Bluetooth
connect takes seconds and the person would otherwise press the key again) and
`KindFailure` ("Switch Failed"). Both are desktop notifications whatever is
chosen. A person whose switches go to the indicator therefore gets a popup for
the slow half of the operation and a panel for the end of it.

The hold is `glance.DefaultHold`, 1.5 s, with 3 s for a switch. Both are
constants in `internal/gui/indicator.go`. On this desk 1.5 s is too short:
the panel is placed on the pointer's screen by the compositor, the eye has to
find it, and the number is gone by then.

## Decisions

- **D1. A hotkey's messages are a choice of their own**, not an extension of
  the switch choice. The two settings answer different questions: "what does
  an automatic switch say" and "what does a key I pressed say". A key press
  has a person in front of it who is waiting; an automatic switch does not.
  `hotkey_in_osd`, one key, off by default, so nothing changes for anybody
  who does not ask.
- **D2. It covers connecting and failing, not switching.** The switch at the
  end of a hotkey connect keeps following `switch_in_osd`: it is the same
  message, about the same event, and two settings that can disagree about one
  notification is a bug waiting to be filed. This does relax spec 001's rule
  that a failure is always a desktop notification — see the risk below.
- **D3. A message is drawn under the meter**, in the line that names the
  device, over the current volume. It is not a new panel shape: the indicator
  stays one thing that says what the audio is doing, and a failure draws in
  the warning colour with the error icon so it is not read as an ordinary
  switch. A message always shows, where a repeated volume snapshot does not:
  two connects in a row are two events, and the person pressed the key twice.
- **D4. One hold setting, and the longer hold derives from it.** A switch and
  a message are worth a longer look than a step of the volume, which is the
  existing 1.5 s / 3 s relation; keeping one dial keeps that relation without
  asking the person to reason about two numbers. `osd_hold_ms`, default
  2500 ms, and the long hold is that plus 1500 ms.
- **D5. The one-shot path is unchanged.** A hotkey pressed while ototo is not
  resident runs `oneShot` in a process with no window, and sends
  notifications. There is no indicator to route to, and starting the whole
  window to draw one panel is worse than the popup.

## Requirements

### R1. Settings

- R1.1 `Config.HotkeyInOSD bool` (`hotkey_in_osd`), default false: a hotkey's
  "Connecting..." and its failures show in the indicator instead of going to
  the notification service.
- R1.2 `Config.OSDHoldMS int` (`osd_hold_ms`), default
  `config.DefaultOSDHoldMS` = 2500; 0 in the file means the default, so a
  `config.json` from the PyQt6 program and from an older ototo both read.
- R1.3 `core.SetSwitches` writes both. The hold is bounded, 500 ms to
  15000 ms, with the error naming the bounds, as the text size does.

### R2. Where a message goes

- R2.1 `notify.Notification` carries `Hotkey bool`: the message is about an
  operation a key press started. It is set by `core`, not by the window.
- R2.2 `core.SwitchRequest.Hotkey bool` marks a switch a key press asked for.
  It is set on the forwarded hotkey path (`gui.handle`) and on the one-shot
  path, and nowhere else: a click in the window is not a hotkey.
- R2.3 The `KindConnecting` and `KindFailure` notifications a hotkey switch
  sends carry `Hotkey: true`. `KindSwitched` does not: it follows
  `switch_in_osd` (D2).
- R2.4 `routingNotifier`: a notification with `Hotkey` true, when
  `hotkey_in_osd` is on, draws in the indicator and is not sent to the bus.
  With the setting off, or from a process with no indicator, it is sent.

### R3. What the indicator draws

- R3.1 A snapshot carries an optional `note` and a `bad` flag. `note`
  replaces the device name; `bad` draws it in the warning colour with the
  error icon. The meter and the value are the current volume, read when the
  message arrives.
- R3.2 A snapshot with a note is never suppressed as a repeat.
- R3.3 A volume read that fails does not lose the message: the message is
  drawn over the last snapshot instead.

### R4. The hold

- R4.1 Every show passes the configured hold: a volume step gets
  `osd_hold_ms`, a switch and a message get `osd_hold_ms` + 1500 ms.
- R4.2 Settings, in the Volume indicator card: "Indicator display time", a
  selector of 1, 1.5, 2, 2.5, 3, 4 and 5 seconds, with a value from an older
  file shown as it is, as the text size row does.
- R4.3 Settings, in the Switching card: "On a hotkey", a selector of "Send a
  desktop notification" and "Show it in the indicator", with the tip saying
  that it covers connecting and failures and that the switch itself follows
  the row above.

## Acceptance Criteria

- [x] R1: the two keys default as specified, an absent `osd_hold_ms` reads as
      the default, and `SetSwitches` rejects a hold outside the bounds.
- [x] R2: a hotkey switch's connecting and failure notifications carry
      `Hotkey`; a window click's do not; `routingNotifier` sends to the bus
      with the setting off and to the indicator with it on, and the switched
      message keeps following `switch_in_osd`.
- [x] R3: a note draws in the device line, a second identical note shows
      again, and a failure note draws in the warning colour.
- [x] R4: the hold in the settings is the hold the indicator is shown for,
      and the long hold is 1500 ms more; both selectors write their key.
- [x] `make screenshots`: the Settings section now has two rows more, so
      the screenshot and its caption are stale until it is run on a KDE
      session.
- [x] Verified on the desktop: the keys with the setting on show their
      message in the indicator and no popup. Keeping the volume under the
      message turned out to carry the panel's identity: it is recognisably
      the switcher's own indicator rather than a second thing in a style of
      its own, which is the reason not to give a message a panel of its own
      (D3). The display time was raised to 4 s on this desk, where it then
      coincides with the switch sound.
- [x] README: the two new configuration keys and a changelog entry.

## Risks & Assumptions

- **A failure in the indicator can be missed.** Spec 001's rule was that a
  failure is always a desktop notification, which is a popup with a
  dismissal. In the indicator it is gone after a few seconds and there is no
  history of it. This is the person's choice to make and it is off by
  default; the reason for allowing it is that the failure of a key press is
  seen by the person who just pressed the key, who is looking at the screen.
  The events log in the window keeps the line either way.
- **The indicator is the resident process's.** With ototo not running, a
  hotkey's messages are notifications whatever the setting says (D5). The
  setting's tip does not say so; the Hotkeys section already says the keys
  forward to the running instance.
- **Rollback**: every change is additive — two settings keys, two struct
  fields and two rows. Reverting the commit restores the previous holds and
  routing; a `config.json` carrying the new keys is read by the old build,
  which ignores them.

## Alternatives Considered

- A text-only panel for a message, with no meter. Rejected: it is a second
  panel shape for one line of text, and the volume under a "Connecting..."
  is information, not noise.
- Folding the hotkey messages into the existing "On an automatic switch"
  choice. Rejected: it would make one row govern two unrelated events, and
  the row is already named for one of them.
