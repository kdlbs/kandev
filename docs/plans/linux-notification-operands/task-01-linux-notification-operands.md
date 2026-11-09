---
id: "01-linux-notification-operands"
title: "Preserve Linux notification operands"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATIONS-001.13
  - AC-PLATFORM-NOTIFICATIONS-001.14
  - AC-PLATFORM-NOTIFICATIONS-001.15
system_design:
  - ../../specs/platform/system-design/linux-notification-operands.md
---

# Task 01: Preserve Linux notification operands

## Summary

Make Linux `notify-send` receive notification text as positional operands,
including option-like text. Add independent subprocess-level regressions and
preserve existing execution behavior.

## In scope

- Test `SystemProvider.Send` through a temporary fake `notify-send` executable;
  separately exercise Linux dispatch if WSL routing makes the provider case
  unavailable. Observe individual arguments and parsed title/body, timeout and
  icon; cover option-like title, option-like body and normal text.
- Establish RED on the missing boundary before changing production, then add
  the terminator and prove GREEN. Test nonzero exit, cancellation after the
  child starts, and `KANDEV_E2E_MOCK=true` without real notifications.
- Keep fixtures isolated, nonparallel where environment is changed, and joined
  on completion; preserve current fallback and other platform behavior.

## Out of scope

- Other notification transports, UI, Office features, configuration,
  dependencies, localization, shell injection claims, or real OS display tests.

## Acceptance

- The subprocess receives configured options and literal, unmodified payload
  operands, including the qualified `--help` body and an option-like title.
- Failure and cancellation remain observable, and mock E2E mode starts no
  notification process; no fixture reaches the user's notification daemon.
- Scoped tests and lint pass after a demonstrated causal regression failure.

## Verification

Run from the repository root only after the later implementation request and
ROOT's resource grant. Use the installed Go 1.26 toolchain. Give every invocation
its own `KANDEV_RUN_QUIET_DIR`; retain and join any asynchronous process handle.
Run the first test once against the test-only change and record its expected
operand-boundary failure, then repeat it after the fix and run the package test
and lint sequentially. If the eventual test file name differs, update the lint
path before running it.

```bash
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d)" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p=1 -count=1 ./internal/notifications/providers -run '^TestSystemProviderLinuxNotification$')
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d)" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p=1 -count=1 ./internal/notifications/providers)
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS='-p=1' bash scripts/lint-go-changed apps/backend/internal/notifications/providers/system_provider.go apps/backend/internal/notifications/providers/system_provider_linux_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/platform docs/plans/linux-notification-operands apps/backend/internal/notifications/providers
git status --short -- docs/plans/linux-notification-operands
```

The lint script uses its normal 3 GiB default, independent of the test's 512 MiB
limit. Record actual outcomes and any skips. Do not infer OS display from a
successful fake transport.

## Files likely touched

- `apps/backend/internal/notifications/providers/system_provider.go`
- `apps/backend/internal/notifications/providers/system_provider_linux_test.go`
- This work order and `plan.md` for execution status and results.
- The paired requirement and design for lifecycle promotion after verification.

## Dependencies

None. A later explicit implementation request and ROOT resource grant are required.

## Risks

- Test fixtures must capture argument boundaries without using the production
  argument construction as their expected parser, and must cleanly stop children.
- Existing `notify-send` parsing may accept options after a positional operand;
  an ordinary-title-only check would miss the qualified failure.

## Parallelism

Sequential.

## Inputs

- [Notification requirements](../../specs/platform/requirements/notifications.md),
  criteria .13-.15, and [Linux transport design](../../specs/platform/system-design/linux-notification-operands.md).
- Existing provider, `runCommand`, inbox service flow, and provider tests.

## Results

On Linux with Go 1.26.0, the focused test command above failed before the fix:
`help body` and `option title` were parsed as help, and `unknown body` exited 2;
ordinary, command failure, cancellation and E2E suppression passed. After the
fix, the same focused command passed all seven cases and the package command
passed. The focused race check also passed:

```bash
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d)" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p=1 -count=1 ./internal/notifications/providers -run '^TestSystemProviderLinuxNotification$')
```

The scoped changed-Go lint passed with zero issues. The fixture invoked only a
temporary symlink to the test executable; it did not contact a desktop bus.
The full-provider test skips on WSL, where the Windows route is selected; no
other platform or actual OS display was exercised.
