---
created: 2026-09-25
status: done
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
legacy_specs: []
---

# Implementation Plan: PR #3886 permission remediation

## Overview

Correct five merge blockers and two important follow-ups found in PR #3886.
First make the mode overlay safe and opt-in; then install it at each agent-visible
executor path; then fix live mode confirmation, approval history, and phone
presentation. Finish with a cross-executor and authentication acceptance matrix.
This package supplements the original
[agent-permission-control-integrity plan](../agent-permission-control-integrity/plan.md)
and keeps its completed work orders intact.

## Scope

### In scope

- A session-owned start-mode overlay that reads selected settings only.
- Correct Docker, SSH, and Kubernetes installation and delivery reporting.
- Safe handling of malformed or non-object source settings.
- Mode-report timing and concurrent request attribution.
- Durable selected-option audit data for automatic approvals.
- A touch-accessible mismatch explanation with the same mode choices.
- Evidence for first-turn mode behavior and authentication preservation.

### Out of scope

- Changing provider permission classification or adding a command allowlist.
- Automatically copying host agent homes or credentials.
- Changing portable bundle selection or warm-resume policy.
- Addressing unrelated review threads about profile validation or launch timeouts.

## Technical approach

### Overlay and authentication boundary

Follow [the session-mode configuration ADR](../../decisions/2026-09-25-session-mode-configuration-boundary.md).
Refactor `initial_mode.go` and `initialmode/materialize.go` so the resolved
overlay receives selected settings as an explicit input. It must not call the
backend process's home-directory resolver to import unselected settings.
Validate the source root before nested assignment, write a private temporary
file and rename it, and return a typed delivery outcome. Preserve an explicit
agent configuration directory and authentication source, or mark the start mode
unavailable. Agent-specific delivery remains behind `agents.InitialModeDelivery`.

### Executor installation

`manager_launch.go` computes the mode, but executor preparation owns its final
installation. Docker's `seedSessionDir` must finish selected bundle seeding
before the overlay is merged. SSH upload and Kubernetes pod materialization
must put the overlay in the same session home that their launch environment
names. Record the actual agent-visible path, not a backend host path. A transfer
failure cannot return `Delivered: true`. Keep warm resumes from reimporting
host settings. Verify that selected bundle values other than the mode survive.

### Live mode and audit data

In the ACP adapter, capture a per-session observation sequence before
`session/set_mode`, serialize requests for that session, and settle using the
first relevant report even if it arrives before the RPC response. Keep an
unconfirmed result distinct from a confirmed clamp. For auto-approval, extend
the agentctl event payload and task permission message data with option ID,
kind, and source. Persist them before status completion; avoid silent loss of
the decision event.

### UI

Share mode state and selection logic in `mode-selector.tsx`. Fine-pointer
desktop retains the dropdown and focus disclosure. Phone/coarse-pointer uses
the shipped `MobilePickerSheet` pattern; its warning row names requested and
effective modes above the options. The sheet has one internal scroll region,
safe-area clearance, 44px touch rows, and focus return. New copy uses i18n.

## ASCII UI preview

`UI-01` is the task composer mode selector after an agent reports a different
mode (`AC-...-002.2`, `002.3`, `002.18`). Structure and the two mode names are
required; wording and spacing are illustrative. The phone entry point follows
the shipped task header picker pattern.

Desktop, mismatch state:

```text
[! Default  v]  <- focus/hover: Requested Bypass; effective Default
  Available modes
  [x] Default
      Bypass
```

Phone, mismatch state:

```text
[! Default  v]  <- visible touch target
       bottom picker
  Permission mode                    [Close]
  Requested: Bypass
  Effective: Default
  -----------------------------
  [x] Default
      Bypass
  (safe-area padding)
```

When modes match, the warning block and icon disappear. Dismissing the picker
returns focus to the selector. The picker owns scrolling; the page does not
gain horizontal overflow.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `002.12`, `002.15`, `002.17` | `initialmode/materialize_test.go`, `initial_mode_test.go`: no unselected host read, null/scalar/array handling, auth/config-dir matrix. |
| `002.13`, `002.14` | Docker, SSH, Kubernetes executor tests assert final file contents and agent-visible path after all transfers. |
| `002.16`, `002.1`–`002.3` | ACP adapter tests with deterministic barriers before/after RPC response, clamp, timeout, and concurrent calls. |
| `003.4`, `003.9`, `003.11` | Agentctl event and orchestrator/task message tests assert durable option ID, kind, source after replay and event failure. |
| `002.18` | Mode-selector component test and phone Playwright test assert warning access, choices, and focus return. |

## E2E tests

- Extend the original plan's permission E2E scenario for `002.7`, `002.10`,
  `005.1`–`005.4`: same workspace/executor and Git command under default and
  unattended profiles. A reported mode alone does not pass.
- Add `apps/web/e2e/tests/task/mobile-session-mode-mismatch.spec.ts` on
  `mobile-chrome` for `002.18`: open picker, read both modes, change a mode,
  close, and verify focus and viewport containment.
- Desktop mode selector coverage checks the same mismatch information and
  choice list. Implementation must build the current web/backend artifacts
  before Playwright, as required by the E2E runner.

## Work orders

- [x] [Task 01: Make the mode overlay safe](task-01-safe-mode-overlay.md)
- [x] [Task 02: Install modes in every executor](task-02-executor-mode-delivery.md)
- [x] [Task 03: Confirm ACP mode reports correctly](task-03-acp-mode-confirmation.md)
- [x] [Task 04: Persist auto-approval decisions](task-04-auto-approval-audit.md)
- [x] [Task 05: Show mode mismatch on phones](task-05-mobile-mode-warning.md)
- [x] [Task 06: Prove end-to-end permission behavior](task-06-permission-acceptance.md)

## Verification results

Tasks 01–06 are implemented. The targeted lifecycle/executor regression tests,
ACP confirmation tests, automatic-approval audit tests, backend integration
replay check, mobile component and browser tests, ESLint, web typecheck, i18n
check, and attended/unattended Git E2E all passed. The executor matrix records successful
Docker, SSH, and Kubernetes file delivery plus truthful unavailable outcomes
for host authentication, explicit config-directory, and path-mismatch cases.

The Git E2E uses the mock agent; a real Claude CLI first-turn enforcement smoke
check remains outside this package's evidence. No commit or push was created.

## Risks

- Some host Claude authentication paths may reject a changed configuration
  directory. The implementation must expose unavailable delivery until a
  provider channel is verified; it must not claim first-turn enforcement.
- SSH and Kubernetes session homes differ from Docker's fixed mount path.
  Tests must inspect each final process environment and filesystem together.
- ACP notifications have no request ID. Serialization and observation
  sequencing prevent local misattribution, but a provider that never reports
  remains unconfirmed.
- Audit event delivery crosses processes. A message-status update alone cannot
  establish the selected option after replay.
