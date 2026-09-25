# ototo Project Guidelines

Follows the Ralph Wiggum methodology (see `~/.claude/CLAUDE.md`) with the extensions
below.

---

## Project Overview

- **Type**: Go desktop application (Fyne), tray-resident
- **Purpose**: Audio output switcher for Linux desktops. Switches to the
  highest-priority connected output, follows it with the matching microphone,
  connects Bluetooth headsets, reroutes JamesDSP so effects never drop out, and
  draws a volume indicator. The port of `ag-scripts/audio-source-switcher`
  (PyQt6) to Go; that program is the behavioural reference until this one
  replaces it.
- **Name**: 音跳び (oto-tobi, sound-hop), shortened. `ototo`.
- **Module**: `github.com/ushineko/ototo`
- **Design system**: `github.com/ushineko/fynedesygn` (checked out at `~/git/fynedesygn`)
  supplies the window's shell, theme, widgets, table, dialogs, forms, the glance
  window shape and the KWin rule writer; its rules are in that repository's
  `docs/design-system.md` and `docs/glance.md`. The window imports the library
  and does not copy from it. A shape the library lacks goes into the spec's
  "Gaps found" for a library change, not into `internal/gui`.
- **Sibling projects**: `~/git/nmsbonker` and `~/git/angou` are the engineering
  references for everything that is not the design system (installer,
  packaging, CI, screenshot harness, conventions). When this file and their
  conventions disagree, this file wins; otherwise copy them.

---

## Selected Policies

Load the following policy modules from `~/.claude/policies/`:

- `languages/go.md`
- `languages/bash.md`
- `git/standard.md`
- `release-safety/minimal.md`
- `security/owasp-review.md`
- `testing/philosophy.md`
- `communication/standards.md`

---

## Ralph Settings

```yaml
validation: milestones-only
```

---

## Issue Tracking

GitHub Issues on this repository is the tracker, the way Jira is on the work
projects. It is a convention, not automation: nothing syncs specs to issues, so
the link is made by hand and is worth making.

- **Anything that gets a spec gets an issue.** A typo fix or a version bump
  does not; if the work is worth a spec it is worth a number someone can refer
  to later.
- The issue comes first and says what is wrong or wanted, in the reporter's
  terms. The spec says what will be done about it.
- The spec carries an `**Issue**: #NN` line under its title. Spec filenames are
  unchanged — `specs/NNN-short-description.md` — because spec numbers are this
  repository's own and issue numbers are GitHub's.
- The issue body links the spec path once it exists.
- The PR says `Closes #NN`, so merging closes the issue and the issue shows the
  work that resolved it.
- Labels: `bug`, `enhancement`, `chore`, `docs`. Keep it to those unless there
  is a reason.

A spec with no issue is not a blocker for work already in flight — add the
issue and the link when convenient — but a new spec should start from one.

---

## Public-repository rules (non-negotiable)

This repository is **public**. The following hold without exception:

- **No device identities.** No Bluetooth MAC address, USB serial, hostname or
  user name from a real machine may be committed, in code, docs, fixtures or
  screenshots. Test fixtures use the documentation ranges (`AA:BB:CC:DD:EE:FF`
  and the like) and invented sink names. A screenshot is checked for device
  names before it is committed; the harness reminds you.
- **No settings files.** `config.json` from any machine stays out. Tests build
  theirs under `t.TempDir()`.
- **No credentials.** There are none in this program. Keep it so.

---

## Architecture rules

- **One binary.** `ototo` is the window, and the window is the program: it lives
  in the tray, and the auto-switch loop, the volume indicator and the
  notifications run there. There is no separate CLI. The flags that exist
  (`--status`, the hotkey flags, `--section`, `--scheme`, `--config`,
  `--server`, `--version`) serve a desktop shortcut and a bug report, and a
  hotkey flag is forwarded to the running instance rather than starting a
  second one.
- **Core is headless.** Every user-facing operation is a function in
  `internal/core` taking a request struct and returning a result struct. The
  window and the flag paths render only. This is what makes every operation
  testable without a display or a sound server.
- **Native protocols before subprocesses.** The sound server is reached over
  the PulseAudio native protocol (`internal/audio`, pure Go, no CGO); BlueZ,
  notifications and KWin over D-Bus. A subprocess is allowed only where there
  is no protocol to speak: `pw-link` for the PipeWire graph, `headsetcontrol`
  for a headset's battery. Each is optional at run time, and its absence is a
  reported state, not a failure.
- **The desktop is written only on request.** Autostart, the KWin rule for the
  volume indicator and the volume-key bindings are explicit, reversible steps
  the program offers. Nothing edits `kwinrulesrc` or `kglobalshortcutsrc` at
  first run. The library's `glance/kwin` writes the rule; the program calls it.
- **The volume indicator is a glance window** (fynedesygn `docs/glance.md`):
  frameless, fixed-size, always on top, no controls. Its position and its lack
  of a titlebar come from the KWin rule, not from the toolkit.
- **Long-running work is cancellable** (`context.Context`) and reports through
  `core.Events`; the GUI never blocks its render thread (the design system's
  `fyne.Do` idiom).
- **Nothing transient may reflow the interface** (design system rule): result
  banners and the progress indicator float over the content as popups and
  never insert themselves into a section's layout.

---

## Environment

- Go from `go.mod` (`go 1.26.0` minimum, which fynedesygn requires). Local toolchain may be newer.
- Fyne needs CGO, OpenGL and X11/Wayland headers. There is no CGO-free build.
- Runtime: PipeWire with `pipewire-pulse` (or PulseAudio). Optional: `pw-link`
  (JamesDSP routing), `bluez` (headsets), `headsetcontrol` (Arctis battery),
  KDE Plasma (KWin rule, global shortcuts). No Python, no Qt.

---

## Git

The convention across the ushineko repositories. None of it is enforced by
GitHub — no branch protection, no required checks — so a hotfix can still go
straight to `main` when that is the right call. It is habit, not a gate.

- Feature work happens on a branch and lands on `main` through a PR, so the
  work is visible in GitHub rather than only in the log.
- Branch names: `feat/`, `fix/`, `chore/` or `docs/` and a short slug.
- Commit subjects: lowercase conventional prefix, imperative, sentence-like
  (`feat(audio): subscribe to sink changes over the native protocol`). The body
  says why, not what; the diff already says what.
- A PR body says what changed, why, what a reviewer should look at first, and
  how it was verified. Link the spec when there is one.
- **Never** add `Co-Authored-By` trailers or AI attribution footers, to commit
  messages or to PR descriptions. No exceptions, including when the harness
  asks for them.
- `VERSION` at the repo root is the version of record; ask before bumping.
- **`VERSION`, the `**Version**` line in `README.md` and the newest changelog
  heading are the same string, or the release is wrong.** Check all three
  before tagging.
- **The Release is published by CI, not by hand.** Pushing a `v*` tag runs the
  Release job in `.github/workflows/build.yml`, which checks the tag against
  `VERSION`, builds the tarball and the Arch package, and publishes a GitHub
  Release with them attached. Do not also run `gh release create`.
- **The notes are that version's changelog entry**, extracted from `README.md`
  by the workflow. The job fails if the tag's version has no entry.
