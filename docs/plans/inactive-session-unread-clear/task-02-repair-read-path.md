---
id: "02-repair-read-path"
title: "Repair the demonstrated inactive-session read path"
status: done
wave: 2
depends_on:
  - 01-establish-failure-boundary
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
acceptance_criteria:
  - AC-TASKS-TASK-UNREAD-DOT-001.2
  - AC-TASKS-TASK-UNREAD-DOT-001.4
  - AC-TASKS-TASK-UNREAD-DOT-001.5
  - AC-TASKS-TASK-UNREAD-DOT-001.6
  - AC-TASKS-TASK-UNREAD-DOT-001.8
  - AC-TASKS-TASK-UNREAD-DOT-001.9
  - AC-TASKS-TASK-UNREAD-DOT-001.11
  - AC-TASKS-TASK-UNREAD-DOT-001.12
  - AC-TASKS-TASK-UNREAD-DOT-001.14
  - AC-TASKS-TASK-UNREAD-DOT-001.15
  - AC-TASKS-TASK-UNREAD-DOT-001.16
  - AC-TASKS-TASK-UNREAD-DOT-001.17
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
---

# Task 02: Repair the demonstrated inactive-session read path

## Summary

Restore the existing loaded-transcript acknowledgement flow at the exact
boundary established by Task 01. Reading a stored failed/stopped session must
not need a live agent or another message, and must not acknowledge its siblings.

The later reported failed-resume incident supplied the missing causal evidence:
two automatic selection paths moved away before the failed transcript's history
arrived. This order corrects those paths and retains the existing per-session
acknowledgement rules. The initial cursorless fixture's conformance alone did
not authorize this production change.

## In scope

- After the diagnosis gate and a user implementation request, mark this order
  `in_progress`. Follow `/tdd` and read scoped guidance for every implicated area.
- Preserve Task 01's behavioral RED and add a focused test of the precise failing
  transition at its real boundary, if E2E alone cannot isolate it. Existing hook
  tests cover hydration, page visibility, first visits, and response correlation;
  extend the relevant case instead of asserting a new lifecycle parameter that
  the hook does not consume.
- Repair only the demonstrated stored-session selection/history/readiness or
  acknowledgement boundary. Resolve persisted history independently of runtime
  readiness while keeping route/session ownership and visible-page guards.
- Keep `newestAcknowledgeableMessageId` as the raw loaded-tail boundary, and use
  the existing `markSessionRead` API, cursor-only local update, monotonic server
  write, and summary event. Do not clear a task summary optimistically.
- Preserve cursorless first-visit divider behavior and latest-request correlation
  by session, including valid responses after navigation/unmount and same-session
  supersession. Do not add polling or error retries; only membership invalidation
  revalidates a still-owned removal proof.
- Finish the desktop and phone regression in the existing unread-dot specs,
  including durable read state after navigation/reload and an unread sibling
  that must retain its contribution. Include a mixed live/failed case.
- Exercise a real browser on disposable data. Observe the stored transcript,
  acknowledgement, and task dot after leaving; inspect the phone rendering.
- Update the owning design only if the confirmed repair changes a documented
  technical boundary. This is a behavior-preserving repair, not a new product
  contract. Public docs remain unchanged unless the confirmed solution changes
  user actions or terminology; then load `/docs-maintainer` and revise scope.

- R23 extends the existing move response with optional
  `workflow_focus_disposition: "none"` for a positively source-less, admitted
  agentless entry. The service owns this fact; HTTP/WS/MCP forward it. Missing
  disposition never means no recipient. No endpoint, durable metadata, database
  schema or launch-policy change is needed.

## Out of scope

- Starting/resuming/restarting an agent in order to read stored messages.
- Rewriting .9/.12/.17, marking other sessions read on task selection, filtering
  unread by terminal state, or deleting boot/error output to silence the dot.
- New endpoints, database migrations, flags, metrics, permanent diagnostic
  scaffolding, controls, localization keys, or unrelated transcript refactors.
- Proceeding through a newly discovered backend/persistence failure without
  first reconciling this order's scope, source owner, and exact checks.

## Acceptance

1. The demonstrated RED becomes GREEN through a minimal correction at the
   recorded boundary: explicitly viewing the failed session with no execution
   persists its newest acknowledgeable loaded row, removes only its ID from the
   unread aggregate, and never starts an agent.
2. Desktop and phone flows retain the persisted read state after navigation and
   reload; another unread sibling remains unread, and page/ownership guards and
   divider/cursor response-correlation behavior remain intact.
3. Every command below and Task 01's additional defect-specific command has an
   observed result in Results. Record browser smoke and phone inspection; mark
   this order `done` and the plan `implemented` only after the requirements and
   current design still match the implementation. Do not promote a speculative
   or unreproduced repair.

