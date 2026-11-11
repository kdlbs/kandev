---
id: "01-touch-disclosure"
title: "Restore touch disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-AGENT-TODO-LIST-PANEL-002
acceptance_criteria:
  - AC-UI-AGENT-TODO-LIST-PANEL-002.1
  - AC-UI-AGENT-TODO-LIST-PANEL-002.2
  - AC-UI-AGENT-TODO-LIST-PANEL-002.3
  - AC-UI-AGENT-TODO-LIST-PANEL-002.4
system_design:
  - ../../specs/ui/system-design/agent-todo-list-panel.md
---

# Task 01: Restore touch disclosure

## Summary

Select the interactive checklist disclosure for coarse pointers at any width, keeping compact chrome and fine-pointer desktop interactions as they are. Prove the original failure independently before the minimal production change, then verify real task/session behavior in Chromium and phone emulation.

## In scope

- Independently author a real-component regression using real responsive hooks and Radix primitives. First run against unchanged production and record the wide/coarse causal failure and phone/coarse plus desktop/fine passing controls.
- Change only `TodoIndicator` disclosure selection; keep item content, status, count and pinned panel behavior.
- Add native desktop/coarse, fine desktop and phone E2E tests with real session todo state, plus rendered captures and geometry/dismissal checks.

## Out of scope

Global responsive breakpoints, shared primitives and hook semantics, data/model changes, new copy or settings, panel and layout redesign.

## Acceptance

- Wide coarse tap opens and dismisses the checklist; compact phone/tablet tap and fine desktop hover/focus remain usable. Empty session has no chip.
- Real session checklist text, status and completion values remain correct; phone/coarse hit target is at least 44px and overlays fit without horizontal overflow. Focus returns on dismissal where applicable.
- Record causal RED and control results, GREEN, actual browser captures and all command exits; do not describe simulated component events as native browser evidence.

## Rendered interaction contract

