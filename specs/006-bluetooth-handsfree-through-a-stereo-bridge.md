# 006: JamesDSP reaches a Bluetooth headset through a stereo bridge, so handsfree is not silent

**Issue**: #48

## Status: COMPLETE

## Executive Summary

A call that switches a Bluetooth headset to handsfree went silent, because
JamesDSP outputs nothing into a mono HFP sink (JDSP4Linux #298) and WirePlumber
0.5.14 removed the stereo loopback that used to hide it. ototo now owns that
loopback: a `pw-loopback` child starts on the first Bluetooth route, JamesDSP
plays into it, and its playback side is linked to the headset — FL/FR on A2DP,
both channels summed into MONO on handsfree, re-wired on every profile change
because the switch destroys the Bluetooth node.

Reviewers should look first at `internal/bridge/bridge.go` (the child's lifetime
and the `node.autoconnect=false` argument, without which WirePlumber links the
stream into JamesDSP's own sink and makes a feedback loop) and at the relink
paths in `internal/graph/graph.go` and `internal/core/switch.go`, where the
non-Bluetooth call sequences had to stay byte-identical. The mono downmix drops
the bridge stream to 0.5, since summing both channels into one port clipped by
up to 6 dB in the first run.

## Context

When a call switches a Bluetooth headset from A2DP to handsfree
(`headset-head-unit`), everything routed through JamesDSP goes silent: call
audio and the line-in. Dropping back to A2DP restores it.

JamesDSP outputs silence into a mono sink (`playback_MONO`), even in
passthrough: [JDSP4Linux #298](https://github.com/Audio4Linux/JDSP4Linux/issues/298).
WirePlumber up to 0.5.13 put a stereo loopback in front of every Bluetooth
sink, so JamesDSP never saw the mono port. WirePlumber 0.5.14 removed that
loopback (MR !794). R6.2's relink now pairs JamesDSP's sorted outputs with the
sink's sorted inputs, which against a handsfree sink links only
`output_FL → playback_MONO` — the configuration #298 describes.

Verified on the development host, 2026-11-21, WirePlumber 0.5.19, PipeWire 1.6.11, ototo
stopped, by ear and by `btmon -S` (distinct SCO TX payloads):

| Route into the handsfree sink | Result |
|---|---|
| JamesDSP direct | silent; 1 distinct TX payload (encoded silence) |
| no JamesDSP, line-in `pw-loopback` direct | audible; 533/533 distinct |
| JamesDSP → stereo `pw-loopback` bridge → sink | **audible (mono); 533/533 distinct** |

The bridge that worked: capture side an `Audio/Sink` with `audio.position=[FL,FR]`;
playback side `node.autoconnect=false`, wired with `pw-link` — FL→FL, FR→FR on
A2DP, FL and FR both → MONO on handsfree. Findings that shape the design:

- With autoconnect, WirePlumber ignored `target.object` and linked the bridge's
  output into `jamesdsp_sink`, a feedback loop once JamesDSP feeds the bridge.
- The profile switch destroys and recreates the Bluetooth node, taking the
  bridge→sink links with it.
- JamesDSP will not take a virtual sink as its own output device, and re-links
  itself to its configured device when a new stream starts; ototo's relink
  already wins that contest on each switch and tick.
- ototo's stream move (`MoveSinkInputs`) moves every sink-input, which would
  carry the bridge's own playback stream into `jamesdsp_sink`.
- A `pw-loopback` property value with spaces (`node.description="a b"`) made the
  process exit at once when launched through `systemd-run`.

## Decisions

- **Scope: every Bluetooth sink, in every profile** (operator choice). One path;
  JamesDSP's link target stays the same node across A2DP↔handsfree, which is what
  WirePlumber ≤0.5.13 did.
- **Owner: a `pw-loopback` child of the resident process** (operator choice),
  the same shape as R9.3's line-in fallback in `internal/loopback`.
- Started lazily, the first time a switch routes JamesDSP to a Bluetooth sink,
  and kept until ototo exits. Restarted on the next switch or tick if it has died.
- Non-Bluetooth routing is unchanged.

## Requirements

### R1. The bridge node

- R1.1 One child process: `pw-loopback` with capture props
  `node.name=ototo_bt_bridge media.class=Audio/Sink audio.position=[FL,FR]` plus a
  description that has no spaces, and playback props
  `node.name=ototo_bt_bridge_out node.autoconnect=false`.
- R1.2 Started on first need (R2), never more than one; a dead child is restarted
  on the next switch or tick.
- R1.3 Stopped when ototo exits (signal and normal exit), leaving no orphan.
- R1.4 If `pw-loopback` is absent or the bridge does not appear, this is reported
  once and R2 falls back to the current direct relink (R6.2), so nothing regresses.

### R2. Routing JamesDSP to a Bluetooth sink

- R2.1 When R5.1 routes through JamesDSP and the target sink is a Bluetooth sink
  (`bluez` in the sink name or `device.api=bluez`), JamesDSP's outputs are relinked
  to the bridge's playback ports (FL→FL, FR→FR), and the bridge's output is wired
  to the target sink (R3).
- R2.2 Any other target: R6.2 as today.

### R3. Wiring the bridge to the sink

- R3.1 The bridge's existing output links are removed, then: if the sink has a
  single `playback_MONO` port, both `output_FL` and `output_FR` link to it;
  otherwise outputs pair with the sink's sorted playback ports, as R6.2.
- R3.2 Re-asserted after every server change event that can recreate the sink
  (sink/card new or change) and on every auto-switch tick, so the handsfree switch
  is re-wired without waiting for the next tick when events arrive.
- R3.3 Idempotent: when the links are already correct nothing is unlinked.
- R3.4 Level: into a mono port both channels are summed, which clipped audibly in
  the first E2E run (operator, 2026-11-21). The bridge stream's gain is set to 0.5
  linear (-6 dB, an (L+R)/2 downmix) while wired to a mono port and to unity while
  wired to a stereo one; it is written only when it differs. Confirmed by ear with
  a manual -6 dB before implementing.

### R4. The bridge's stream is not moved

- R4.1 `MoveSinkInputs` skips the sink-input whose node is `ototo_bt_bridge_out`.

### R5. Auto-switch sees through the bridge

- R5.1 Where `decide` compares where JamesDSP plays with the wanted sink, a target
  of `ototo_bt_bridge` resolves to the sink the bridge's output is wired to. With
  the bridge wired to the wanted sink, a tick does not switch again.

### R6. Volume follows the hardware

- R6.1 When the default is `jamesdsp_sink` and JamesDSP plays into the bridge, the
  volume the window shows and the keys change is the Bluetooth sink's (spec 001
  R6.3), not the bridge's.

## Acceptance Criteria

- [x] R1: the bridge child starts once on first Bluetooth route, is restarted after
      being killed, and is gone after ototo exits.
- [x] R1.4: with `pw-loopback` unavailable, a Bluetooth switch relinks JamesDSP
      directly as before and reports the bridge failure once.
- [x] R2/R3: unit tests over the fake graph — Bluetooth target relinks JamesDSP to
      the bridge and wires bridge→sink; stereo sink pairs FL/FR; mono sink gets both
      outputs on MONO; already-correct links cause no unlink.
- [x] R3.4: unit test — the bridge stream is set to 0.5 when wired to MONO and
      back to 1.0 on stereo; the wire value of 0.5 is the cube root of the norm.
- [x] R4: unit test — the bridge's sink-input is not moved; other inputs are.
- [x] R5: unit test — a snapshot with JamesDSP on the bridge and the bridge on the
      wanted sink decides no switch.
- [x] R6: the volume target with JamesDSP on the bridge is the Bluetooth sink.
- [x] Non-Bluetooth switches produce the same pw-link calls as before (existing
      switch tests pass unchanged).
- [x] **Integration (real PipeWire, development host, WirePlumber 0.5.19)**: the locally
      built ototo routes line-in → JamesDSP → bridge → XM6; audible on A2DP; a call
      switching to handsfree stays audible (operator, by ear) with `btmon -S`
      showing distinct SCO TX payloads; returning to A2DP stays audible; no
      repeated switching in ototo's log over 2 minutes.
- [x] `make` checks (vet, lint, tests) pass.

## Risks & Assumptions

- **Rollback**: revert the commit / reinstall the previous ototo binary; WirePlumber
  stays pinned at 0.5.13 on the development host until this is validated, so the old path
  keeps working there.
- Assumes JamesDSP's self-relink (on new streams) is corrected by R3.2's event and
  tick re-assertion, as today; a brief silence at a new stream start is possible
  and is the same race ototo already has with the hardware sink.
- Extra loopback hop on A2DP: one quantum of latency; not measured.
- Summing FL+FR into MONO raised the level by up to 6 dB and clipped in the first
  E2E run; R3.4 halves the gain. Material with out-of-phase channels loses level in
  the downmix, as any mono downmix does.
- On WirePlumber ≤0.5.13 the bridge feeds WirePlumber's own loopback; harmless, not
  yet tested.
- Shared-code safety: `graph.Relink` and `MoveSinkInputs` gain cases; non-Bluetooth
  call sequences must stay byte-identical (covered by existing tests).

## E2E Test Plan

Environment: the development host, PipeWire 1.6.11, WirePlumber 0.5.19 installed for the test
(0.5.13 restored after unless unpinning is decided), JamesDSP running, line-in
loopback running, Sony WH-1000XM6.

1. Stop the installed ototo; run the worktree build. Expect the bridge sink
   `ototo_bt_bridge` to appear on the first switch to the XM6, and
   `jdsp… → ototo_bt_bridge → bluez_output.<addr>.1` in `pw-link -l`. Line-in
   audible on A2DP. (R1, R2, R3)
2. Start a Teams call with the XM6 mic. Expect handsfree, bridge re-wired to
   `playback_MONO`, line-in and call audible at call quality, `btmon -S` distinct TX
   payloads. (R3.2, integration AC)
3. End the call. Expect A2DP, bridge re-wired FL/FR, audible. (R3.2)
4. Watch ototo's log for 2 minutes: no repeated switch. (R5)
5. Volume keys change the XM6 volume. (R6)
6. Quit ototo: bridge node gone, no `pw-loopback` orphan. (R1.3)

### Results (2026-11-21)

Passed by ear on the development host, by the operator, on the current stack
(WirePlumber 0.5.19, PipeWire 1.6.11): A2DP audible through the bridge, a call
switching to handsfree stayed audible, and the return to A2DP stayed audible.
Run once before the host rebooted and confirmed again after it.

Observed once, not explained: the return from handsfree to A2DP took about a
minute rather than the few seconds expected. Not reproduced, and the cause may
lie outside ototo. ototo's re-assertion tick is 5 s (`gui.TickInterval`), so a
delay of that length is roughly twelve ticks and is unlikely to be the tick
cadence; the BlueZ/WirePlumber re-establishment of the A2DP transport after the
SCO teardown is the more likely place to look. To be raised as its own issue if
it recurs.

Unrelated finding during the same run: the headset negotiates A2DP/LDAC at
96 kHz and is silent in that state, with every graph reading healthy. That is a
codec fault on the host, not a bridge fault — it reproduces with ototo stopped —
and is mitigated outside this repository by excluding LDAC from the offered
codecs. The A2DP legs above were verified on AAC at 48 kHz.

### Prod visibility

Not applicable: desktop application; observed on the operator's machine.
