---
created: 2026-09-14
status: implemented
requirements:
  - REQ-TASKS-OPEN-FOLDER-001
system_design:
  - ../../specs/tasks/system-design/open-task-folder.md
legacy_specs: []
---

# Implementation plan: Open task folder

## Overview

One sequential vertical work order adds the shortcut using an already implemented
folder endpoint. Implementation estimate: 1-2 hours including focused UI tests.

## Scope

In scope: desktop shortcut, multi-worktree selection, shared request/error handling,
phone Files action parity, localization and public usage documentation.
Out of scope: new native/remote filesystem integrations and archived-task changes.

## Technical approach

Follow [system design](../../specs/tasks/system-design/open-task-folder.md).
Reuse `Service.OpenFolder` and its existing `worktree_id` request contract.
Keep the session API options argument compatible with current callers.

## ASCII UI preview

### UI-01: Desktop task tools, selected session

```text
[Layout] [Open in IDE | v] [Folder]
                           |
                           + Choose folder dialog (only when multiple)
                             repo-a  branch-a
                             repo-b  branch-b
```

### UI-02: Phone Files panel, workspace menu expanded

```text
[Files                         ...]
  +------------------------------+
  | Add repositories             |
  | Open workspace folder        |
  +------------------------------+
          -> [Choose a folder] (multiple only)
             [repo-a / branch-a]
             [repo-b / branch-b]
```

Structural requirements: desktop adjacency and independent enablement; phone visible
menu entry with 44px targets and one internal scroll owner. Spacing/icons are
illustrative. No session: disabled shortcut. Opening: spinner and disabled action.
Failure: localized toast, control becomes retryable. AC-001.1 through AC-001.4
refer to the full AC-TASKS-OPEN-FOLDER-001 identifiers in the requirement.

## Tests

`hooks/use-open-session-folder.test.ts`: no session, selected worktree payload,
legacy no-payload invocation, failed request and retry (AC-001.2, .3, .5).
`components/task/open-task-folder-button.test.tsx`: editor independence, disabled
and busy states, single-worktree opening, multi-worktree selection, changed session
and picker dismissal (AC-001.1 through .3).
Existing `editor-worktree-options.test.ts` and Go editor service tests cover labels
and server-side session/worktree path selection.

## E2E tests

New `e2e/tests/task/open-task-folder.spec.ts` on the configured desktop project:
assert adjacency, click-to-request, selection payload, and failure recovery.
New `e2e/tests/task/mobile-open-task-folder.spec.ts` on `mobile-chrome`:
Files menu, repository picker, correct request, touch geometry and no overflow.
Together these cover AC-001.1 through .5 at the browser boundary; native Finder
visibility requires the separate macOS smoke check.

## Work orders

- [x] [Task 01: Add folder shortcut](task-01-folder-shortcut.md)

## Verification commands

```bash
(cd apps/web && pnpm exec vitest run hooks/use-open-session-folder.test.ts lib/api/domains/session-api.test.ts components/task/open-task-folder-button.test.tsx components/task/editor-worktree-options.test.ts components/task/file-browser-toolbar.test.tsx components/task/file-browser-responsive.test.tsx components/task/task-top-bar.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/task/open-task-folder.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-open-task-folder.spec.ts tests/task/mobile-add-workspace-sources.spec.ts)
(cd apps/backend && go test ./internal/editors/...)
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
```

## Verification results

Implemented REQ-TASKS-OPEN-FOLDER-001 and all five acceptance criteria.

- RED: request tests demonstrated dropped worktree IDs, unhandled failures and duplicate launches; the new button tests demonstrated the absent action.
- Focused frontend verification: seven suites passed 49 tests; after the menu-row extraction and cancellation coverage, the two affected suites passed 15 tests. There are 50 distinct passing focused tests. Final typecheck, targeted ESLint and i18n checks passed.
- Existing Go editor packages passed using `GOCACHE=/tmp/kandev-open-folder-go-cache` (the default cache is read-only in this environment).
- Full backend and production Vite E2E builds passed. Local sockets required sandbox escalation. Desktop folder tests passed 2/2; the final phone folder test passed 1/1 after waiting for menu/picker animation completion before geometry checks.
- Public-doc validators, specification catalog/lint and whitespace checks passed. Public documentation: `docs/public/developer-tools.md`.

