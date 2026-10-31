# 004: Device names a person recognises

**Issue**: #41, #42

## Status: COMPLETE

## Executive Summary

A USB device is named by its own card name where the USB ID database
disagrees with it, without a database vendor the device does not claim
(#41), and a wired device in the order is named, while away, by the name it
had when last seen, kept in `device_names` (#42). Reviewers should look
first at `ownUSBName` in `internal/devices/devices.go` and at
`rememberedNames` in `internal/core/status.go`, which writes the settings
file from the status read.

## Context

Spec 001 R4.2 names a sink the way audio-source-switcher did: the Bluetooth
alias, else `device.vendor.name` + `device.product.name`, else
`device.description`, else the sink name. R4.3 names a remembered device with
no sink by its Bluetooth alias, else by its id.

Two rows on a real machine come out wrong.

- **#41.** For a USB device, PipeWire fills `device.vendor.name` and
  `device.product.name` from the system's `usb.ids` database, by vendor and
  product id. A device built on another company's chip reports that chip's
  ids. A UGREEN BT701 transmitter reports CSR's `0a12:4007`, which `usb.ids`
  lists as an Mpow headset, so the row reads "Cambridge Silicon Radio, Ltd
  Mpow HC5 Headset in charging mode - HID / Mass Storage". The device's own
  strings are correct and are in the ALSA card properties:

  | Device | `device.vendor.name` | `device.product.name` | `alsa.card_name` | `alsa.long_card_name` (before " at ") |
  |---|---|---|---|---|
  | UGREEN BT701 | Cambridge Silicon Radio, Ltd | Mpow HC5 Headset in charging mode - HID / Mass Storage | UGREEN-BT701 | TaiYiLian UGREEN-BT701 |
  | CUBILUX CB5 | Realtek Semiconductor Corp. | CUBILUX CB5 | CUBILUX CB5 | Generic CUBILUX CB5 |
  | Arctis Nova Pro Wireless | SteelSeries ApS | Arctis Nova Pro Wireless | Arctis Nova Pro Wireless | SteelSeries Arctis Nova Pro Wireless |
  | NVIDIA HDMI (PCI) | NVIDIA Corporation | AD102 High Definition Audio Controller | HDA NVidia | HDA NVidia |

  The NVIDIA row shows why the rule cannot be "always prefer the card's
  name": for a PCI device the database name is the better one.

- **#42.** A non-Bluetooth device in the priority order whose sink is gone
  is listed by its id, `alsa_output.pci-0000_01_00.1.hdmi-stereo-extra1
  [Disconnected]`. Nothing that names it exists once the sink is gone, so
  the name has to be remembered while it is there.

## Decisions

- **D1. The device's own strings decide, for USB only.** On a sink with
  `device.bus = "usb"`, the card name (`alsa.card_name`, else
  `api.alsa.card.name`) is what the device reports as its product (ALSA's
  usb-audio driver takes it from the USB `iProduct` string), and
  `alsa.long_card_name` before " at " is its manufacturer followed by that
  product. These are compared with the database's names. Every other bus
  keeps R4.2 as it is.
- **D2. Product: the card name wins when the two disagree.** The names agree
  when, compared case-insensitively over letters and digits only, one
  contains the other. Containment rather than equality, because ALSA cuts a
  card name at 31 characters and the database name is then the longer form
  of the same thing; when they agree, the database name is kept.
- **D3. Vendor: the prefix stays only when the device agrees with it.** The
  manufacturer string from the long card name is compared with
  `device.vendor.name` by the same rule. "SteelSeries" agrees with
  "SteelSeries ApS" and the Arctis keeps its prefix, unchanged, which `isHeadset` needs
  and the person expects. "Generic" and "TaiYiLian" do not agree with
  "Realtek Semiconductor Corp." and "Cambridge Silicon Radio, Ltd", so those
  rows carry the product alone. The manufacturer string itself is not used
  as the prefix: "Generic" says nothing, and "TaiYiLian" is the OEM rather
  than the brand already in "UGREEN-BT701". When there is no manufacturer
  string to compare (no long card name, or one that does not end in the
  card name), the prefix stays, as in R4.2.
- **D4. Microphones follow the same rule** without separate work, because
  `DisplayName` names sources too and a USB source carries the same
  properties.
- **D5. A remembered name is kept in `config.json`**, under a new key
  `device_names`: priority id to the name the row last had while the sink
  was live, with the port and without state suffixes ("Speakers - Line Out",
  not "... [Disconnected]"). The port is kept because it is what tells two
  HDMI outputs of one card apart.
- **D6. `core.Status` writes it.** It is the read the window makes on every
  refresh and the only one that runs whether the auto-switch is on or off.
  It writes only when a live, non-Bluetooth device in the priority order has
  a name that differs from the one stored, so a steady machine writes
  nothing. The map holds priority ids only; an entry whose id has left the
  order is dropped at the next write. `ototo --status` goes through the same
  function and can therefore write the file. That is accepted: it writes
  only what the window would have written, and only on a machine that
  already has a priority order, which means a settings file already exists.
- **D7. Bluetooth ids are not remembered.** They are named by the adapter
  while away (R4.3), which is the current name and knows about a rename on
  the phone. This keeps the scope to what #42 asks.

## Requirements

### R1. USB display names (#41)

- R1.1 For a sink or source with `device.bus = "usb"` and a non-empty card
  name, when the database product and the card name disagree (D2), the
  product used is the card name.
- R1.2 For the same sink, the vendor prefix is dropped when the long card
  name carries a manufacturer string before the card name and that string
  disagrees with `device.vendor.name` (D3). With no long card name, or one
  that does not end in the card name, the prefix stays.
- R1.3 A non-USB sink, a USB sink with no card name, and a Bluetooth sink are
  named exactly as spec 001 R4.2 names them.
- R1.4 Spec 001 R4.2 records the change and points here.

### R2. Remembered names for disconnected devices (#42)

- R2.1 `devices.Device.Label` is the row's name with its port and without
  its state suffixes, for a live sink.
- R2.2 `config.Config.DeviceNames map[string]string` (`device_names`); a file
  without the key reads as an empty map.
- R2.3 `devices.Inputs.Names` carries it to the list. An offline
  non-Bluetooth id is named by its remembered name, followed by
  ` [Disconnected]`; with no remembered name, by its id, as now.
- R2.4 `core.Status` saves the remembered names under D6's conditions.
- R2.5 Every place that builds the list passes the remembered names, so the
  window, the auto-switch, `--connect`, the hotkeys and the tray name a
  device the same way.
- R2.6 Spec 001 R4.3 records the change and points here.

## Acceptance Criteria

- [x] UGREEN BT701's property set is named "UGREEN-BT701"; CUBILUX CB5's is
      "CUBILUX CB5"; the Arctis's is "SteelSeries ApS Arctis Nova Pro Wireless",
      as today;
      the NVIDIA HDMI set is "NVIDIA Corporation AD102 High Definition Audio
      Controller" (unit tests over property sets from the table above,
      serials and bus paths invented).
- [x] A USB card name cut short by ALSA still counts as agreeing, and the
      database name is kept (unit test).
- [x] The Arctis is still recognised as the headset, battery suffix and all
      (existing test, unchanged).
- [x] An offline USB or HDMI id with a remembered name is listed as
      "<name> [Disconnected]"; without one, as its id (unit tests).
- [x] `core.Status` against a demo server writes `device_names` for a live
      priority device to a real `config.json`, does not rewrite the file when
      nothing changed, and a second `Status` with that sink gone lists it by
      the remembered name (core test, real file under `t.TempDir()`).
- [x] Spec 001 R4.2 and R4.3 updated.
- [x] `make test` and `make lint` pass.
- [x] Changelog line under `### Unreleased` in `README.md`.

## E2E Test Plan

Environment: the developer's machine, ototo built from this branch and
installed with `install.sh`, running in the tray.

1. Open Outputs. The UGREEN transmitter's row reads "UGREEN-BT701"; the
   CUBILUX row reads "CUBILUX CB5"; the Arctis row is unchanged
   ("SteelSeries ApS Arctis Nova Pro Wireless") and shows its battery. (R1)
2. With every device in the order connected once, `config.json` has a
   `device_names` entry for each non-Bluetooth id in `device_priority`. (R2.4)
3. Unplug the UGREEN transmitter. Its row becomes "UGREEN-BT701
   [Disconnected]" in its place in the order. Plug it back in; the row is
   live again under the same name. (R2.3)
4. Microphone section: the CUBILUX and UGREEN inputs, if present, are named
   by the same rule. (D4)

### Results (2026-10-31)

- Step 1: verified with `ototo --status` on the installed build. The
  transmitter reads "UGREEN-BT701", the DAC "CUBILUX CB5 - Headphones", the
  Arctis "SteelSeries ApS Arctis Nova Pro Wireless", the NVIDIA HDMI output
  unchanged.
- Step 2: verified. `device_names` was written with an entry for each live
  non-Bluetooth id in the order and nothing else in the file changed. The
  transmitter is not in the order and is not remembered, as D6 says. The
  second HDMI output was away throughout, so it still reads by its id until
  it is seen once.
- Steps 3 and 4: verified by hand on the installed build.

### Prod visibility

Not applicable: a desktop program with no telemetry. A person sees the row
names.

## Risks & Assumptions

- **Assumption: `alsa.card_name` is the device's `iProduct`.** True of the
  ALSA usb-audio driver on this machine's three USB devices; a driver quirk
  table can override it for some devices, in which case the name is the
  quirk's, which is also the device's rather than the database's.
- **Risk: a lost update between two writers of `config.json`.** Every core
  operation loads, changes and saves the whole file without a lock, so two
  at once can drop one's change. D6 adds a writer, but one that writes only
  when a name changes, which on a steady machine is never; the risk does not
  grow in practice and is not addressed here.
- **Risk: a name changes for a person used to the old one.** The CUBILUX row
  loses "Realtek Semiconductor Corp."; the UGREEN row changes entirely. Both
  are the fix #41 asks for.
- **Shared code**: `isHeadset` matches on the display name; the Arctis keeps
  "SteelSeries" and "Arctis Nova", so it is unaffected.
- **Rollback**: revert the PR. An older ototo reading a file with
  `device_names` ignores the unknown key.

## Alternatives Considered

- Considered naming every USB device by `node.nick`; rejected because it
  drops the vendor from the Arctis and is not the device's name for every
  driver.
- Considered using the long card name's manufacturer as the prefix;
  rejected because it is "Generic" on the CUBILUX and an OEM on the UGREEN.
- Considered remembering names in the window rather than in core; rejected
  because the auto-switch, the hotkeys and `--connect` build the same list
  and would name the device differently.
