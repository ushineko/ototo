# 007: The Bluetooth bridge attaches only to a streaming headset

**Issue**: #52

## Status: COMPLETE

## Executive Summary

ototo 0.1.7 wired its stereo bridge (spec 006) to a Bluetooth headset that
BlueZ was not yet streaming to. A headset resumed that way plays nothing,
while every reading says it is playing. That happened on every restart and
every reconnect. ototo now wakes an A2DP headset with JamesDSP directly and
moves it onto the bridge once BlueZ's transport for it is `active`.
Handsfree is wired as in 0.1.7. Reviewers should look first at
`attachBridge` and `bridgeReady` in `internal/core/switch.go`.

## Context

Spec 006 put a stereo bridge (`ototo_bt_bridge`, a `pw-loopback` child) between
JamesDSP and every Bluetooth sink, so that handsfree is audible on WirePlumber
0.5.14 and later. ototo 0.1.7 shipped it. On the development host the
WH-1000XM6 then went silent on A2DP after an ototo restart and after a
reconnect, with every reading healthy: the sink `RUNNING`, no errors, the BlueZ
transport `active`, and the links the same as when it plays. The host was
reverted to WirePlumber 0.5.13 and ototo 0.1.6.

The trigger was isolated on 2026-11-23 (sysadmin
`docs/wireplumber-0.5.14-hfp-regression.md`, "Silent A2DP on the bridge
stack"). The tests used WirePlumber 0.5.13 with ototo stopped, the bridge built
with ototo's own `pw-loopback` arguments, and a 3 s tone judged by ear:

| How the suspended headset is resumed | Result |
|---|---|
| By a bridge that is wired to it while it is suspended | silent 3/3 |
| By a direct JamesDSP link, then moved onto the bridge (bridge created while suspended) | audible 2/2 |
| By a bridge that had already been wired to it while it was running | audible 2/2 |
| By a direct JamesDSP link | audible 3/3 |

Once silent, a suspend and resume of the headset, or switching outputs in
ototo and back, clears it.

### The handover moment (2026-11-24)

The first build of this spec moved JamesDSP onto the bridge as soon as the
sound server reported the headset's sink `RUNNING`. It was silent on the first
E2E run, and so was the switch chime played straight into the sink. The whole
headset was silent, not only the bridge's stream. Bisected by hand on the same
host (WirePlumber 0.5.13, ototo stopped, a fresh bridge each run, line-in
music judged by ear):

| JamesDSP moved from the direct link onto the bridge | Result |
|---|---|
| 30 to 38 ms after the direct link, as soon as the sink read `RUNNING`, BlueZ transport still `idle` | silent 2/2 (plus the ototo run: 3/3) |
| about 20 ms after the BlueZ transport read `active` (93 to 127 ms after the link) | audible 3/3 |
| several seconds after the direct link | audible 1/1 |

The sink reads `RUNNING` about 10 ms after the direct link, and the transport
goes from `idle` to `active` 60 to 90 ms after that. `RUNNING` is the sound
server starting the node. `active` is BlueZ streaming to the headset, and that
is the moment that counts.

## Decisions

- **D1. JamesDSP wakes the headset, then the bridge takes over.** This is the
  2/2 row of the first table, and option 1 of the write-up's next steps. The
  other option was to cycle the sink after wiring. It was not tested before
  the fact, and it briefly suspends a headset that may already be playing.
- **D2. "Held" means this bridge child, this sink node, wired when ready.**
  The node is identified by its sink index on the sound server, which changes
  whenever PipeWire recreates the node (reconnect, profile switch). A new
  bridge child holds nothing. Once held, the node suspending and resuming
  through the bridge is left alone (row 3 of the first table).
- **D3. Ready is BlueZ streaming.** For a headset whose card is in an A2DP
  profile, the bridge is wired to a node it does not hold only once the node
  is not asleep and BlueZ's `MediaTransport1.State` for the headset is
  `active` (the second table). The card profile comes from the sound server,
  which ototo already speaks to. The transport state comes from BlueZ over the
  system bus, which ototo already reads for the paired devices. No subprocess
  is added.
- **D4. The switch waits briefly; the tick finishes the job.** A switch links
  JamesDSP directly and reads the sink and the transport every 50 ms for up to
  2 s. On this host the transport goes `active` within about 100 ms, because
  the line-in keeps JamesDSP fed. When nothing is playing it may not, and then
  the direct route stays, which is the 0.1.6 route and audible on A2DP. The
  tick and the sink events move JamesDSP onto the bridge once BlueZ streams.
- **D5. While the headset is waking, the direct route is correct.** The
  auto-switch does not treat "JamesDSP straight into the headset" as a reason
  to switch while the bridge serves that headset. Otherwise every tick would
  re-run the switch.
- **D6. Handsfree is wired as in 0.1.7.** Handsfree has no A2DP transport to
  wait for, and JamesDSP straight into its mono port is the silence of #48. A
  card in any profile other than A2DP, or one the server does not report, gets
  the bridge at once, as spec 006 wired it and as its E2E passed by ear. On
  WirePlumber 0.5.13 the headset's sink is WirePlumber's own stereo loopback,
  which carries no profile. That is why the profile is read from the card and
  not from the sink.

## Requirements

### R1. What the decision is made from

- R1.1 `audio.Device.State` is the sink's state from the server:
  `StateRunning`, `StateIdle`, `StateSuspended`, or `StateUnknown`. Unknown is
  not asleep, so a server or a fake that does not report a state routes as
  0.1.7 did.
- R1.2 The demo server carries a `state` per sink; no value means running, so
  existing demo files keep their behaviour.
- R1.3 `audio.Client.BluetoothProfile(address)` is the active profile of the
  card whose `api.bluez5.address` is that address, or "" when there is none.
- R1.4 `bluetooth.Adapter.TransportState(mac)` is the state of the device's
  own media transport: `active` if any of its transports is, "" when it has
  none. It is exposed to core as `Probes.Transport`; a nil probe cannot tell.

### R2. Attaching the bridge

- R2.1 `bridge.Bridge.Ensure` reports whether it started a new child. A new
  child holds no node (D2).
- R2.2 For a Bluetooth sink the bridge serves, ototo:
  - when the bridge holds the sink's current node, or the node is ready (D3,
    D6): wires the bridge to the sink (spec 006 R3), sets the level, links
    JamesDSP to the bridge if it is not already, and records the node as held;
  - otherwise: links JamesDSP directly to the sink if it is not already, and
    does not wire the bridge to it.
