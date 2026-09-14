---
id: "07-pre-launch-warning"
title: "Pre-launch reachability warning"
status: pending
wave: 5
depends_on: ["06-settings-surface"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-003
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-003.2
system_design:
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
---

# Task 07: Pre-Launch Reachability Warning

## Summary

A launch against an SSH host currently recorded as unreachable warns before the
attempt and proceeds anyway. The warning informs; it never gates.

## In scope

- A pre-launch warning shown where a launch is initiated, naming the host and
  the age of the last successful probe, or stating that none has been recorded.
- The launch proceeds without a confirmation step. No blocking dialog, no
  disabled action, no extra click on the path to starting work.
- No warning at all while `probing_enabled` is `false`, even when a record
  retained from an enabled period still reads `unreachable`.
- The warning reads the reachability record from the store slice task 06
  created; it issues no fetch of its own.
- Mobile-viewport layout, and the warning exposed to assistive technology as
  text rather than by color alone.
- Copy through `t()` in `tasks.json` across `en`, `pt-pt`, `zh-cn`, `zh-hk`,
  `zh-tw`, with the failure reason rendered as translated copy, never the raw
  token.

## Out of scope

- **Deferred:** the task-card reachability indicator. Retired IDs
  `AC-EXECUTORS-SSH-REACHABILITY-002.8` and `002.9` are not reused; the
  behavior is specified in the requirement document's `## Out of scope`.
- The settings card, the store slice, and the API client — task 06 owns all
  three.
- Blocking, confirming, deferring, or re-routing the launch. The warning is
  informational and the attempt proceeds.
- The non-interactive half of the warning, recorded against the launched
  session. Task 05 owns it.
- Any backend file.

## Acceptance

- Launching against an `unreachable` host shows a warning naming that host and
  the age of the last successful probe, and the launch proceeds without a
  confirmation step.
- When no successful probe has ever been recorded, the warning says so rather
  than rendering an empty or zero age.
- Launching against a `reachable` or `unknown` executor shows no warning, and
  neither does launching while probing is disabled against a retained
  `unreachable` record.

## Verification

```bash
# From apps/web:
pnpm vitest run components/task/launch-warning.test.tsx
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task/launch-warning.tsx`
- `apps/web/components/task/launch-warning.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/tasks.json`

## Dependencies

Task 06. The warning reads the store slice and the record type that task
creates.

## Risks

- The pre-launch warning sits on the path to starting work. A warning that
  accidentally becomes a confirmation step converts an informational surface
  into the gate REQ-003 forbids; assert the launch proceeds unattended, with no
  user interaction between the warning and the attempt.
- The last-success age can be absent, which is a different sentence from a zero
  age. Rendering `0m` where "never" is meant misreports a host that has never
  once answered as one that just did.
- This work order is now a single criterion. Resist folding it into task 06:
  it is a launch surface with its own locale namespace, and merging it would
  put the settings page and the launch path in one commit.

## Parallelism

`sequential`

## Inputs

- System design, sections *Frontend components* (Launch surfaces) and
  *Launch interaction*.
- Task 05 for the non-interactive half of the same criterion.
- The `mobile-parity` skill for native mobile interaction expectations.

## Results

Pending.
