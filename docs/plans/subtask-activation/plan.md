---
created: 2026-11-03
status: implemented
requirements:
  - REQ-TASKS-SUBTASK-ACTIVATION-001
system_design:
  - ../../specs/tasks/system-design/subtask-activation.md
legacy_specs: []
---

# Implementation plan: Subtask activation

## Overview

Correct [issue #5025](https://github.com/kdlbs/kandev/issues/5025) through one
vertical work order. Use real SPA navigation after creation and keep task-page
session selection within the rendered task. Add hook, mounted-page, and desktop
and phone E2E regressions in the same work order.

The implementation is complete and verified; the results below record the
delivered behavior. The design package is included in this PR for review.

## Evidence and root cause

Investigation used repository commit `8cd31a0896a` on 2026-11-03. The canonical
issue has no image attachments. It records successful child creation and a
backend status request with the parent's task ID and the child's session ID.

`createSubtask` selects the child's task/session and calls `replaceTaskUrl`.
That helper calls only `history.replaceState`. The SPA router subscribes to
`popstate` and `kandev:navigation`, neither of which that call emits.
`useTaskDetails` therefore retains `initialTask.id`, while the active-session
validation checks the global child task. `TaskPageContentLive` combines those
identities for `useSessionResumption`.

A temporary Vitest reproduction used the real `replaceTaskUrl` and `useParams`:

```tsx
window.history.replaceState({}, "", "/t/parent-task");
const { result } = renderHook(() => useParams());
act(() => replaceTaskUrl("child-task"));
expect(window.location.pathname).toBe("/t/child-task"); // passes
expect(result.current.taskId).toBe("child-task"); // receives parent-task
```

Command: `(cd apps/web && pnpm exec vitest run lib/routing/issue-5025-repro.test.ts)`.
Result: one expected failure and one passing control. The control uses the real
`useRouter().replace` and receives the child route and session query correctly.
The temporary file was removed after evidence capture. No production or
permanent test files changed during investigation.

Existing coverage misses this boundary: `use-subtask-submit.test.ts` mocks the
URL helper. The desktop E2E accepts any `/t/` URL, including the parent route.
The existing phone subtask-menu test cancels before submission.

## Requirements and assumptions

Tasks owns the created child's activation, not a new reusable router contract.
The selected-parent defaults requirement excludes activation. UI navigation
criterion `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7` supplies an adjacent
ownership constraint but does not specify successful subtask creation.
The missing creation outcome has one new vertical requirement/design pair.

The issue settles the expected child destination and forbids mixed requests.
Replacement navigation preserves current history semantics. A response without
a session preserves the current no-activation behavior. No material question
blocks this package. The design reuses existing boundaries and needs no new ADR.

## Scope

### In scope

- Creation handoff through the existing guarded SPA router.
- Rendered-task ownership for active, route, and fallback session candidates.
- Guard cancellation, delayed hydration, failure, and no-session coverage.
- Exact-child desktop and phone creation/navigation regressions.

### Out of scope

- Backend changes, schema changes, creation retries, or agent lifecycle changes.
- Global changes to `replaceTaskUrl` or migration of other task-switch callers.
- New copy, translations, dialog geometry, or task-layout redesign.

## Technical approach

1. Update `useSubtaskSubmit` and its private `createSubtask` helper to use
   `router.replace(linkToTask(childId, { sessionId }), { scroll: false, onNavigated })`.
   Close first and select through `setActiveSession` only after commit.
2. Resolve `task` before validating session candidates in `useTaskPageData`.
   Require known ownership by that rendered task. Keep existing candidate
   priority, route hydration, and request-generation guards.
3. Exercise the real router and store with mocked create/status transports.
   Keep existing submission contract tests and strengthen rendered E2E evidence.

Delivered as described. Candidate ownership lives in the private
`useRenderedTaskSessionId` hook beside the page
(`task-page-session-ownership.ts`); it validates the active, route, and
agent-fallback candidates in that order against `taskSessions.items` and returns
null until hydration supplies an owned row.

## ASCII UI preview

### UI-01: Creation from an open parent

Current state, supported by the reproduction:

```text
URL: /t/child
Visible task: Parent       Selected session: Child
Status request: (Parent, Child session) -> ownership error
```

Proposed desktop result after successful creation:

```text
URL: /t/child?sessionId=child-session
+-------------+----------------------+------------------+
| Task list   | Child task           | Existing panels  |
| Parent      | Child conversation   |                  |
|   Child *   | [Message composer]   |                  |
+-------------+----------------------+------------------+
New Subtask dialog: closed
Status/recovery: (Child, Child session)
```

Proposed phone result through the existing task actions menu:

```text
+----------------------------------+
| Menu     Child task               |
|----------------------------------|
| Child conversation               |
| (one conversation scroll region) |
|----------------------------------|
| Message composer                 |
| Existing bottom navigation       |
+----------------------------------+
New Subtask dialog: closed
```

During child reads, the retained parent view has its owned session or no
session. Unknown child ownership waits for hydration. A rejected create keeps
the current view and existing error feedback. A guarded navigation keeps the
current selection until commit. A no-session success closes without activation.

The structural requirements are the closed dialog, matching child identity,
and focused phone conversation. Panel placement and example labels are
illustrative. Existing translated labels, safe areas, and touch targets remain.
Criteria: `AC-TASKS-SUBTASK-ACTIVATION-001.1` through `.5`.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| `.1`, `.4` | `use-subtask-submit.navigation.test.tsx`: real route subscriptions, session query, close-before-commit, deferred/cancelled navigation |
| `.2` | `task-page-session-ownership.test.tsx`: mounted parent/child transition, foreign and unknown active/route candidates, later owned candidate, delayed hydration |
| `.3` | `use-subtask-submit.test.ts`: rejected create, missing session, primary-session fallback, double submit |
| `.1`, `.2` | Real routing plus captured session request pairs during child route reads |

## E2E tests

- Desktop: extend `apps/web/e2e/tests/task/subtask.spec.ts`, test
  `create subtask from the sidebar task context menu`, project `chromium`.
  Capture exact creation IDs and assert child identity/conversation before
  leaving the page. Cover `.1`, `.2`, `.5`.
- Phone: add `apps/web/e2e/tests/task/mobile-subtask-activation.spec.ts`, test
  `opens the created child conversation`, project `mobile-chrome`.
  Reuse the existing mobile task-actions entry. Submit by touch.
  Assert the exact child route, conversation, and absence of horizontal overflow.
  Cover `.1`, `.2`, `.5`.
- Both: observe task/session request pairs from before submit through settled
  child rendering. Require a positive child-owned status request, reject every
  foreign pair, and assert no ownership-error UI. Reload only after those checks.

## Work orders

- [x] [Task 01: Align child navigation and session ownership](task-01-align-child-navigation.md)

One sequential work order. No delegation is authorized.

## Verification results

- Diagnostic reproduction: one expected route-identity failure, one passing SPA-router control.
- `python3 scripts/list-docs.py validate`: passed, 428 decisions and 1,895 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository `validateCoverage` preflight: passed for the prospective two-file
  production change and its work order, with no reference errors.
- All relative package links resolve. Explicit whitespace inspection passed for
  all four untracked documents. `git diff --check` passed for the package paths.
- Catalog discovery includes both new Tasks specifications.
- Implementation unit/integration: 6 files / 55 tests passed for the work-order
  command; broader task-page and route regression suites: 7 files / 101 tests passed.
- `pnpm run typecheck`: passed (with a 6 GiB Node heap for the rich-output check).
- Lint: `eslint --max-warnings 0` passed for the production files, new helper,
  unit tests, E2E specs, and E2E helpers.
- Fixup (review findings): an explicit route session whose row is missing now
  stays pending instead of falling back to the task's first session, automatic
  session ensuring is disabled while the route session is unresolved or an
  owned session already exists, and the desktop Dockview layout prefers the
  global active session only when its row belongs to the rendered task. Added
  route-pending, same-task active-precedence, ensure-gating, and Dockview
  ownership regressions.
- Desktop E2E: `subtask.spec.ts`, 16/16 passed on `chromium`; the extended
  sidebar creation case uses passive pre-reload assertions (one visible
  `session-chat` with the exact child `data-session-id`, child-only prompt
  content, no parent marker), proves via an armed main-frame document-navigation
  observer that no reload occurred before the positive child status request, and
  then repeats the exact child-session/content checks after the separate
  explicit reload. Every observed status pair is task-owned and no ownership
  error renders.
- Phone E2E: `mobile-subtask-activation.spec.ts`, 1/1 passed on `mobile-chrome`
  with the same passive exact-session/content and no-navigation evidence, an
  editable composer, reachable bottom navigation, no document horizontal
  overflow, and a later child-owned status request after the explicit reload.
- Documentation validators: `list-docs validate`, `lint-spec-files.test.py`
  (36 tests), and `lint-spec-files.py --all` passed; `git diff --check` passed.
- Issue assignment: verified `carlosflorencio` on issue #5025.

## Documentation impact

Internal requirement, design, plan, and work order only. Public task-workflow
documentation describes the existing creation entry points. This repair restores
their expected destination without adding controls, copy, configuration, or APIs.
The implementation must reassess public documentation only if that scope changes.

## Risks

- Router notification alone cannot prevent a mixed pair while the parent view
  remains mounted. The rendered-task guard and delayed-read regression are required.
- Child session rows can arrive after the creation response. Ownership validation
  must resume with hydration without guessing ownership or launching another session.
- Broad changes to URL-only navigation can affect sidebar selection and guards.
  Keep this correction in the subtask caller and its task-page consumer.
- The browser proof is limited to the two managed E2E entry points (desktop
  sidebar creation and phone task-actions creation); another creation surface
  would need its own coverage.
