---
id: "01-preserve-comment-editor"
title: "Preserve the mounted comment editor"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-001.9
  - AC-TASKS-PLAN-COMMENTS-001.10
  - AC-TASKS-PLAN-COMMENTS-001.11
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 01: Preserve the mounted comment editor

## Summary

Keep loaded Plan content mounted during refresh. Prove new comment text and
unsaved edits survive browser foreground return on desktop and phone.

## In scope

Narrow the panel loading branch, add failing-first component regressions and
real loader/foreground evidence, and add desktop/phone browser regressions.

## Out of scope

Backend/storage changes, refresh suppression, layout changes, recovery after
reload/dismissal/task navigation, and unrelated comments.

## Acceptance

1. Pending, successful, and failed same-plan refresh preserves the same input
   node, text, selected text, and add/edit mode on desktop and phone.
2. Foreground return still requests authoritative state, never saves or sends
   draft text, and explicit Add/Update receives the preserved value.
3. Initial loading, task change, and confirmed plan deletion/replacement never
   expose or act on the outgoing draft. Existing mutation guards still pass.

## ASCII UI preview

Excerpt of [UI-01 and UI-02](plan.md#ascii-ui-preview), AC-001.9 through .11:

```text
Desktop Popover:                Phone bottom Drawer:
  "Selected text"              | Comment                  |
  [Unfinished feedback...]     | "Selected text"          |
                 [Add] [Run]   | [Unfinished feedback...] |
                              |              [Add] [Run] |
```

Refresh leaves these surfaces mounted. Editing retains Update/Delete and
existing inline failure feedback. Keep phone dynamic height, one internal
scroll owner, safe-area clearance, and 44 px actions. No geometry changes.

## Verification

Run from repository root. Install dependencies once in this fresh worktree.
Run new component tests first against unchanged production code and record
expected failure (input removed or body reset), then implement and run:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- components/task/task-plan-panel.refresh.test.tsx components/task/task-plan-panel.session-switch.test.tsx components/task/plan-selection-popover.test.tsx hooks/domains/comments/use-plan-comments.test.tsx hooks/domains/comments/plan-comment-loading.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/task-plan-panel.tsx components/task/task-plan-panel.refresh.test.tsx hooks/domains/comments/use-plan-comments.test.tsx hooks/domains/comments/plan-comment-loading.test.ts e2e/tests/session/task-plan-comments.spec.ts e2e/tests/session/mobile-task-plan-comments.spec.ts e2e/tests/session/plan-comment-foreground-helpers.ts)
# Initial managed RED run builds backend and fixture plugin. If either source
# changed or those artifacts are absent, run the managed commands without --no-build.
(cd apps/web && pnpm run build:e2e)
(cd apps/web && E2E_PORT_OFFSET=17 pnpm e2e:run --no-build --project chromium tests/session/task-plan-comments.spec.ts --retries=0)
(cd apps/web && E2E_PORT_OFFSET=17 pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-task-plan-comments.spec.ts --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The initial managed RED run builds all backend binaries and the fixture plugin.
After the frontend-only fix, explicitly rebuild `build:e2e` before reusing those
unchanged backend artifacts with `--no-build`; never use stale assets. Check
that offset 17's backend/agentctl range is free before running, or choose and
record another free offset. Run desktop and phone sequentially against
test-base isolated data. Use causal transport
waits rather than arbitrary sleeps. Inspect the rendered phone Drawer and
desktop Popover after return. Record any headless limitation and the manual
Alt-Tab result separately. Add no automatic focus stealing.

## Files likely touched

- `apps/web/components/task/task-plan-panel.tsx`
- `apps/web/components/task/task-plan-panel.refresh.test.tsx` (new)
- `apps/web/hooks/domains/comments/use-plan-comments.test.tsx`
- `apps/web/hooks/domains/comments/plan-comment-loading.test.ts`
- `apps/web/e2e/tests/session/task-plan-comments.spec.ts`
- `apps/web/e2e/tests/session/mobile-task-plan-comments.spec.ts`

## Dependencies

None. Current backend and loader behavior supply the refresh path.

## Risks

Keep shared loading semantics and owner isolation. Mocking the Popover/Drawer
would hide the regression; use real inputs and assert mounted identity.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-comments.md), AC-001.9 to .11.
- [Design](../../specs/tasks/system-design/plan-comments.md#open-comment-editor-during-background-reads).
- Existing `task-plan-panel.session-switch.test.tsx`,
  `plan-selection-popover.test.tsx`, and plan-comment E2E suites.
- [Historical recovery package](../plan-comment-recovery/plan.md).

## Results

Implemented on 2026-09-13.

RED: `pnpm test -- components/task/task-plan-panel.refresh.test.tsx` ran the
real panel, Popover/Drawer, Zustand store, `usePlanComments`, foreground hook,
and plan loader with deferred API responses. Eight new/edit desktop/phone
success/failure cases failed because the input became absent during loading;
four initial-load and identity-change controls passed. The regression also
asserts explicit Add/Update receives the preserved body after settlement.

The foreground integration coverage lives in the new panel test because it
exercises the real hook/loader and input together. Existing hook and loader
test files need no changes; their suites remain in the verification command.
The E2E suites share the new `plan-comment-foreground-helpers.ts` helper; include
it in changed-file lint. Browser checks are complete.

Final verification:

- The five-file `pnpm test -- ...` command above passes 48 tests, including
  all 12 new panel regressions (eight previously failing preservation cases
  and four identity/loading controls).
- `pnpm run typecheck` and the full listed zero-warning ESLint command pass.
  The corrected E2E helper also passes a final targeted ESLint/typecheck run.
- The initial managed Chromium run built backend binaries, web assets, and the
  fixture plugin. Its new-comment foreground regression failed because the
  textarea disappeared during the held refresh, confirming browser RED.
- After the one-condition production fix, `pnpm run build:e2e` passes and
  produces fresh web assets. The backend source and fixture plugin are unchanged.
- With `E2E_PORT_OFFSET=17`, the exact `--no-build` Chromium command above
  passes all 8 scenarios in 2.1 minutes, and the phone command passes all 8
  in 1.3 minutes. Both use one worker, strict WS accounting, and zero retries.
- `python3 scripts/list-docs.py validate` passes (267 decisions and 868 specs),
  `python3 scripts/lint-spec-files.py --all` passes, and `git diff --check` passes.
- Desktop Popover and Pixel 5 Drawer screenshots were inspected against UI-01
  and UI-02: entered text, selected text, and explicit actions remain visible.
  The phone flow also asserts no document horizontal overflow.

An initial full desktop run encountered a fixture workspace PATCH 404 before
UI setup; its cause was not established. The full suite subsequently passed
on an explicitly free port range. The new edit scenario initially clicked a
zero-size badge wrapper; it now clicks the visible marked plan text. The stale
run was interrupted before rerunning all scenarios. No production changes were
made for either test setup issue.

Desktop coverage changes browser tabs with `bringToFront` and also dispatches
focus because headless browsers may not emit the OS event. Phone coverage
uses the real foreground handler and Drawer. A manual OS Alt-Tab check was not
performed in this headless environment. The managed fixtures cleaned up their
owned runtime data. Screenshots and build logs are ignored or under `/tmp`.
No public-doc changes are needed: this repairs the existing comment interaction
without changing navigation, copy, APIs, or persistence.

### Review remediation: pending plan edits

Codex identified that keeping the plan mounted also allowed content edits while
an incoming revision could replace them. `TaskPlanPanel` now passes loading as
Tiptap read-only state. The editor remains mounted, suppresses formatting,
slash, and drag controls during the read, and resumes editing on success or
failure. The separate comment input stays editable on desktop and phone.

The browser RED assertion expected `contenteditable=false` during the held read
and received `true` on the previous implementation. After the fix, both full
comment suites pass (8 Chromium and 8 mobile-chrome, zero retries), using
`E2E_PORT_OFFSET=17`, a fresh `pnpm run build:e2e`, and unchanged backend/plugin
binaries from the earlier managed build. The shared helper now asserts both
read-only and restored editing, plus comment editability while pending.

The original five focused test files still pass (48 tests). Running
`pnpm test -- components/editors/tiptap/tiptap-plan-editor.test.tsx components/editors/tiptap/plan-bubble-menu.test.tsx`
adds 21 passing tests, including a real Tiptap read-only transition proving
stable editor/input identity and no content publication. TypeScript,
changed-file ESLint, documentation catalog, and specification checks pass.
Desktop/phone screenshots were recaptured after the production change.

PR CI and reviewer completion remain pending; local checks do not establish
remote CI success.

### Aggregate review follow-up

CodeRabbit and Claude requested explicit create-path selection checks and a
clearer historical verification record. The refresh tests now assert
`selectedText`, `anchorFrom`, and `anchorTo` after each desktop/phone successful
or failed read. All 12 cases pass; this strengthens coverage of existing
behavior without a production change. The plan now labels the design-only
validation as pre-implementation and records the subsequent code/test changes.

### CI remediation: deterministic last-card fixture

E2E run 34773291709, shard 14, failed the dense/sparse swimlane last-card check
on every CI attempt. The exact test also failed locally with retries disabled.
Its concurrent task-seeding batch did not guarantee that the highest numbered
title received the final server-assigned arrival position. The UI orders by
position, so scrolling to the end did not guarantee that particular card was
visible. This expectation was introduced with the task-reordering change.

The fixture now seeds all but one task concurrently and creates the intended
tail afterward, retaining 440 cards and every virtualization, viewport, sizing,
and navigation assertion. This is a test-only CI correction; no Kanban runtime
behavior or documented contract changes. The full six-case swimlane suite passes
with zero retries using the same fresh web assets and unchanged backend build.

The focused case also passes four consecutive runs with `--repeat-each=4` and
`--retries=0` (33.1 seconds), following the full suite's six passes (25.3
seconds). Commands use `E2E_PORT_OFFSET=17 pnpm e2e:run --no-build --project chromium tests/kanban/swimlane-height.spec.ts`;
the repeated run additionally selects `--grep 'dense and sparse workflows size independently'`.
Changed-file ESLint, Prettier, catalog/spec validation, and whitespace checks
pass. Final CI/review state is tracked in the task's external plan.