See the [interaction contract](plan.md#rendered-interaction-contract) for AC-002.1-.3. The existing chip opens the same anchored checklist on phone, tablet and wide coarse touch, while fine desktop retains hover/focus. Verify the list's internal scrolling, viewport containment, touch target and dismissal against the rendered surface.

## Verification

Run sequentially from the repository root only after a later explicit implementation release and the global heavy lease. Put the pinned pnpm 9.15.9 directory before Node 24.21.0/bin and Go 1.26.0/bin on `PATH`. The directories below are distinct task-owned log locations. Preserve every returned native session handle and actual join/exit. Use `--summary` only for E2E. No install or heavy command is authorized by this plan alone.

```bash
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/install scripts/run-quiet install -- pnpm --dir apps install --frozen-lockfile
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/component-red scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser-locales components/task/chat/todo-indicator-touch.test.tsx --maxWorkers=1 --no-file-parallelism
# After the causal RED and passing controls, implement the minimal repair.
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/component-green scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser-locales components/task/chat/todo-indicator-touch.test.tsx --maxWorkers=1 --no-file-parallelism
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/content scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser components/task/chat/todo-indicator.test.tsx --maxWorkers=1 --no-file-parallelism
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/desktop-e2e scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/todo-indicator-touch.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/phone-e2e scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-todo-indicator-touch.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/lint scripts/run-quiet lint -- pnpm --dir apps/web exec eslint components/task/chat/todo-indicator.tsx components/task/chat/todo-indicator-touch.test.tsx e2e/tests/task/todo-indicator-touch.spec.ts e2e/tests/task/mobile-todo-indicator-touch.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/todo-touch-disclosure
```

The desktop managed run builds fresh. The phone command builds fresh too; add `--no-build` only if exact production input and built dist fingerprints prove the desktop artifacts remain unchanged. Preserve owned desktop captures outside `.pr-assets` before the next managed run clears it. If a shared browser helper is added, include its exact path in owned files and scoped lint. If typecheck is required, run canonical web typecheck at 6 GiB:

```bash
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/typecheck NODE_OPTIONS=--max-old-space-size=6144 scripts/run-quiet typecheck -- pnpm --dir apps/web run typecheck
```

Record actual outcomes; do not run broad suites.

## Files likely touched

- `apps/web/components/task/chat/todo-indicator.tsx`
- `apps/web/components/task/chat/todo-indicator-touch.test.tsx`
- `apps/web/e2e/helpers/todo-disclosure.ts`
- `apps/web/e2e/tests/task/todo-indicator-touch.spec.ts`
- `apps/web/e2e/tests/task/mobile-todo-indicator-touch.spec.ts`
- Owning requirement/design and this plan package.

## Dependencies

None. Later explicit implementation and heavy lease are required before production edits, install or checks.

## Risks

Confirm actual coarse media and real CDP touch events, not merely a changed viewport. Avoid counting transcript text as disclosure evidence. Preserve captures across managed cleanup.

## Parallelism

sequential

## Inputs

- [Requirement](../../specs/ui/requirements/agent-todo-list-panel.md#req-ui-agent-todo-list-panel-002-status-chip-disclosure) and [design](../../specs/ui/system-design/agent-todo-list-panel.md#status-chip-disclosure).
- `apps/web/components/task/chat/todo-indicator.tsx`, `apps/web/hooks/use-compact-task-chrome.ts`, and `apps/web/e2e/tests/settings/todo-list-panel.spec.ts` task setup.

## Results

The independent component test failed only the wide-coarse disclosure before the fix (1 failed, 3 controls passed); after the fix all 4 passed. The existing content suite passed (2 tests). Real managed Chromium passed wide CDP tap and touch scroll plus fine desktop hover/focus (2 tests), and mobile-chrome passed phone tap, CDP touch scroll and tap dismissal (1 test). Focus return, 44px targets and viewport containment were checked. Initial fresh builds passed; after the touch-only test refinement, unchanged production inputs and dist fingerprints were compared before and after focused `--no-build` reruns. Scoped ESLint, canonical web typecheck at 6 GiB and `i18n:check` passed. Browser captures and command logs are retained in task-owned evidence. Specification catalog and lint passed. Public docs need no change: existing access is repaired without new workflow or terminology.

## Required CI remediation: update notification coexistence

The current-head E2E shard 18 artifact shows the pseudo-locale update notification test matching two distinct update toasts by their shared title: one for v0.98.0 and the fixture's v1.2.3. The helper forwards the real backend WebSocket while injecting the fixture event. The exact upstream emitter of the first notification is not established. The repair owns only `apps/web/e2e/tests/i18n/pseudo-coverage.spec.ts` and this work-order record; no production behavior or sibling notification is suppressed.

First extend the existing test to deterministically emit a distinct sibling update notification through the existing fixture, assert both localized version-specific notifications are present, and retain the original shared-title selector for a causal RED with the same strict-locator ambiguity. Then select the intended v1.2.3 toast by its localized title **and** version-specific body, retaining a separate assertion of the complete expected localized body. Keep the sibling forwarded and rendered; do not use `first()` or `nth()`. Re-run the same focused test for GREEN. This checks coexistence and preserves the localization oracle, rather than accepting any one matching title. The focused coexistence test produced the expected shared-title strict-locator RED (two visible versions), then passed after selecting v1.2.3 and checking its full localized body.

Run sequentially from the repository root only after a separate implementation and HEAVY release, preserving every actual native handle and join. The current test has no describe-level retry override; if one is introduced, override it only in a disposable proof copy, never in the permanent suite. Use fresh managed assets, or `--no-build` only after exact production-input and dist fingerprints prove current assets match. The runner clears `.pr-assets`; preserve and checksum the owned PR captures and manifest before invoking it.

```bash
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/pseudo-red scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/i18n/pseudo-coverage.spec.ts -- --workers=1 --retries=0 --grep 'localizes a Kandev update notification received from the backend'
# After the expected strict-locator RED, correct the version-specific selector.
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/pseudo-green scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/i18n/pseudo-coverage.spec.ts -- --workers=1 --retries=0 --grep 'localizes a Kandev update notification received from the backend'
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/todo-touch-disclosure-evidence/pseudo-lint scripts/run-quiet lint -- pnpm --dir apps/web exec eslint --max-warnings=0 e2e/tests/i18n/pseudo-coverage.spec.ts
```

After the focused checks and work-order result update, stage the two owned paths and use normal active hooks, a Conventional Commit, and the authorized push. Preserve existing captures and PR body sections. No CI retry, observer renewal, or merge is authorized by this remediation plan.

Remediation results: managed Chromium with one shard, one worker and zero retries reported the expected strict-locator RED (1 failed) and then GREEN (1 passed), using `--no-build` after matching production-input and dist fingerprints. Scoped ESLint passed with zero warnings. The existing backend WebSocket path and sibling notification remain intact.
