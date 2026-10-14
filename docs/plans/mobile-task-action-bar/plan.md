---
created: 2026-10-12
status: in_progress
requirements:
  - REQ-UI-MOBILE-TASK-ACTION-BAR-001
  - REQ-UI-MOBILE-TASK-CHROME-001
system_design:
  - ../../specs/ui/system-design/mobile-task-action-bar.md
  - ../../specs/ui/system-design/mobile-task-chrome.md
legacy_specs: []
---

# Implementation Plan: Mobile task action bar

## Overview

Give the terminal notice its own phone row, retain complete reachable controls,
and prove the archive/workflow paths still work. Implement and validate one
vertical UI slice, then capture PR evidence and deliver through merge.

The user authorized PR creation with screenshots and merge. Repository rules
require this design-package handoff followed by a later explicit implementation
request. No delegation is authorized. Keep design files uncommitted at handoff.

## Scope

### In scope

- Responsive notice/action composition in chat and the shared passthrough host.
- Affected control touch sizing and localized terminal notices/touched labels.
- Focused component and mobile/desktop rendered regression coverage.
- Fresh screenshots, Conventional Commit, PR, required CI/review handling, merge.
- A minimal nested-workspace-menu correction needed for the existing mobile
  reconnect regression that fails identically on the untouched main baseline.

### Out of scope

- New task/PR semantics, archive confirmation design, workflow option behavior,
  navigation, persisted preferences, backend changes, or broad control audits.

## Technical approach

Keep `ChatStatusBar`, `ChatStatusBarActions`, `TranscriptNavGroup`, and the
shared terminal banner state owners. Allocate full width to the phone notice
before the action region. Audit the passthrough `PassthroughStatusRow` parent allocation.
Use natural text wrapping, nonshrinking touch targets, and complete-control
wrapping for dense utility clusters. Use `useResponsiveBreakpoint`, preserving
phone sizing on narrow fine-pointer viewports as well as coarse pointers.
Localize notice variants in `task.json` for all real locales. Reuse existing
archive hooks/phone adapters, usage drawer, share dialog, and Proceed callbacks.

## ASCII UI preview

These previews record this package's original delivery composition. The later
normal-chat correction places the phone workflow action above utility icons;
its authoritative UI-02 preview and follow-up implementation belong to the
[composer placement follow-up](../mobile-composer-action-wrapping/task-02-place-action-above-utilities.md).
This package retains notice/sizing/state scope and its prior verification
results. Passthrough composition is unaffected by that follow-up.

### UI-01: Phone task composer, merged PR

Entry: task conversation, idle session with terminal PR and eligible next step.
Before: source and supplied screenshot show the notice squeezed by action width.

```text
Before (phone)
+---------------------------------------+
| PR #3948        [nav][usage][share]    |
| has been            [Done >]         |
| merged.   Archive x                  |
| You can                             |
| archive                             |
| this task.                          |
+---------------------------------------+

After (phone)
+---------------------------------------+
| merge PR #3948 merged   [Archive] [x] |
+---------------------------------------+
| [Todos] [nav] [Usage] [Share] [Done >] |
| extra complete controls wrap here    |
+---------------------------------------+
| Message composer ...                 |
+---------------------------------------+
| Existing phone navigation / safe area|
+---------------------------------------+
```

Structure is required; icon names/spacing/copy are illustrative. The notice is
full width. Long notice text wraps in its allocated region. Controls have 44px
phone targets and may wrap as whole controls. The composer tray remains outside
the transcript scroller, with no new scroller or duplicated safe-area padding.
Criteria: 001.1, 001.2, 001.3, 001.5, 001.7, 001.8.

### UI-02: Desktop task composer, merged PR

```text
+---------------------------------------------------------------------+
| Todos | PR #3948 has been merged. ... Archive x | nav Usage Share Done >|
+---------------------------------------------------------------------+
| Message composer ...                                                |
+---------------------------------------------------------------------+
```

Retain inline/wrapping desktop composition and compact control density. The
terminal message is localized. At 768px fine pointer use desktop composition;
coarse tablets retain touch sizing. Criteria: 001.3, 001.6, 001.7.

### UI-03: Phone alternate and pending states

```text
Closed:   [closed icon] PR #3948 closed
          without merging             [Archive] [x]
No notice: [Todos] [nav] [Usage] [Share] [Done >]
Moving:    [Todos] [nav] [Usage] [Share] [Done > disabled]
Archive:   existing phone confirmation with Archive / Cancel
```

Cancel restores the unchanged task/notice; dismiss removes only the notice.
No notice means no blank row. Long translated notices expand vertically; long
step labels wrap inside their target. Criteria: 001.4, 001.6, 001.7.

