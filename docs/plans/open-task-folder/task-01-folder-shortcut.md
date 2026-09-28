---
id: "01-folder-shortcut"
title: "Add task folder shortcut"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-OPEN-FOLDER-001
acceptance_criteria:
  - AC-TASKS-OPEN-FOLDER-001.1
  - AC-TASKS-OPEN-FOLDER-001.2
  - AC-TASKS-OPEN-FOLDER-001.3
  - AC-TASKS-OPEN-FOLDER-001.4
  - AC-TASKS-OPEN-FOLDER-001.5
system_design:
  - ../../specs/tasks/system-design/open-task-folder.md
---

# Task 01: Add task folder shortcut

## Summary

Expose native folder opening beside the IDE action, with explicit worktree selection
and the corresponding phone Files-menu flow. Reuse existing backend behavior.

## In scope

Shortcut, picker, compatible client payload, shared error handling, localization,
focused tests and public usage documentation.

## Out of scope

New OS integration, remote mounting, editor preferences and archived-task tool changes.

## Acceptance

- Desktop shortcut opens the intended session folder independently of editor settings.
- Desktop and phone select the intended worktree and recover visibly from request failures.
- Targeted checks pass, with native macOS smoke evidence or its explicit environment limitation.

## ASCII UI preview

See [combined preview](plan.md#ascii-ui-preview).

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

## Verification

Use repository TDD: add failing behavior tests before production edits. Install
workspace dependencies from `apps/` if this worktree lacks them. Run:

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

On macOS: open a single-repository task, activate Folder, verify Finder shows the
worktree; repeat selecting the second repository of a multi-repository task.

## Files likely touched

- `apps/web/components/task/task-top-bar.tsx`, new `open-task-folder-button.tsx` and test.
- `apps/web/hooks/use-open-session-folder.ts` and new test; `apps/web/lib/api/domains/session-api.ts`.
- `apps/web/components/task/file-browser.tsx`, `file-browser-data.ts`, and a shared folder picker.
- `apps/web/e2e/tests/task/open-task-folder.spec.ts`, `mobile-open-task-folder.spec.ts`.
- `apps/web/src/locales/` as needed and `docs/public/developer-tools.md`.

## Dependencies

None. Existing folder endpoint and mobile Files workspace menu are already shipped.

## Risks

Preserve client options compatibility, prevent stale-session selection, and catch
request failures for every existing hook caller. Do not use native GUI launching in CI.

## Parallelism

`sequential`

## Inputs

[Requirements](../../specs/tasks/requirements/open-task-folder.md),
[design](../../specs/tasks/system-design/open-task-folder.md), existing editor button,
`useOpenSessionInEditor`, and `mobile-add-workspace-sources.spec.ts`.

## Results

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

## Host opener availability correction

User testing on 2026-09-17 identified that missing host commands must disable the
folder action before a repository picker appears. The shared editor discovery
response now reports executable availability; desktop, mobile, and file editor
menus use a fail-closed capability with request and backend guards. The original
UI composition and touch sizing remain unchanged. RED: toolbar regression failed
because the missing-opener button was enabled; backend detection test failed to
compile before its implementation. Added installed/missing/unsupported command,
request suppression, store retention, and desktop/phone unavailable-action checks.

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
