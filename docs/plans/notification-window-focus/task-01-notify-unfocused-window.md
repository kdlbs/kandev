---
id: "01-notify-unfocused-window"
title: "Notify when the task window is unfocused"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATIONS-001.2
  - AC-PLATFORM-NOTIFICATIONS-001.3
  - AC-PLATFORM-NOTIFICATIONS-001.4
  - AC-PLATFORM-NOTIFICATIONS-001.11
system_design:
  - ../../specs/platform/system-design/local-notification-attention.md
---

# Task 01: Notify When the Task Window Is Unfocused

## Summary

Make the existing Local notification suppression predicate consult current
document focus. Prove the correction through registered real handlers and keep
all existing delivery behavior after admission intact.

## In scope

- Add independent tests in the existing handler suite using the matrix and
  exact behavior names in [the plan](plan.md#tests-and-acceptance-mapping).
- Extend `shouldSuppressNotification` with current `document.hasFocus()`.
- Make the existing viewing-task control deterministic, retain bounded
  compatibility controls, and cover task-scoped Office handling.
- Record actual RED/GREEN commands and results; keep plan/work-order lifecycle
  accurate and promote the draft design to current when implemented.

## Out of scope

No listeners/cache/settings, OS/native implementation, new event semantics,
click routing, delivery retry, provider/backend changes, active-task lifecycle,
settings races, markup, copy/i18n, new dependencies, or private qualification
source/runner. Do not edit notification-settings atomicity or prior packages.

## Acceptance

1. The `notification window attention` regressions fail on dispatch assertions
   against the unchanged handler for visible/unfocused/current task and for
   post-registration focus changes, for both semantic events and both transport
   paths. Collection/import/CLI errors are not RED evidence.
2. The minimal predicate correction makes that matrix green, while focused
   current-task suppression, hidden/other-task delivery, sound admission,
   event identity/copy, native selection, Office and existing fallbacks remain
   covered. Neither an OS toast nor audible playback is claimed.
3. The entire existing handler suite, scoped lint and canonical typecheck, spec/reference
   coverage checks, diff check, and normal hooks pass; record every command and
   its actual result. No production/test change occurs during the design phase.

## Verification

Run from the repository root after the later explicit implementation request.
Use the installed Node 24 environment. In a fresh worktree, run this exactly once
from `apps/` before package commands, only after the implementation lease:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

After writing tests and before changing production, run the affected RED group:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales lib/ws/handlers/notifications.test.ts --maxWorkers=1 -t 'notification window attention')
```

Record the failing dispatch assertions and passing controls. Then add the focus
conjunct and rerun the identical command for GREEN. After the final code edit,
run the full affected file and scoped lint:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales lib/ws/handlers/notifications.test.ts --maxWorkers=1)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/ws/handlers/notifications.ts lib/ws/handlers/notifications.test.ts)
```

The existing file uses full locale setup and is not in either reviewed
Node/browser allowlist. Do not add a new suite or alter project selection.
Vitest 4 accepts `--maxWorkers=1`; do not pass `--minWorkers`.

After the final affected GREEN and scoped eslint, run the maintained canonical
web typecheck, including its existing pretypecheck and rich-output checks:

```bash
(cd apps/web && NODE_OPTIONS="${NODE_OPTIONS:+${NODE_OPTIONS} }--max-old-space-size=8192" pnpm run typecheck)
```

The task-local 8 GiB heap was explicitly authorized after the default 4 GiB
attempt exhausted its heap. The canonical command and other Node options are
preserved; no temporary TypeScript config or host setting is changed.

Light documentation gates may run during design:

