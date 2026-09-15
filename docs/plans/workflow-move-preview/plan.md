---
created: 2026-09-14
status: implemented
requirements:
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-001
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-002
system_design:
  - ../../specs/tasks/system-design/workflow-move-preview.md
legacy_specs: []
---

# Implementation Plan: Workflow Move Preview

## Overview

Add compact task-specific recipient and model feedback before a step move.
Deliver a read-only shared backend decision first, then integrate the desktop
popover and phone drawer with end-to-end move verification. The backend boundary
needs isolated side-effect and routing-parity proof before any UI depends on it.

Confirmed user choices: two summary lines, current/other/new conversation,
unchanged or before/after model, additional settings count, optional details.
The preview includes conditional Codex model changes without profile changes.

## Scope

### In scope

- Task stepper and compact step disclosure, including task preview use sites.
- Read-only move prediction using runtime overrides and entry options.
- Conditional set, keep, restore-original and no-match behavior.
- Loading, retry, uncertainty, accessibility, five locales, and phone composition.

### Out of scope

- Changes to session routing policy, profile defaults, provider acceptance,
  native subagents, workflow editing, or mandatory move confirmation.
- Board drag, bulk moves, and unrelated move menus.
- Production instance mutation or live provider probes during development.

## Technical approach

Follow the [design](../../specs/tasks/system-design/workflow-move-preview.md).
Extract shared read-only decisions from orchestrator workflow routing and
conditional configuration. Add the proposed task move-preview endpoint and
DTO, then a domain API client/hook and shared renderer. Keep execution-time
atomic checks and existing move semantics authoritative.

Nearest shipped mobile surface: `workflow-step-disclosure.tsx`.
Curated baseline: MobilePickerSheet and the mobile UI language's temporary
choice drawer. Use its fixed header, internal scroll, safe-area clearance,
focus return, and shared state logic.

No schema or persistent preview cache. The design carries the endpoint contract;
no separate architecture decision is needed for an advisory projection of the
existing lifecycle. Public documentation changes belong to implementation.

## ASCII UI preview

Structural requirements: exactly two collapsed summary rows above Move here;
optional details expand below the summary; Options and existing capability/progress
content stay available. Spacing and abbreviated data below are illustrative.
All drawn strings become localized copy or model/session data.

### UI-01: Desktop step hover or keyboard disclosure

AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.1 through .5 and -002.1, .4.

```text
Retained override           Conditional model change
+-------------------------+ +-------------------------+
| Reuse current session   | | Reuse current session   |
| Astra (overrides Luna) i| | Astra -> Luna  +2     i |
|       Move here         | |       Move here         |
|        Options          | |        Options          |
| [existing capabilities] | | [existing capabilities] |
+-------------------------+ +-------------------------+

Unchanged model             New conversation
+-------------------------+ +-------------------------+
| Reuse current session   | | New session             |
| Astra                 i | | 5.6 Luna              i |
|       Move here         | |       Move here         |
|        Options          | |        Options          |
+-------------------------+ +-------------------------+

Other conversation          Expanded details
+-------------------------+ +------------------------------+
| Reuse: Implementation   | | Reuse current session        |
| Luna                  i | | Astra -> Luna  +2       Hide |
|       Move here         | | Planned changes              |
|        Options          | | Reasoning: medium -> max     |
+-------------------------+ | Context: reset               |
                            | Checked again when moving.   |
                            |         Move here            |
                            |          Options             |
                            +------------------------------+
```

`+2` means reasoning and context reset here, excluding the already displayed
model change. A model-only change has no count. Details disclose uncertainty,
skipped rules, full labels, profile, and source retirement as applicable.

### UI-02: Phone step drawer

Entry: tap the existing current-step disclosure. The drawer keeps its fixed
header and one scroll body. AC-TASKS-WORKFLOW-MOVE-PREVIEW-002.2, .4.

```text
+--------------------------------+
| Move to                     x  |  fixed header
|--------------------------------|
| Analysis (current)              |
|--------------------------------|
| Implement                      |
| Reuse current session          |
| Astra -> Luna  +1    [Details]  |
| [Options]        [Move here]    |
|--------------------------------|
| Review                         |
| Reuse: Analysis                |
| Astra               [Details]  |
| [Options]        [Move here]    |
|________________________________|  safe-area clearance
```

Details expand in the same scroll body, immediately after the summary. All touch
actions have at least 44px hit areas. No stacked overlays or horizontal scrolling.
Same projection and move draft serve desktop and phone.

### UI-03: Loading, error, unknown

AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.5, .7 and -002.3.

```text
+-------------------------+ +-------------------------+
| Checking session...     | | Preview unavailable     |
|                         | | Retry                   |
|       Move here         | |       Move here         |
|        Options          | |        Options          |
+-------------------------+ +-------------------------+

+-------------------------+
| New session             |
| Model not known       i |
|       Move here         |
|        Options          |
+-------------------------+
```