- R2.3 A switch to a Bluetooth sink applies R2.2. When that leaves JamesDSP
  direct, it reads the sink and its transport again every 50 ms for up to 2 s
  and applies R2.2 again as soon as the node is ready (D4).
- R2.4 `MaintainBridge`, on every tick and sink or card event, applies R2.2 to
  the sink the bridge serves. With the node held and the links correct, it
  changes nothing (spec 006 R3.3).
- R2.5 A link already correct is not remade: JamesDSP is relinked only when it
  does not already play into the place R2.2 wants.
- R2.6 Bridge failure (spec 006 R1.4) and the non-Bluetooth route are
  unchanged. A bridge that cannot be wired during a switch is the failure
  spec 006 R1.4 reports; on a tick it is a node whose ports are not up yet,
  tried again on the next.
- R2.7 Each change of the bridge's level is logged at debug level with the
  value written.

### R3. The auto-switch accepts the waking route

- R3.1 With the bridge serving the wanted Bluetooth sink and not yet on it,
  JamesDSP playing straight into that sink is a valid state, not a reason to
  switch (D5).

## Acceptance Criteria

- [x] R1.1/R1.2: the client reads the sink state (checked against `pactl`
      on the development host); the demo server's `state` defaults to running
      (`TestTheDemoSinkStateDefaultsToRunning`, `TestTheSinkStateIsThePulseValue`).
- [x] R1.3/R1.4: the card profile and the transport state are read through
      ototo's own code on the development host (`a2dp-sink-sbc_xq`,
      `active`); the transport is the device's own
      (`TestTheTransportIsTheDevicesOwn`).
- [x] R2.2/R2.3: switching to an asleep A2DP headset links JamesDSP directly
      first, wires the bridge only after BlueZ streams, then moves JamesDSP
      onto it (`TestAnAsleepHeadsetIsWokenBeforeTheBridgeTakesIt`, which
      asserts the order of the graph changes).
- [x] D3: a node that is `RUNNING` while BlueZ is not yet streaming stays on
      the direct route, and the tick does not fight it
      (`TestARunningHeadsetWaitsForBlueZ`; fails if the transport is ignored).
- [x] D2: a reconnected node is woken again, a held node is left alone when it
      suspends, and a restarted bridge child holds nothing
      (`TestAReconnectedHeadsetIsWokenAgain`, `TestARestartedBridgeHoldsNothing`).
- [x] D6: a new handsfree node gets the bridge at once at half level
      (`TestHandsfreeIsWiredAtOnce`; fails if handsfree is gated too).
- [x] R2.2: switching to a streaming headset wires the bridge straight away,
      as in 0.1.7, and the spec 006 tests pass unchanged.
- [x] R3: `decide` does not switch while the wanted headset is waking on the
      direct route, and still does when the bridge does not serve it
      (`TestDecideLetsAWakingHeadsetBe`).
- [x] Non-Bluetooth switches produce the same `pw-link` calls as before
      (existing tests pass unchanged).
- [x] `go test -race ./...` and `make lint` pass.
- [x] **Integration (real PipeWire, development host)**: E2E steps 1 to 5
      below, audible by ear.
- [x] Changelog line under `### Unreleased` in `README.md`.