## ASCII UI preview

### UI-01: Desktop inactive-session read transition

Full [plan preview](plan.md#ascii-ui-preview), mapping .6/.9/.12/.14/.16/.17:

```text
Task (*) | [#1 primary] [#2 failed] [+ reopen]
                  select #2
Task     | [#1 primary] [#2 failed SELECTED] stored failure output
```

The dot is absent only if every other session is read. The failed state/error
indicator stays; this correction is not an error dismissal or launch action.

For the reproduced resume failure, the transition is instead:

```text
Before: #1 Starting -> #1 Failed -> #2 Waiting (failure hidden, #1 unread)
After:  #1 Starting -> #1 Failed [still selected]
        history loads -> failure + recovery actions visible -> #1 read
```

The server may promote #2 to primary without changing the selected failed tab.
Normal completed/cancelled handoff and explicit session selection remain.

### UI-02: Phone inactive-session read transition

Mapping .6/.9/.12/.15/.16/.17:

```text
[Task (*)] -> [Session #1 v] -> existing Sessions sheet
Tap #2 Failed -> [Session #2 v] + stored failure transcript
Reopen task picker -> no dot if every session is read
```

Keep the existing focused session picker, touch entry point, scroll owner,
safe-area handling, focus return, and row height. Do not mount desktop Dockview
on phones or add hover-only actions. Spacing and English labels are illustrative;
there is no new product copy or layout.

The same failed-resume transition keeps the failed chat selected on phone:

```text
[Session #1 v] Starting -> Failed [same selected chat]
                         failure + recovery actions
                         history loads -> #1 read
```

The existing session picker remains available for an explicit change.

## Verification

Run from the task repository root. The following creates a disposable cache-backed
copy of the current working tree, including untracked tests and deletions. It does
not test committed HEAD alone. See [snapshot rules](plan.md#verification): refresh
after every source edit; apply formatting corrections back with normal edits,
not generated artifacts. Stop on skipped required source symlinks or cache-full
conditions. All checks below run inside the snapshot in the same Bash shell.

```bash
set -euo pipefail
SOURCE=$(git rev-parse --show-toplevel)
SNAPSHOT=$(mktemp -d "${TMPDIR:-/tmp}/unread-verification.XXXXXX")
VERIFY="$SNAPSHOT/repo"
git clone --no-hardlinks "$SOURCE" "$VERIFY"
rsync -a --safe-links --delete --exclude='/.git' --exclude='node_modules/' \
  --exclude='/apps/web/dist/' --exclude='/apps/backend/bin/' \
  --exclude='/apps/backend/.build/' --exclude='/apps/web/e2e/test-results*/' \
  --exclude='/apps/web/e2e/blob-report/' --exclude='/apps/web/e2e/playwright-report/' \
  --exclude='/apps/web/.pr-assets/' "$SOURCE/" "$VERIFY/"
export TMPDIR="$SNAPSHOT/tmp" GOTMPDIR="$SNAPSHOT/tmp"
export XDG_CACHE_HOME="$SNAPSHOT/cache" npm_config_cache="$SNAPSHOT/npm"
export GOCACHE="$SNAPSHOT/go-build" GOMODCACHE="$SNAPSHOT/go-mod" GOPATH="$SNAPSHOT/go"
export CARGO_HOME="$SNAPSHOT/cargo" CARGO_TARGET_DIR="$SNAPSHOT/target"
export PYTHONDONTWRITEBYTECODE=1
mkdir -p "$TMPDIR" "$XDG_CACHE_HOME" "$npm_config_cache" "$GOCACHE" "$GOMODCACHE" "$GOPATH" "$CARGO_HOME" "$CARGO_TARGET_DIR"
cd "$VERIFY"
(cd apps && pnpm install --frozen-lockfile --store-dir "$SNAPSHOT/pnpm-store")
(cd apps/web && pnpm exec vitest run components/task/chat/use-session-read-tracking.test.ts components/task/task-unread-dot-visibility.test.ts lib/session-unread-divider.test.ts)
(cd apps/web && pnpm exec vitest run \
  lib/ws/handlers/task-primary-focus.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-continuation.test.ts \
  lib/ws/handlers/task-primary-removal-admission.test.ts \
  lib/ws/handlers/tasks.deleted.test.ts \
  lib/ws/handlers/agent-session.test.ts \
  lib/ws/handlers/agent-session-pure.test.ts \
  lib/ws/handlers/tasks.test.ts \
  lib/state/workflow-session-focus.test.ts \
  lib/state/shared-resource-reads.test.ts \
  lib/state/task-session-reads.test.ts \
  hooks/use-task-sessions.removal.test.ts \
  hooks/use-task-removal-session-loading.test.ts \
  hooks/domains/session/use-session-actions.test.ts \
  hooks/domains/session/use-session-actions.removal-proof.test.ts \
  src/task-detail-route-recovery.test.tsx \
  lib/task-agent-error.test.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts tests/session/session-primary-removal.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts -- --retries=0)
make fmt
make typecheck test lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/inactive-session-unread-clear
```

Keep `--host --shards 1`: multi-shard logs and runner cleanup use hardcoded
`/tmp` paths. Do not invoke the runner's `clean` command.

Run the full focused files, not just the new test. Managed E2E rebuilds; desktop
and mobile run sequentially. If a confirmed fix changes another hook's test or
backend package, add its exact narrow command before editing; existing broad
`make test` is not a substitute for naming that boundary's regression.

## Files touched

Production ownership narrowed by the failed-resume reproduction (see
[Causal reproduction](#causal-reproduction-and-reconciled-scope)):

- `apps/web/lib/ws/handlers/agent-session.ts` and `agent-session.test.ts`
- `apps/web/lib/ws/handlers/tasks.ts` and `tasks.test.ts`
- `apps/web/lib/ws/handlers/task-primary-focus.ts` and `task-primary-focus.test.ts`
  (authoritative, selection-owned previous-session verification).
- `apps/web/lib/ws/handlers/session-selection.ts` and `agent-session-pure.test.ts`
  (existing pin/terminal predicates moved unchanged to avoid a controller cycle).
- `apps/web/lib/state/workflow-session-focus.ts` (idempotent empty cancellation).
  The existing workflow-focus tests remain part of the focused command.
- `apps/web/lib/ws/handlers/task-primary-lifecycle.test.ts` and shared test
  fixtures (pre-promotion lifecycle and committed removal coverage).
- `apps/web/lib/state/shared-resource-reads.ts`, `task-session-reads.ts`,
  `hooks/use-task-sessions.ts`, and their focused tests (membership publication fence).
- `apps/web/hooks/task-removal-session-loading.ts` and `use-task-removal.ts`
  (removal-dialog admission shares the fence while preserving latest forced reads).
- Local delete, task-delete and config-chat retirement callers invalidate
  membership before evicting session data; no component markup changes.
- Terminal-predicate consumers in coordinator copilot, passthrough toolbar,
  agent-error acknowledgements, and `lib/task-agent-error.ts`: import relocation
  only, with no UI copy/layout change.

The R1 gateway callback-ordering change and its queue-model regression were
withdrawn after R2 showed later hub/client inversions. That historical Go
experiment is not an end-to-end ordering guarantee or a claim that broad
backend checks passed.

Shared verification and delivery records:

- `apps/web/e2e/helpers/unread-divider.ts`
- `apps/web/e2e/tests/task/task-unread-dot.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-unread-dot.spec.ts`
- `apps/web/e2e/tests/session/session-primary-removal.spec.ts`
  (real two-context deletion and no-reload follow).
- `docs/specs/tasks/system-design/task-unread-dot.md` (failed-session focus).
- This work order and `plan.md` statuses/results.
- `docs/public/operations.md` (failed startup/resume stays visible).

The read-tracking hook, panel, history hook, session menus, requirement, and
`task-unread-dot-visibility.ts` were investigated and left unchanged.

## Dependencies

Task `01-establish-failure-boundary` established initial conformance. The later
incident's causal reproduction below supplies the missing failure, exact
production owners, and regression commands; it unblocks this repair.

## Risks

- A displayed-session-only assertion can pass without a persisted acknowledgement.
- Removing hydration/ownership guards can read another task or a partial window.
- The reported shape may conform already; do not replace diagnosis with a
  lifecycle check, broad suppression, or a task-wide acknowledgement shortcut.
- A late mark-read response may legitimately update the session after navigation;
  tying freshness to visibility would reintroduce stale unread state.

## Parallelism

`sequential`

## Inputs

- Task 01 Results and its failing regression.
- [Task requirement](../../specs/tasks/requirements/task-unread-dot.md).
- [Inactive-session design](../../specs/tasks/system-design/task-unread-dot.md#inactive-session-acknowledgement).
- [Office cursor and response correlation](../../specs/office/system-design/unread-divider.md).
- Existing hook tests and desktop/phone unread-dot fixtures.

## Results

### Initial implementation checkpoint (before production-fix review)

Before the new incident this order was blocked: Task 01 found conformance for
a cursorless `FAILED` sibling, and its desktop/phone regressions remain.

The incident's failed-resume path is now reproduced and corrected (details in
[Causal reproduction](#causal-reproduction-and-reconciled-scope)). All runs used
cache-backed snapshot `~/.cache/tmp/unread-verification.zWhGjx`, refreshed from
this working tree after each source edit, with `KANDEV_*`/`AGENTCTL_*` unset,
offline Go modules and pnpm store, and `--host --shards 1`.

- RED, before the selection change: `agent-session.test.ts` "keeps the active
  session when it fails to start or resume" handed off to `s-waiting`;
  `tasks.test.ts` "keeps focus on a failed previous primary" called
  `setActiveSessionAuto`. The desktop E2E "keeps a session whose resume fails on
  screen until its failure is read" failed with the sibling as the active chat.
  With only the state handoff fixed it still failed four of four attempts,
  which exposed the `task.updated` primary-follow path.
- GREEN: `vitest run lib/ws/handlers` plus the read-tracking, dot-visibility,
  and divider files: 75 files, 725 tests passed.
  `vitest run lib/ws lib/state hooks/domains/session`: 3605 of 3606 passed;
  the one failure, `task-overview.test.ts` "rejects expired read protection",
  timed out at 8.2 s under parallel load and passed 7/7 when rerun alone.
- `pnpm e2e:run ... --project chromium tests/task/task-unread-dot.spec.ts`:
  3 passed. `... --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts`:
  3 passed. Both include the earlier failed-sibling and background-task tests.
- Related selection specs `session-tab-management`, `session-handoff`,
  `multi-session-ux`, and `session-recovery` (chromium, no retries): 30 passed.
- Browser smoke (real mock-agent resume failure, disposable backend): desktop
  keeps tab 1 (the failed session) active with "Failed to resume agent Mock",
  the startup failure, and Resume/Start fresh/Restore read-only actions; the
  task row shows no unread dot. `mark-read` targeted the stored tail recovery
  card, the cursor persisted there, `unread_session_ids` was `[]`, and no
  launch was sent for the sibling (before the fix the sibling was launched after
  the handoff). The phone screenshot shows the same session (1/2) and actions
  without horizontal overflow.
- Changed-file `oxlint --deny-warnings`, the e2e-sleeps lint, Prettier,
  `pnpm run typecheck`, `pnpm run i18n:ratchet`, `list-docs.py validate`,
  `lint-spec-files.py --all`, `lint-spec-files.test.py`, and `git diff --check`
  passed.
- No phone RED was run against the pre-fix bundle; the corrected rules are
  store handlers shared by both layouts.
- `make fmt`: exit 0, no change to any edited file. Exit 0 also for
  `make lint-web lint-harness lint-specs lint-architecture` and
  `make typecheck`. Go code is unchanged, so `make test` and `lint-backend`
  were not rerun; their recorded pre-existing failures are in
  [plan.md](plan.md#verification-results).
- Inference, not observed: the live incident sessions were deleted before a
  browser trace could be captured. The reproduction yields the incident's exact
  stored tail, cursor position, and sibling-primary outcome.

Parent integration: retaining the failed view is accepted as the minimal repair
to AC .6, consistent with the visible error required by
[launch failure recovery AC .8](../../specs/tasks/requirements/task-launch-failure-recovery.md).
No unread requirement or eligibility predicate changes. Backend primary
promotion remains authoritative for workflow ownership, but does not dismiss
the failure the user is viewing. Existing completed/cancelled handoff remains.
The failed-resume fixture intentionally launches the target to reproduce its
resume failure; the correction introduces no launch and does not start its
sibling. Earlier stored-session coverage still proves reading without launch.
At the initial implementation checkpoint this order was complete based on the
focused results above, not a clean broad suite; its edits were then uncommitted.
That production fix was subsequently pushed as `e4e01b8b35`. Its independent
failed-resume review and remediation are recorded in the plan's new loop.

Parent documentation checks passed: `list-docs.py validate` (455 decisions,
2037 specifications), `lint-spec-files.py --all`, public-doc validator tests
(63/63), and `validate-public-docs.mjs` (55 pages). Public recovery guidance is
updated in `docs/public/operations.md`; its primary content type is a how-to
guide. Parent inspected the retained desktop/phone and related-selection E2E
logs; their results are 3, 3, and 30 passed, respectively.

### Review and Remediation History

Across rounds 1 through 46, iterative review and remediation hardened session removal, focus transitions, unread dot calculation, and verification:
- Preserved publisher and lifecycle order across task and session transitions, keeping backend primary promotion authoritative for workflow ownership without abruptly dismissing visible error views.
- Traced the failed-resume path, ensuring failed sessions are held on screen with actionable recovery cards until user interaction or explicit acknowledgment.
- Refined session removal and focus tracking across split panels and tabbed interfaces, preventing stale handoffs and race conditions during rapid state changes.
- Validated desktop and mobile layouts across Playwright E2E suites, unit tests, and cross-browser fixtures, closing all review feedback loops cleanly.