Loading, error, or unknown alone does not disable Move here. Existing permission,
pending-move, and task gates still apply. Phone rows use these same statuses.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| 001.1, 001.2 | workflow_move_preview_test.go: PreviewMatchesMove, SameProfileOverride, ExplicitTargets, TerminalCandidates, NewSessionDefaults |
| 001.3, 001.4, 001.5 | workflow_move_preview_test.go: ConditionalSetKeepRestore, OriginalEligibility, AmbiguousRules, UnknownCapabilities, DeduplicatedChanges |
| 001.6 | workflow_move_preview_test.go: NoSideEffects; workflow_move_preview_http_test.go: AuthorizationAndRedaction |
| 001.7, 002.3 | use-workflow-move-preview.test.ts: LateResponse, DraftChanges, ReopenReconnect, Retry; deferred-move backend fixture |
| 002.1, 002.2, 002.4 | workflow-move-preview.test.tsx and workflow-stepper.test.tsx: summary/details, keyboard, counts, localization; phone E2E |

## E2E tests

New `tests/workflow/workflow-move-preview.spec.ts` (chromium) covers a retained
session model override during an in-place move, reuse of another named session,
and creation of a fresh profiled session. Each scenario inspects the preview,
executes the move, and checks the resulting session identity or model against
isolated fixture API state. Unit and integration tests cover conditional
settings, reset and skipped states, no-session launch gates, authorization,
fresh-launch configuration projection, and stale-response handling.

New `tests/workflow/mobile-workflow-move-preview.spec.ts` (mobile-chrome) covers
the coarse-pointer touch drawer, inline details, 44px controls, focus return on
dismissal, and no horizontal overflow. The scenario uses the repository's
tablet context because the full phone task route uses `SessionMobileLayout` and
does not render the desktop task stepper. Existing step-targeting and move-option
suites provide the adjacent mobile regression evidence. No production DB or task
is used.

## Work orders

- [x] [Task 01: Read-only move decisions and API](task-01-move-decision-api.md)
- [x] [Task 02: Compact preview and move verification](task-02-compact-preview.md)

Order: 01 -> 02. Both are sequential. No delegation is authorized by this plan.

## Verification results

Design checks passed on 2026-09-14:

- `python3 scripts/list-docs.py validate`: 267 decisions and 911 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- Relative-link, acceptance-reference, and whitespace checks passed for all five files.
- `git diff --check` passed; scoped git status confirmed all five new artifacts.

Implementation is complete and both work orders are complete.

- Backend package tests and the targeted race suite passed.
- Frontend focused tests passed with 44 tests across five files. Typecheck,
  full lint, localization checks, and the new-code localization ratchet passed.
- `make -C apps/backend build` and `pnpm run build:e2e` passed.
- Desktop workflow E2E passed all 12 tests. Mobile workflow E2E passed all 5
  tests after the final drawer exit-state assertion was stabilized.
- Specification, public-doc, and diff checks passed.

The mobile browser scenario uses the existing coarse-pointer tablet context to
exercise `CompactWorkflowStepDisclosure`. The full Pixel 5 task route uses the
separate `SessionMobileLayout` and does not render the desktop task stepper.
This is a test-surface detail; the shared touch drawer remains the production
mobile disclosure used by the task preview and tablet task surfaces.

### Review remediation addendum

The implementation evidence was extended after code-only review. Backend preview
recipient selection now uses the same no-session launch predicate as execution,
including the no-auto-start and skip-without-instructions gates. The preview
returns `no_session` without a recipient when the task remains idle, and keeps
the task-level profile fallback when an allowed fresh launch can occur.

The preview endpoint authorizes the task before reading task state. Existing
source sessions retain current-session routing when no destination profile is
configured, while profile changes can predict a fresh target session even when
the destination does not auto-start a prompt. Fresh launches project applicable
session settings, and context reset and mode changes report skipped states when
execution cannot apply them. Busy reusable targets report deferred dispatch,
and no-session dispatch has its own localized detail label.

Open previews now invalidate from live store revisions and connection changes,
including session model and fallback updates, clear stale success while
refreshing, and retain late-response guards. Every movable disclosure row can
receive a preview through a two-request queue. Typed diagnostics and Kandev-owned
fields and values render through the active locale; provider option labels remain
bounded display data.

Additional remediation checks passed on 2026-09-15:

- `go test ./internal/workflow/move ./internal/orchestrator`.
- `make lint` from `apps/backend`.
- Focused Vitest: 3 files, 13 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` from `apps/web`.

Final post-remediation verification passed on 2026-09-15:

- Focused Vitest passed all 50 tests across six affected files.
- `make build` from `apps/backend` and `pnpm run build:e2e` from `apps/web`.
- The focused desktop move-preview spec passed 2 tests, and the focused mobile
  move-preview spec passed 1 test.
- `go test -race ./internal/orchestrator ./internal/task/handlers -run 'Preview|WorkflowStepSession|WorkflowSessionConfig|WorkflowSessionTarget|SameProfile'`.
- Specification, public-doc, formatting, and localization checks passed after
  trimming unrelated generated catalog churn.

## Risks

- Selection helpers can write provenance or warnings; preview must isolate pure decisions.
- Live provider rejection and deferred moves cannot be guaranteed by a hover snapshot.
- Runtime configuration can differ from mutable profile rows.
- Read-only snapshots and WS updates race; stale-generation tests are required.
- Long translations can crowd the compact surface; full details stay accessible.
