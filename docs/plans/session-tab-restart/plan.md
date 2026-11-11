---
created: 2026-10-14
status: implemented
requirements:
  - REQ-TASKS-SESSION-TAB-RESTART-001
system_design:
  - ../../specs/tasks/system-design/session-tab-restart.md
legacy_specs: []
---

# Implementation plan: Session tab restart

## Overview

Add one action that stops and resumes the same session.
The user confirmed that history and workspace must remain available.
One work order covers the shared action, all menu consumers, and their tests.

Tasks owns the [requirement](../../specs/tasks/requirements/session-tab-restart.md)
because restart changes its session lifecycle. No existing task restart package
owns this exact outcome. Configuration Chat replacement is a separate contract.

## Scope

### In scope

- Shared stop-then-resume action with pending and failure feedback.
- Desktop workbench and preview context menus, plus the phone Sessions menu.
- Localized labels, focused tests, and public session documentation.

### Out of scope

- Session replacement, server restart endpoints, and runtime policy changes.
- New dialogs, confirmation steps, and unrelated menu restyling.
- Durable coordination across browser reloads or separate clients.

## Technical approach

Follow the [system design](../../specs/tasks/system-design/session-tab-restart.md).
Extend `useSessionActions` and pass the action through `SessionTabMenuActions`,
`SessionTab`, `PreviewSessionTabMenu`, and the mobile `SessionActionsMenu`.
Use a store-scoped pending entry so multiple mounted consumers cannot submit
duplicate sequences. Keep the entry until both requests settle or one fails.

The existing Stop can promote an active sibling to primary. Keep that lifecycle
behavior, as requested for the normal stop-and-resume sequence.
The existing Stop response precedes physical runtime exit. Normal Resume
performs terminal-runtime cleanup. Tests must exercise immediate sequencing
against the real mock backend, including a running agent and a non-active tab.
Do not introduce a delay to hide a lifecycle failure.

| Execution path | Intended behavior | Evidence and fallback |
| --- | --- | --- |
| Existing resumable agent and executor | Reuse Stop and normal Resume with the same identities | Unit payload checks and mock-backend E2E |
| Provider or executor cannot resume | Preserve history and surface existing recovery failure | Rejected-launch test. No blank replacement or silent settings fallback |
| Concurrent activity from another client | Existing server authorization and lifecycle guards apply | No new cross-client guarantee |

Use `/docs-maintainer` during implementation to update the action table in
`docs/public/sessions-and-review.md`. Public docs remain unchanged during design.

## ASCII UI preview

UI-01: Desktop session-tab context menu, active session.

```text
[Agent A] [Agent B]
           +--------------------+
           | Rename             |
           | Set as Primary     |
           |--------------------|
           | Stop               |
           | Restart session    | <- new, targets Agent B
           | Delete             | <- when eligible
           | ...                |
           +--------------------+
```

UI-02: Phone Sessions picker, then the selected row's actions menu.

```text
+--------------------------------+
| Sessions                       | <- fixed picker header
| Agent A                    [.] | <- visible actions button
| Agent B                    [.] |    list scrolls internally
+--------------------------------+
                tap Agent B [.] ->
+--------------------------------+
| Set as Primary                 |
| Stop                           |
| Restart session                | <- at least 44px hit area
| Delete                         | <- when eligible
| Handoff                        |
+--------------------------------+
```

UI-03: After activation, on both surfaces.

```text
Menu closes. Toast: Restarting session...
Reopened target menu: Restart session [disabled]
Stop / Resume / Delete: disabled while restart is pending
Success: Session restart requested
Failure: Session restart failed. <cause>
```

