---
id: "07-integrated-verification"
title: "Run integrated repository gates"
status: done
wave: 4
depends_on:
  [
    "01-failure-containment",
    "02-settings-route-determinism",
    "03-mobile-selector-stability",
    "04-preload-recovery",
    "05-self-update-reload",
    "06-browser-regressions",
  ]
plan: "plan.md"
decision: "../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md"
---

# Task 07: Run Integrated Repository Gates

## Acceptance

- Formatting runs before every other repository-wide gate.
- Typecheck, tests, and lint all pass against the integrated source state.
- Any failure is classified against the changed surfaces before remediation;
  unrelated code or tests are not changed opportunistically.
- The ADR, specs, plan, and task statuses accurately describe the final
  implementation and evidence.

## Owned files

- No application or test files unless a failing gate identifies a defect in an
  earlier task and the primary session assigns a focused remediation.
- Planning/spec/ADR status metadata is updated by the primary session after all
  gates pass.

## Verification

```bash
make fmt
make typecheck
make test
make lint
```

## Output contract

Report every command, exit status, relevant test counts, duration, and any
failure classification. The primary session updates this task, the plan, and
the ADR/spec statuses after accepting the result.

## Result

- `make fmt`, metadata generation, `make typecheck`, `make test`, and
  `make lint` passed on the final integrated tree.
- Backend: 206 test packages passed and 61 packages had no tests.
- Web: 925 files passed with 7,063 tests passed and 4 skipped.
- CLI: 30 files passed with 280 tests passed.
- The initial backend failures were test-environment artifacts: inherited
  `umask 0002` produced shared-writable Go temporary fixtures that the
  repository-parent security validator correctly rejected. Verification
  passed with `umask 022`, `TMPDIR=/var/tmp`, and `GOTMPDIR=/var/tmp`.
