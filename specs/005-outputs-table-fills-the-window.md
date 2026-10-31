# 005: The Outputs table fills the window and keeps its controls in place

**Issue**: #43

## Status: COMPLETE

## Executive Summary

The Outputs section is a `Border` with the device table in the centre, so
the table takes the height left below the controls, down to about four rows,
and the page no longer scrolls the buttons away or leaves the table's
scrollbar describing rows below the window. The old column needed 768 units,
more than the default 760-unit window. Reviewers should look first at the
end of `buildOutputs` in `internal/gui/views_outputs.go`.

## Context

The Outputs section is one `VBox` (`internal/gui/views_outputs.go`): the
heading, the Sound server and Volume cards, the action buttons, the
auto-switch check, and the device table pinned at `outputsTableHeight`
(360). The shell puts every section in its content scroller.

When the window is shorter than that column:

- the page scroller takes over and the bottom of the table lies below the
  window edge;
- the table's scrollbar spans its full 360 units rather than the part on
  screen, so a row can be scrolled to and still be off-screen;
- Fyne gives the wheel to the innermost scroller under the pointer and does
  not pass it on, so over the table the page cannot be scrolled;
- the page scroller can carry Switch to, Connect, Disconnect, Move up and
  Move down off-screen, against the design system's rule that a control
  which starts work stays in position.

fynedesygn `docs/design-system.md` ("A control that starts work stays in
position") gives the shape for a section built around one large table:
`Border(top, bottom, nil, nil, FixedHeight(table, N))`, with no scroller of
its own.

## Decisions

- **D1. Border, table in the centre.** The heading, both cards, the action
  row and the auto-switch check are the top; the table is the centre and
  takes the height the window leaves. The shell's scroller sizes its content
  to at least the viewport, so the Border fills the window and the page
  scroller does not engage while the section's minimum height fits.
- **D2. `outputsTableHeight` becomes a minimum of about four rows.** It is
  what keeps the table usable in a short window, and it is small enough that
  the section's minimum fits a normal window. Below that height the page
  scroller does engage, as it must for any section.
- **D3. The table's height follows the window.** The comment on
  `outputsTableHeight` gave "keeps the section still while a person reads
  it" as the reason for a fixed height. Only a resize changes the height
  now; an operation, a refresh or a volume step does not, so the section
  still holds still while it is being read.
- **D4. No library change.** The pattern exists in fynedesygn.

## Requirements

- R1. `buildOutputs` returns a `Border` with the table, in its
  `FixedHeight` minimum, as the centre and every other part in the top.
- R2. `outputsTableHeight` is the minimum for the header and about four
  rows.
- R3. The no-server and not-yet-loaded states, which have no table, are
  unchanged.
- R4. `updateVolumeInPlace` and `redrawTable` still swap the table inside
  the same holder, so a volume step does not rebuild the section.

## Acceptance Criteria

- [x] In a window shorter than the old column (the section in a vertical
      scroller, as the shell hosts it), the section's minimum height fits,
      the table's bottom edge is inside the window, and the action buttons
      are inside it too (headless GUI test).
- [x] In a taller window the table is taller: its height follows the space
      left, not a constant (same test).
- [x] Existing Outputs tests pass unchanged (volume in place, move,
      selection).
- [x] `make test` and `make lint` pass.
- [x] Screenshot harness at a short window height (checked by hand on the installed build instead): no page scrollbar, the
      buttons visible, the table's scrollbar matching its visible rows.
- [x] Changelog line under `### Unreleased` in `README.md`.

## E2E Test Plan

Environment: the developer's machine, ototo built from this branch and
installed with `install.sh`.

1. Open Outputs and shrink the window until the table has room for about
   four rows. No page scrollbar appears; Switch to, Connect, Disconnect,
   Move up and Move down stay visible. (R1, R2)
2. With more devices than fit, the table's scrollbar reaches its last row
   with that row on screen, and the wheel over the table scrolls the table.
   (R1)
3. Enlarge the window; the table grows and the controls do not move. (D3)
4. Change the volume with the keys; the section does not rebuild and the
   selection stays. (R4)

### Results (2026-10-31)

Steps 1 to 4 verified by hand on the installed build.

### Prod visibility

Not applicable: a desktop program with no telemetry.

## Risks & Assumptions

- **Assumption: the shell's content scroller gives its content at least the
  viewport's height.** Fyne's `Scroll` lays its content out at
  `max(content.MinSize(), size)`, which is what lets a `Border` fill the
  window.
- **Risk: a very short window.** Below the section's minimum (top plus four
  rows) the page scroller engages and the problem described in Context
  returns in that range. Four rows keep that range below any window a
  person would use for this section.
- **Rollback**: revert the PR; the change is confined to one function and
  one constant.
