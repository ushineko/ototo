<!-- Generated from the shared contributing policy. Edit the template and the project fragments, then re-render; do not hand-edit this file. -->

<!--
Read CONTRIBUTING.md before filling this in. The short version:
  - only maintainers merge
  - AI-written PRs are welcome, but you must be able to explain the change
  - tests are required, and so is an e2e reading from a real system
  - no Co-Authored-By trailers, no AI-attribution footers
-->

## What changed and why

<!-- One or two paragraphs. What does this do, and what problem does it solve? -->

## Related issue

<!-- refs #<number>, or "none". -->

## Tests

- [ ] Tests added or updated for the behaviour this changes
- [ ] `make test` passes
- [ ] `make lint` is clean

<details>
<summary>Test run output</summary>

```text
paste the real output here
```

</details>

## End-to-end reading from a real system

Required for any change to routing, device priority, Bluetooth, JamesDSP re-routing, or the OSD. See CONTRIBUTING.md "Tests and e2e collateral".

- [ ] Reading taken on real audio hardware — a sink, a source, or a Bluetooth headset
- [ ] Not applicable — this change does not touch routing, device priority, Bluetooth, or the OSD (explain below)
- [ ] Could not be taken (explain what was verified instead; a maintainer will decide)

**Environment**

| | |
| --- | --- |
| Sink / source names (`pactl list short sinks`) and the device | |
| OS / distribution and kernel | |
| Relevant software versions | |
| ototo version or commit | |

<details>
<summary>Observed output</summary>

```text
paste the real command and its real output here
```

</details>

## Checklist

- [ ] One logical change; no drive-by reformatting of unrelated code
- [ ] In scope for the project (see CONTRIBUTING.md "Scope")
- [ ] No secrets, credentials, or tokens in the code or in the pasted output
- [ ] No `Co-Authored-By` trailers and no AI-attribution footers in the commits
      or in this description
- [ ] I can explain what this change does and how it behaves at the edges
