---
id: "01-session-restart"
title: "Add the session restart action"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-TAB-RESTART-001
acceptance_criteria:
  - AC-TASKS-SESSION-TAB-RESTART-001.1
  - AC-TASKS-SESSION-TAB-RESTART-001.2
  - AC-TASKS-SESSION-TAB-RESTART-001.3
  - AC-TASKS-SESSION-TAB-RESTART-001.4
  - AC-TASKS-SESSION-TAB-RESTART-001.5
system_design:
  - ../../specs/tasks/system-design/session-tab-restart.md
---

# Task 01: Add the session restart action

## Summary

Implement the shared action and expose it in desktop and phone session menus.
Use TDD for sequencing, failure, and pending state. Run focused browser tests
against the managed E2E backend.

## In scope

- Hook action, store-scoped duplicate guard, and one localized feedback sequence.
- Workbench, preview, and mobile menus with common eligibility and pending state.
- Locale catalogs, component tests, E2E tests, and public session documentation.

## Out of scope

- New backend APIs, runtime policy changes, blank sessions, and host restarts.
- Confirmation dialogs, automatic retries, and cross-client restart transactions.

## Acceptance

1. The shared action targets one captured session and dispatches Resume only
   after Stop succeeds. Failures retain data and produce truthful feedback.
2. All three menu consumers expose the action with shared pending protection.
   History, workspace, selection, and unsent input remain intact.
   Primary promotion follows the existing Stop lifecycle.
3. Unit, rendered component, desktop E2E, mobile E2E, and localization checks pass.

## ASCII UI preview