UI-01 and UI-02 map to AC-TASKS-SESSION-TAB-RESTART-001.1 and .5.
UI-03 maps to .3 and .4. Ordering, target identity, pending behavior, touch
geometry, and focus are structural requirements. Spacing and copy are
illustrative. Preserve existing neutral menus and their motion language.
The phone menu uses shared responsive geometry, safe areas, and internal scrolling.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| .1 | `session-tab-menu.test.tsx` and `mobile-sessions-section.test.tsx`: active/terminal/unknown eligibility |
| .2 | `use-session-actions.test.ts`: stop precedes launch, exact target, no prompt or deletion |
| .3 | `use-session-actions.test.ts`: stop error, launch error, timeout, missing connection and target |
| .4 | `use-session-actions.test.ts`: deferred requests, duplicate hook instances, unmount, target change, cleanup |
| .5 | Menu component tests plus desktop keyboard and phone geometry E2E |

## E2E tests

Add `tests/session/session-tab-restart.spec.ts` for the `chromium` project.
Cover workbench and preview menus, selected/non-active targets, same-session
history retention, unchanged sibling sessions, and one stop/launch pair.
Include a running agent so the test covers stop teardown followed by resume.
Use causal WebSocket response waits and session API state checks.

Add `tests/session/mobile-session-tab-restart.spec.ts` for `mobile-chrome`.
Open the Sessions picker, use the visible overflow button, and restart.
Check retained history, an unsent draft, target identity, focus, 44px hit areas, viewport
containment, and no horizontal page overflow. Include 767px and 768px boundary
checks plus a narrow fine-pointer check in desktop E2E.
These tests cover AC-TASKS-SESSION-TAB-RESTART-001.1 through .5 together with
the controlled failure tests above.

## Work orders

- [x] [Task 01: Add the session restart action](task-01-session-restart.md)
- [x] [Task 02: Guard composer submission during restart](task-02-composer-restart-guard.md)

### Composer follow-up

The user approved blocking submission while a session restarts. Task 02 connects
the existing pending state to the composer and its shared submit handler. The
draft stays editable, the pending status remains visible, and other sessions
remain usable. Normal readiness and recovery rules apply after the attempt.
Task 01 results below remain historical evidence for the earlier implementation.

Task 02 initial local verification passed: 144 targeted unit tests, 5 desktop browser
tests, and 2 phone browser tests. Its [results](task-02-composer-restart-guard.md#results)
record the guard, failure behavior, and checks. Screenshot publication and fresh
remote CI/review for the new commit remain delivery steps.

An additional two-session check reproduced automatic sibling adoption during
Stop. Task 02 then added shared pending-state guards to both automatic WebSocket
selection paths, so the selected composer's send guard stays visible.

## Verification results

Design validation on 2026-10-14:

- `python3 scripts/list-docs.py validate`: passed, 381 decisions and 1609 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for this package and
  the prospective `use-session-actions.ts` change. All references resolved.
- Catalog discovery found both new specifications. Whitespace checks passed.

Implementation completed on 2026-10-14. The [work order results](task-01-session-restart.md#results)
record the exact final commands: 93 targeted tests, 3 desktop E2E tests, and
1 phone E2E test passed. Typecheck, lint, localization, public documentation,
specification, coverage, formatting, and whitespace checks passed.

### PR remediation

The review found that Stop could select a live sibling during restart.
The action now restores the original selection after the attempt settles,
unless a later user navigation changes the task or navigation revision.
Backend primary promotion remains unchanged. Desktop and phone regressions
cover a selected target with a live sibling.

The restoration fallback also preserves selection ownership. An automatically
selected target stays unpinned and does not advance the user navigation revision.
An existing user pin is restored after either successful or failed Resume.

The [work order results](task-01-session-restart.md#pr-remediation-results)
record the final local checks. Remote CI and review remain pending until the
remediation reaches the PR and its checks complete.

## Risks

- Stop and Resume are separate requests. A disconnected browser can leave the
  session stopped. Existing Resume remains the recovery path.
- The backend owns delayed teardown and launch arbitration. A failing focused
  E2E test requires diagnosis before expanding this package's server scope.
- Mock-agent tests prove menu wiring and sequencing. They do not prove that
  every external provider can restore an expired conversation.