### UI-04: Existing Tasks picker workspace recovery

```text
Before: Tasks sheet > workspace menu > select workspace B
        Both surfaces close; page remains pointer-blocked.
After:  Tasks sheet (modal owner) > workspace menu (non-modal)
        Select workspace B; close surfaces; task picker remains usable.
```

The visible composition and workspace navigation policy are unchanged. The
parent picker is the only modal boundary. This restores
AC-UI-MOBILE-TASK-CHROME-001.3; the existing shared-context browser scenario
checks pointer unlock and the reopened picker after both workspace switches.

## Tests

- `pr-archive-banners.test.tsx`: merged/closed and mixed/all-terminal eligibility,
  localized singular/plural text, dismiss, archive preference/confirmation (001.4,
  001.6, 001.7).
- `chat-status-bar.test.tsx` and `passthrough-toolbar.test.tsx`: responsive host
  composition and preserved handler/visibility wiring (001.1, 001.5, 001.6).
- Existing affected standalone-control suites: phone sizing condition and
  action state; new ShareButton tests if its props/logic change (001.3, 001.5).
- Use TDD for changed logic and browser layout regressions; observe relevant
  failures before implementation. Do not add class-string mirror tests.

## E2E tests

- New `e2e/tests/chat/mobile-chat-status-bar.spec.ts`, `mobile-chrome`: terminal
  notice with all common actions; merged/closed/plural/mixed/absent cases,
  dismiss/reload, archive cancel/confirm, usage/share actions, long labels,
  viewport containment/target geometry, breakpoint/pointer checks (001.1-001.8).
- Existing `chat/chat-status-bar.spec.ts`, `chromium`: update changed localized
  copy assertions; retain all archive/dismiss/indicator scenarios.
- Existing `cli-mode/mobile-passthrough-composer.spec.ts`: add a merged PR
  notice containment and composer-toggle smoke case (001.1-001.3, 001.8).
- Existing `workflow/mobile-workflow-step-proceed.spec.ts`: retain workflow
  ownership outcome and measure reachable Proceed (001.3, 001.5).

## Work orders

- [x] [Task 01: Repair and prove the phone action bar](task-01-phone-action-bar.md)
- [ ] [Task 02: Capture screenshots and deliver through merge](task-02-pr-delivery.md)

Task 02 depends on Task 01. Execute sequentially in the primary conversation.

## Verification results

Design validation passed on 2026-10-12:

- `python3 scripts/list-docs.py validate`: 377 decisions and 1544 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` preflight through Node 24:
  both work orders covered, no errors. Evaluated the package with the planned
  `chat-status-bar.tsx` production trigger; this is artifact evidence, not a PR check.
- Catalog discovery finds both new specs; package path/status checks pass.
- `git diff --check -- docs/specs docs/plans/mobile-task-action-bar`: passed.

Implementation and rendered checks passed: 116 unit tests, 10 mobile scenarios,
7 desktop scenarios, and 4 final capture regressions. Typecheck, scoped lint,
i18n, public-doc and specification gates passed. The PR is published with inspected screenshots. A CI test-row typing fix and
coarse-tablet wrapping correction passed 21 affected component tests, final
typecheck/scoped lint, and 4 fresh-build E2E regressions. A fifth screenshot
shows the coarse-tablet label containment. Final CI/review/merge evidence is
tracked externally in the task plan.
Exact execution commands live in Task 01; Task 02 records screenshot and GitHub
evidence. Promote specs only after implementation matches their criteria and
all required focused checks pass. The shell initially lacked Node/pnpm on PATH;
both are installed through mise. The inspected repository tool configuration is
now locally trusted; Node 24.21.0 and pnpm 9.15.9 resolve through the recorded
shim PATH bootstrap in the work orders.

## Risks

- The nested passthrough action parent can still constrain a full-width child;
  test allocation in the real host, not an isolated banner.
- Touch sizing increases dense rows; complete-control wrapping must keep both
  composer and Proceed reachable at 320px and with long translated labels.
- Some existing terminal text and optional auto-scroll labels are hardcoded;
  touched copy needs all seven locales without changing identifiers.
- GitHub merge queue/check requirements can delay final merge; never bypass a
  failing required check or discard actionable review findings.

## Public documentation

At implementation, use `/docs-maintainer` to add a concise composer-controls
paragraph in `docs/public/mobile-remote-access.md` (how-to). Explain the notice,
Archive/dismiss, and retained workflow action; preserve existing confirmation
instructions. The public how-to guide now includes the composer-controls paragraph.
