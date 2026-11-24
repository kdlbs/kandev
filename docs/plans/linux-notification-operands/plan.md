---
created: 2026-10-27
status: implemented
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
system_design:
  - ../../specs/platform/system-design/linux-notification-operands.md
legacy_specs: []
---

# Implementation Plan: Linux Notification Operands

## Overview

One sequential work order repairs the Linux System provider's command boundary.
The supplied qualification exercised the actual provider and installed
`notify-send` against an absent task-owned D-Bus socket: an `Inbox: decision`
summary with body `--help` printed help and returned success without sending;
ordinary text and the same option-like body after an explicit terminator reached
the isolated transport. The parser also rejected `--experimental` as an option.
The cause is `sendLinuxNotification` appending title and body immediately after
options without terminating option parsing.

## Scope and approach

Place the option terminator before both operands in the `notify-send` branch;
retain timeout, optional icon, direct execution, cancellation, error handling,
and E2E suppression. `HandleInboxItem` already forwards the item title as body;
the fix applies to every `SystemProvider.Send` caller. No new dependency,
settings, UI, localization, generic notification framework, or changed desktop,
WSL, Windows, Darwin, `zenity`, or Office feature is in scope. This is an
argument-parsing defect, not a shell execution claim.

| Path | Contract and evidence |
| --- | --- |
| Linux `notify-send` | Literal title and body after options; test real provider-to-subprocess boundary with a temporary executable and isolated output. |
| Linux `zenity` fallback | Existing combined-text transport; unchanged. |
| WSL, Windows, Darwin | Existing platform transports; unchanged. |
| Tauri desktop | Native notifier and suppression of duplicate backend delivery remain governed by [ADR 0039](../../decisions/0039-native-desktop-integration-boundary.md). |

## Tests and E2E applicability

In `apps/backend/internal/notifications/providers/system_provider_linux_test.go`,
add `TestSystemProviderLinuxNotification` against the real `SystemProvider.Send`
and an isolated `notify-send` executable selected through a temporary `PATH`.
Record distinct arguments in an unambiguous format, validate their parsed
roles, and exercise option-like title and body (including `--help` and an
unknown flag), ordinary text, and timeout/icon preservation (.13-.14). The
test must fail before the fix for the expected operand-boundary reason. Simulate
a nonzero subprocess exit and an in-flight cancellation (.14); separately
assert mock E2E suppression launches no executable (.15). Use a nonempty test
icon path to avoid asset writes, disable sound, isolate environment changes,
and avoid parallel environment mutation. Where WSL detection would change the
route, exercise the Linux dispatch directly or skip the full-provider case with
an explicit reason. Never reach a real notification daemon.

The provider-to-subprocess check is end-to-end evidence for the changed
transport boundary, not evidence of OS display. Existing service routing
establishes the inbox entry path. There is no rendered UI change, so a browser
Playwright scenario, mobile markup test, or UI preview would not exercise the
defect. Public documentation assessment: `docs/public/operations.md` describes
provider selection and `docs/public/desktop-app.md` describes the separate
native path; both remain accurate. No public documentation, API, terminology,
or screenshot change is needed for this correction.

## Work orders

- [x] [Task 01: Preserve Linux notification operands](task-01-linux-notification-operands.md)

## Verification results

Design validation: `list-docs.py validate` passed (404 decisions, 1748 specs),
`lint-spec-files.test.py` passed (36 tests), `lint-spec-files.py --all` passed,
the new design appears in the Platform catalog, and local Markdown links resolve.
The repository documentation coverage preflight reported `covered` for the
four artifacts plus the planned production path. `git diff --check` passed;
the production provider and existing provider test remain unchanged. The first
preflight attempt could not find `node` on PATH; the installed Node 24.21.0
binary completed it successfully.

Implementation verification passed: focused regression, provider package,
focused race test, and scoped changed-Go lint. The test-only RED showed three
expected option-like payload failures with four passing controls; GREEN passed
all seven cases. Exact commands and platform limits are in the work order.

## Risks

- A fake must observe actual process arguments and distinguish options from
  operands, so a passing test cannot merely mirror an argument builder.
- Environment and subprocess fixtures must not invoke a real notifier or leave
  a blocked child; context cancellation must be bounded and joined.
- The transport boundary cannot establish successful desktop display or daemon
  availability. The supplied real-parser qualification is separate evidence.