```bash
python3 scripts/list-docs.py validate
python3 scripts/list-docs.py specs --system platform --kind system-design --format paths
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository coverage validator against the four-artifact package and the
planned production path. This validates references before production is edited;
it is a documentation preflight, not product-test evidence:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/platform/requirements/notifications.md',
  'docs/specs/platform/system-design/local-notification-attention.md',
  'docs/plans/notification-window-focus/plan.md',
  'docs/plans/notification-window-focus/task-01-notify-unfocused-window.md',
];
const result = validateCoverage({
  changedFiles: [...paths, 'apps/web/lib/ws/handlers/notifications.ts']
    .map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
JS
git diff --check -- docs/plans/notification-window-focus
git status --short -- docs/plans/notification-window-focus
```

Normal commit hooks run during later authorized delivery. Do not bypass hooks or
run broad E2E/full-web checks for this predicate. If inputs change after GREEN,
rerun the affected tests. No live browser/native permission prompt is required.

## Files likely touched

- `apps/web/lib/ws/handlers/notifications.ts`
- `apps/web/lib/ws/handlers/notifications.test.ts`
- `docs/specs/platform/requirements/notifications.md`
- `docs/specs/platform/system-design/local-notification-attention.md`
- `docs/plans/notification-window-focus/plan.md`
- `docs/plans/notification-window-focus/task-01-notify-unfocused-window.md`

## Dependencies

None. The reviewed implementation release was received before execution.

## Risks

Explicitly restore document property/method overrides and Tauri globals. Reuse
distinct occurrence IDs across focus transitions so native dedup cannot mask
admission. Preserve the sound helper's preference checks and the unchanged
provider selection/permission boundaries. Canonical typecheck includes the existing generated release-note/changelog and
rich-output checks; preserve unrelated files and report any pre-existing errors.

## Parallelism

`sequential`

## Inputs

- [Requirement and criterion .11](../../specs/platform/requirements/notifications.md)
- [Attention gate and verification boundary](../../specs/platform/system-design/local-notification-attention.md)
- Existing handler test fixtures and native client/sound implementations.
- [Plan compatibility matrix](plan.md#technical-approach)

## Results

Completed on 2026-10-18. Reviewed implementation release received; canonical
`pnpm run typecheck` replaces the temporary configuration. No temporary config
was created.

- Setup: frozen workspace install passed once. An earlier wrapper-path typo
  exited before invoking pnpm; it was setup error, not an install or RED attempt.
- RED group before production: 10 dispatch assertion failures, 14 passing
  attention controls, 15 unselected existing tests. This includes both semantic
  events and transports, focus transitions, and the shared Office path. An
  earlier RED run reached the sound assertion first; assertion ordering was
  adjusted to demonstrate dispatch failures before the final RED receipt.
- GREEN group after the focus conjunct: 24 passed, 15 unselected existing tests.
- Full affected file: 39 passed. After test-only lint repairs, the final full
  affected file again passed all 39 tests. Following the owned TypeScript
  fixture corrections, the final affected file passed all 39 tests once more.
- Scoped `eslint --max-warnings 0`: passed with no output. The initial command
  exited zero with three warnings; an intermediate strict run exposed one
  remaining callback-length warning. Those fixture-only warnings were fixed.
- Canonical typecheck: attempted once, exited 134 from V8 heap exhaustion near
  the default 4 GiB limit. Pretypecheck and rich-output commands ran. There is
  no TypeScript PASS from that attempt. ROOT authorized one 8 GiB retry.
  That retry completed with exit 2 and nine TS2345 diagnostics in the new
  test fixtures (event tuple widening and union correlation after spreading).
  The tuple and occurrence fixture were corrected; canonical verification of
  that correction then passed the maintained canonical command with the same
  8 GiB task-local heap, after the final 39-test GREEN and zero-warning eslint.
- Catalog/spec lint, actual-path documentation coverage preflight (`covered`,
  no errors), and diff checks passed.
- Normal hooks run through the delivery commit without bypass. Their actual
  receipt and exact-head publication/hosted validation are tracked in the
  external task plan rather than embedded in this head-agnostic work order.

No broad web tests, E2E, backend tests, live OS-toast or audible playback proof
were run. The production diff only adds the current focus conjunct.