Use the [full preview](plan.md#ascii-ui-preview) and its state annotations.

```text
UI-01 Desktop tab -> right click
  Stop
  Restart session
  Delete (when eligible)

UI-02 Phone -> Sessions -> row actions
  Stop
  Restart session       [44px minimum hit area]
  Delete (when eligible)

UI-03 Pending -> one Restarting session... toast
  Restart session       [disabled]
  Stop / Resume / Delete disabled for this target
```

These views cover AC-TASKS-SESSION-TAB-RESTART-001.1, .3, .4, and .5.
Compare rendered menus with the order and geometry in these previews.

## Verification

Run from the repository root. In a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-actions.test.ts components/task/session-tab-menu.test.tsx components/task/preview-session-tabs.test.tsx components/task/mobile/mobile-sessions-section.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/session/session-action-state.ts hooks/domains/session/use-session-restart.ts hooks/domains/session/use-session-actions.ts components/task/session-tab-menu.tsx components/task/session-tab.tsx components/task/preview-session-tab-menu.tsx components/task/mobile/mobile-sessions-section.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-tab-restart.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-tab-restart.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The session-action tests cover the restart hook and shared state predicates. The managed E2E runner
builds current assets. Run desktop and mobile commands sequentially.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-actions.ts` and its test.
- `apps/web/hooks/domains/session/use-session-restart.ts` and `session-action-state.ts`.
- `apps/web/components/task/session-tab-menu.tsx`, `session-tab.tsx`,
  `preview-session-tab-menu.tsx`, and their affected tests.
- `apps/web/components/task/mobile/mobile-sessions-section.tsx` and its test.
- `apps/web/src/locales/*/task.json`, the two new E2E files named above, and
  `docs/public/sessions-and-review.md`.

## Dependencies

None. Existing Stop and Resume backend contracts are reused.

## Risks

The Stop acknowledgment does not prove runtime exit. Preserve the existing
Resume cleanup path. Do not add delays or change backend behavior without
reconciling the design. Network loss between requests can leave a stopped session.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/session-tab-restart.md)
- [System design](../../specs/tasks/system-design/session-tab-restart.md)
- Existing session-action tests and `mobile-session-deletion.spec.ts` for menu access.
- Root and web `AGENTS.md`, `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer`.
- Requested `better-ui`, `i-have-adhd`, and `asd-ste100` skills.

## Results

Completed on 2026-10-14 in the primary session. No subagents were used.

- Targeted Vitest command above: 4 files, 93 tests passed. The restart tests
  first failed on the missing action before implementation.
- Source, changed tests, and E2E ESLint checks: passed with `--max-warnings 0`.
  Prettier checks for all changed production TypeScript files passed.
- `NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck`: passed. The default
  and 4096 MiB heaps were insufficient for this repository's full typecheck.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed. Existing orphan-key
  notices remain advisory. Traditional Chinese and pseudo catalogs were generated.
- `PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build --project chromium tests/session/session-tab-restart.spec.ts -- --retries=0`:
  3 tests passed. The initial managed run built current backend and Vite assets.
  Later runs reused those same assets after test-only corrections.
- `PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-session-tab-restart.spec.ts -- --retries=0`:
  1 test passed, including preservation of the active session's unsent draft.
- Public-doc validator tests: 63 passed. Public-doc validation: 48 pages passed.
- Specification catalog validation and full specification lint: passed.
  Specification linter tests: 36 passed.
- PR documentation coverage preflight and whitespace checks: passed.

Browser checks proved the preview ordering, keyboard activation, phone menu,
focus return, settled 44px hit areas, 700/767/768px boundaries, and no horizontal
page overflow. No custom motion or visual surface was introduced.

The original primary-preservation assumption conflicted with normal Stop.
The requirements and design now retain the existing automatic promotion of an
active sibling, consistent with the user's confirmed stop-and-resume sequence.
The restart action never sends `session.set_primary`.

Public docs updated: `docs/public/sessions-and-review.md`.

## PR remediation results

The action restores selection after successful or failed Resume when Stop
caused automatic sibling adoption. Later user navigation takes precedence.
The regression first failed because session `s2` remained selected instead of `s1`.
Desktop and phone coverage now includes a selected target with a live sibling.
Public session guidance explains how restart preserves the user's selection.

Local verification completed on 2026-10-19:

- The four-file Vitest command in Verification passed: 97 tests.
- Changed-file ESLint and Prettier passed, including the restart E2E helpers.
- `NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck` passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed.
- `pnpm e2e:run --host --no-build --project chromium tests/session/session-tab-restart.spec.ts -- --retries=0`
  passed all 4 desktop tests after a fresh managed build.
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-session-tab-restart.spec.ts tests/chat/mobile-chat-status-bar.spec.ts tests/system/mobile-logs-bundle.spec.ts -- --retries=0`
  passed all 8 phone tests, including the previously failing workflow and
  diagnostic drawer checks.
- Focused Playwright discovery for the six files with CI discovery errors passed:
  31 tests. Fresh CI remains the final evidence for that shard.
- Public-doc validation passed for 48 pages. Specification catalog validation,
  full specification lint, and whitespace checks passed.

The browser environment removed inherited Kandev launch configuration and used
the installed Chromium library paths. These were local runner settings.
The running-turn test now checks retained data and recovery after an uncertain
prompt outcome. A launch acknowledgment alone does not prove readiness.
It covers the existing recovery boundary without replaying the prompt.

Public documentation type: how-to guidance for session selection.
Menu geometry and motion follow the existing shared primitives.
The checks found no actionable UI polish issue.
Final commit verification, screenshot publication, remote CI, and review remain
delivery steps. Exact-head remote evidence belongs in the PR and handoff.

### Selection ownership follow-up

The restoration fallback preserves whether the original selection was automatic
or pinned. The real-store regression first reproduced an unwanted user pin after
both successful and failed Resume. Automatic restoration now uses
`setActiveSessionAuto`, which leaves the navigation revision unchanged.
Existing pins still use `setActiveSession`, and later user navigation wins.

The follow-up changes shared selection state without changing rendered markup,
touch controls, or responsive composition. The existing desktop and phone
restart scenarios remain the browser coverage for the shared action.
Function contracts now document the affected composer, menu, and WebSocket guards.

Local verification on 2026-10-20:

- `pnpm exec vitest run hooks/domains/session/use-session-restart.test.ts hooks/domains/session/use-session-actions.test.ts`
  passed: 35 tests in 2 files. The new real-store cases cover automatic and
  pinned selections after successful and failed Resume.
- The broader affected suites passed: 852 tests in 79 files. From `apps/web`:

  ```bash
  pnpm exec vitest run \
    hooks/domains/session/use-session-restart.test.ts \
    hooks/domains/session/use-session-actions.test.ts \
    components/task/session-tab-menu.test.tsx \
    components/task/preview-session-tabs.test.tsx \
    components/task/mobile/mobile-sessions-section.test.tsx \
    components/task/chat/chat-input-area.test.tsx \
    components/task/chat/chat-input-area-restart.test.tsx \
    components/task/chat/use-chat-input-container.test.ts \
    components/task/chat/use-composer-props.test.tsx \
    components/task/chat/chat-input-container.test.tsx \
    components/task/chat/chat-input-body.test.tsx \
    components/task/chat/use-chat-input-state.test.ts \
    lib/ws/handlers --maxWorkers=2
  ```

- Changed-file ESLint, TypeScript checks, specification catalog validation,
  and full specification lint passed. Normal commit hooks and current-head
  remote CI remain delivery checks.

### Delete-mode tab close follow-up

The delete-mode tab close button uses the shared restart state. It remains
unavailable during both Stop and Resume, including the temporary CANCELLED
projection. Hide-panel behavior is unchanged because it retains the session.

The browser regression first failed because the delete button remained visible
while restart was pending. It also checks that the button is available before
restart and returns after the sequence settles. The test restores the saved
tab close preference in its cleanup.

The focused desktop command passed both the pending-restart regression and the
existing hide-panel persistence scenario, with retries disabled:

```bash
pnpm --dir apps/web e2e:run --host --no-build --project chromium \
  tests/session/session-tab-close-guard.spec.ts \
  tests/session/session-tab-management.spec.ts -- \
  --grep 'blocks delete-mode tab close throughout restart|opt-in tab close hides a panel' \
  --retries=0
```

The run reused the fresh production build that contains the close guard.
The three remaining named test helpers identified by the documentation audit
now document their fixture and focus contracts.

### CI regression follow-up

The interrupted-turn scenario reproduced an extra `session.launch` frame from
the existing `session_focus` recovery signal. Gateway traffic now retains
`activation_source`, so the regression checks one explicit Stop/Resume pair
separately from focus signals. A new traffic-capture unit test failed before
that metadata was retained and passed afterward.

The same CI shard exposed a workflow-preview race during a primary-session
change. The focused scenario reproduced the protocol error with retries disabled.
Its helper now waits for the new primary projection, matches the destination
step, and reads the response body before checking the rendered preview.

The merge with main preserves both sides of the eight task translation catalogs.
Affected unit suites passed: 864 tests in 82 files. Translation, documentation
catalog, and specification checks also passed.

The seven desktop session restart and tab-close cases passed. All three
workflow-preview cases passed three repetitions each (nine runs), with retries
disabled.
Both phone restart cases also passed with retries disabled.