### Verification exception

The existing `mobile-add-workspace-sources.spec.ts` times out while measuring the
separate Add repository menu, after its folder-opening step succeeds. The same
failure reproduces against an isolated unchanged frontend exported from HEAD
`509a1bb9b` (with only the native-opening HTTP call stubbed in the test to avoid a
host GUI launch). This is a baseline source-menu failure, not a folder regression;
no unrelated production code or source-menu assertions were changed.

### Native and visual evidence

Desktop screenshots were inspected: the folder icon is immediately beside the IDE
control and the repository picker matches the planned hierarchy. The final phone
picker screenshot was inspected: both repository rows are contained,
44px touch targets are available, the inset drawer clears the safe area, and there
is no page overflow. Desktop screenshots are retained under
`/tmp/open-folder-desktop-verified/`; phone evidence is under
`/tmp/open-folder-mobile-verified/`. Baseline failure evidence is in
`/tmp/open-folder-e2e-baseline.log` and `/tmp/open-folder-baseline-results/`.
The screenshot-retention desktop rerun reported both tests passing, but its parent
wrapper exited 143; the earlier desktop run exited 0 with both tests passing.
Native Finder visibility cannot be checked on this Linux host; the existing backend
macOS command path is unchanged. The initial implementation was left uncommitted. The user requested the availability correction and a branch push on 2026-09-17.

## Risks

Native launch success does not guarantee a visible window on a headless host.
Remote browser clients operate on the Kandev host, not their own filesystem.
Public usage documentation is updated in `docs/public/developer-tools.md`. No ADR is needed for reusing the existing contract.

### Availability correction validation (2026-09-17)

- Backend editor packages pass, including missing/installed executable detection and unavailable-request rejection.
- Eight focused frontend suites pass (65 tests); final affected hook/button suites pass (15 tests).
- Desktop browser suite passes 3/3 and phone suite passes 2/2, covering both available and unavailable host commands. UI tests stub capability/native opening; backend tests exercise executable discovery.
- Typecheck, changed-file ESLint, i18n, specification validators, backend build, Vite build and whitespace checks pass. The stale E2E plugin fixture was rebuilt before browser verification.
- Native Finder verification remains for the user's macOS machine. No main instance was modified.

## PR review remediation

Merge the current main toolbar grouping/panel toggle and locale additions while
retaining the folder shortcut. Preserve host capability in editor boot state and
refetch it when already-loaded editor items lack capability information. This
covers settings-first navigation without enabling unavailable openers. Regression
coverage includes boot state with installed/missing commands, loaded-state
fallback discovery, known true/false boot capabilities, and failed discovery.

Local remediation validation passes: backendapp and editor Go suites, 43 focused
frontend tests plus the five new hydration/retry regressions, typecheck, targeted lint,
i18n, harness/spec checks, backend and web builds, and all five folder browser
tests (three desktop, two phone). The adjacent source-attachment browser test
still times out measuring its repository menu, matching the previously recorded
baseline failure. Current-head remote CI and review completion remain pending
until this remediation is pushed. Native Finder verification remains external.

CodeRabbit summary suggestions are addressed with precise path-fallback and
capability documentation, plus one delayed retry for transient discovery failure.
Repeated failures remain disabled without an uncontrolled retry loop.

Follow-up Codex concurrency findings: discovery claims the live store loading
state before fetching, and all folder controls share pending state per session.
Deferred-response regressions cover simultaneous discovery consumers, shared
folder-control disabling, duplicate suppression, release, and session independence.

CI remediation: the desktop source-attachment suite also assumes an installed
folder opener. It now explicitly stubs capability and native opening, matching
the phone fixture and preserving real backend discovery tests. Both previously
failing browser cases pass locally (2/2); targeted lint and typecheck pass.

## Follow-up relocation (2026-09-22)

The [editor dropdown package](../editor-dropdown-folder/plan.md) supersedes this
package's standalone desktop button placement. This completed record and its test
results describe PR #3784; they are not validation of the pending relocation.