## E2E Test Plan

Environment: the development host, PipeWire 1.6.11, JamesDSP running, line-in
loopback running, Sony WH-1000XM6, ototo built from this branch and installed
with `install.sh`. Steps 1 to 4 on WirePlumber 0.5.13, where the trigger was
isolated. Step 5 on 0.5.19, which is the reason for the bridge.

1. **Restart, the 0.1.7 trigger.** With the XM6 connected and ototo the
   output, quit ototo, wait until `pactl list sinks short` shows the XM6
   `SUSPENDED`, and start ototo. Line-in and switch chime audible. Repeat three
   times. (R2.2, R2.3; 0.1.7 was silent 2/2.)
2. **Reconnect.** Power-cycle the XM6 with ototo running. After it reconnects
   and ototo switches to it, audible. (D2)
3. **Graph.** After steps 1 and 2, `pw-link -l` shows `jdsp… →
   ototo_bt_bridge → bluez_output.<addr>.1`, and ototo's log shows no switch
   repeating. (R2.4, R3)
4. **Handsfree.** A Teams call moves the XM6 to handsfree. The call is audible.
   Ending it returns to A2DP and stays audible. Note how long the return
   takes. (D6)
5. **WirePlumber 0.5.19.** Install it, then repeat steps 1, 2 and 4.

### Results (2026-11-24)

The first build (handover at `RUNNING`) failed step 1: silent, chime
included. That led to the bisection in Context and the transport gate. Every
result below is for the gated build, judged by ear by the operator.

- Step 1 (0.5.13): audible 3/3. Each run's log shows the wake ("BlueZ is not
  streaming … waking it with JamesDSP directly") and then the handover
  ("BlueZ streams … moving JamesDSP onto the Bluetooth bridge"). The bridge
  child exited with ototo each time.
- Step 2 (0.5.13): the XM6 dropped, ototo fell back to the wired output, and
  the XM6 returned as a new node (new sink index). The wake and the handover
  followed, and it was audible.
- Step 3: the graph is as stated, and the log shows no repeated switch.
- Step 4 (0.5.13): handsfree audible; the return to A2DP was audible but took
  about a minute. ototo changed nothing during either profile switch: on 0.5.13
  the headset's sink is WirePlumber's own loopback, which keeps its node, so
  the bridge held it throughout. The delay was the card staying in handsfree
  after the call, before WirePlumber switched it back.
- Step 5 (0.5.19, installed from the pacman cache and kept):
  - Restart: wake and handover as in step 1, audible.
  - Reconnect (power-cycle): wake and handover, audible.
  - Call 1: handsfree audible but popping and distorted, the bridge stream at
    100% on an 8 kHz mono sink. The level write was not instrumented then, and
    the cause is not known.
  - Call 2, instrumented with a timestamped recorder: the handsfree node was
    wired 0.3 s after it appeared and the level set to 0.5 (79% on the
    protocol's cubic scale). It held at that through the call, and the call
    was clean. Hang-up to A2DP took 4.1 s, with the direct route audible from
    then. The bridge took the node 4.4 s later, back at unity.

### Prod visibility

Not applicable: a desktop program with no telemetry. With `OTOTO_DEBUG=1`,
ototo logs the wake, the handover and each change of the bridge level.

## Risks & Assumptions

- **Assumption: BlueZ's transport `active` marks the safe moment.** Measured
  3/3 audible against 3/3 silent before it, on one headset (WH-1000XM6) and
  one host. Another headset may need longer after `active`. The direct route
  it stays on until then is the 0.1.6 route, so a headset that never streams
  keeps playing without the bridge on A2DP.
- **Open: call 1's distortion on 0.5.19.** Not reproduced with
  instrumentation. R2.7's debug line shows whether the level is written, if it
  recurs.
- **Assumption: the node identity (sink index) matters, not only the bridge
  process** (D2). The first table varies both. Keying on both costs at worst
  one direct wake that was not needed.
- **Handsfree is not gated** (D6). It is the spec 006 behaviour, passed by ear
  in spec 006 and in step 5 here.
- **Shared code**: `graph.Relink` is unchanged. `SetStreamLevel` now reports
  whether it wrote, for R2.7. The non-Bluetooth path makes the same calls as
  before.
- **Rollback**: revert the PR and reinstall 0.1.7, or go back to WirePlumber
  0.5.13 with ototo 0.1.6.

## Alternatives Considered

- Considered moving onto the bridge when the sink reads `RUNNING`; rejected
  after it was silent 3/3 (Context, the second table).
- Considered a fixed delay after the direct link; rejected for the transport
  state, which is the event the delay would be guessing at.
- Considered cycling the headset (suspend and resume) after first wiring a
  bridge; rejected because it was not tested before the fact and interrupts a
  headset that may be playing.
- Considered detecting the silent state and repairing it; rejected because
  every reading is healthy in that state, so there is nothing to detect.
