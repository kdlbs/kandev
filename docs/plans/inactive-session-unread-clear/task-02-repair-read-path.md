---
id: "02-repair-read-path"
title: "Repair the demonstrated inactive-session read path"
status: in_progress
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
CACHE=/home/clem/.cache/tmp
case "$(findmnt -no SOURCE --target "$CACHE")" in /dev/sdd*) ;; *) exit 1 ;; esac
python3 -c 'import shutil; u=shutil.disk_usage("/mnt/clem-cache"); assert u.used/u.total < .9, "Cache above 90%; follow cache-full procedure before continuing"'
SNAPSHOT=$(mktemp -d "$CACHE/unread-verification.XXXXXX")
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
export PLAYWRIGHT_BROWSERS_PATH=/home/clem/.cache/ms-playwright
export PYTHONDONTWRITEBYTECODE=1
mkdir -p "$TMPDIR" "$XDG_CACHE_HOME" "$npm_config_cache" "$GOCACHE" "$GOMODCACHE" "$GOPATH" "$CARGO_HOME" "$CARGO_TARGET_DIR"
cache_only() {
  local p
  p=$(realpath -m "$1")
  while [[ ! -e "$p" ]]; do p=$(dirname "$p"); done
  case "$(findmnt -no SOURCE --target "$p")" in /dev/sdd*) ;; *) printf 'Unsafe output: %s\n' "$1" >&2; return 1 ;; esac
}
for p in "$VERIFY" "$VERIFY/apps/node_modules" "$VERIFY/apps/web/dist" \
  "$VERIFY/apps/backend/bin" "$VERIFY/apps/backend/.build" \
  "$VERIFY/apps/backend/cmd/plugin-fixture/fixture-package/ui" \
  "$VERIFY/apps/web/e2e/test-results" "$VERIFY/apps/web/e2e/blob-report" \
  "$VERIFY/apps/web/e2e/playwright-report" "$VERIFY/apps/web/.pr-assets" \
  "$TMPDIR" "$XDG_CACHE_HOME" "$npm_config_cache" "$GOCACHE" "$GOMODCACHE" \
  "$GOPATH" "$CARGO_HOME" "$CARGO_TARGET_DIR" "$PLAYWRIGHT_BROWSERS_PATH"; do cache_only "$p"; done
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

### Production-fix review round 1 remediation

F1 (historical, superseded by R2): R1 routed `task.updated` through the lifecycle
wildcard alongside session/task state changes. Its memory/NATS-style queue-model
test preserved publisher callback order, unlike separate callbacks.
R2 showed that later hub fan-out and client barriers can invert wire delivery;
the routing change and its limited ordering regression were therefore withdrawn.
The obsolete hard-coded subscription count/counter test remains removed rather
than re-pinned. No end-to-end ordering guarantee is inferred from R1's evidence.

Final backend command in the cache snapshot:
`go test -race ./internal/gateway/websocket ./internal/events/bus -count=1`:
both packages passed. An initial green assertion run exposed a goroutine leak
in the new test's uncancellable background context; cancelling that test context
fixed the fixture, and the final race/goleak run passed. This is NATS queue-model
coverage, not a claim of a live NATS-server integration run.

F2: both layouts now arm FAILED and sibling-primary notification waits before
navigation, use a non-reloading initial readiness check, verify authoritative
primary identity before/after, and require the new failed resume `agent_boot`
followed by a recovery-status tail after the seed cursor. The selected chat must
show the recovery card and failure cause before cursor/reload assertions.

Final E2E commands used `--host --no-build --shards 1`, owning projects, and
`--retries=0`, after a fresh managed backend/web/plugin build:

- Full desktop unread-dot spec: 3 passed (`r1-desktop-confirmed.log`).
- Full phone unread-dot spec: 3 passed (`r1-mobile-confirmed.log`).
- A separate cache-backed copy, `~/.cache/tmp/unread-r1-mutation.Qi8fHD`,
  removed both FAILED focus guards and rebuilt its frontend. Both strengthened
  failed-resume cases then failed on the active chat being the sibling instead
  of the failed session (desktop/mobile exit 1). These are behavioral REDs,
  not setup failures (`r1-mutant-desktop.log`, `r1-mutant-mobile.log`).
- The initial stronger assertions incorrectly expected provider detail in the
  stored recovery-card text and the obsolete bootstrap-card component. Current
  storage uses a safe generic startup summary; the active composer uses
  `SessionRecoveryCard`. Those assertion failures were corrected against source
  and the rendered page, not counted as product RED. The final cases assert
  structured boot/card identities, the visible failed-resume transcript entry,
  and the current recovery card's Start fresh action.
- Parent inspected the passing phone capture: failed session 1/2 remains
  displayed, failure entry and recovery controls visible, no horizontal overflow.
- Logs and retained result directories are under
  `~/.cache/tmp/unread-verification.zWhGjx/`. The final `--no-build` runs reused
  the managed build; subsequent edits were confined to test assertions/docs.

F3: delivery-time statements and earlier reviews are labelled historical.
The plan records the actual pushed production head and a separate review ledger.

### Production-fix review round 2 remediation

R2 showed two later inversion boundaries: the real FAILED payload can take the
global hub lane while a workspace-scoped primary update fans out directly, and
a session message barrier can defer FAILED behind replaceable output while
the task update uses the direct client lane. Callback FIFO is insufficient.
The R1 routing change and its limited queue regression are removed, rather than
adding more global queue constraints or weakening transcript barriers.

Generic primary-follow now verifies the previous session through the existing
session GET (`cache: no-store`). Until matching non-failed readback arrives,
selection remains unchanged while the authoritative task primary still updates.
Failure, missing identity/state, or read errors preserve the view. The response
does not hydrate a stale session or cursor. Selection, navigation (including away
and back), restart, and newer workflow-focus intents cancel/abort the request;
successive primary changes replace its target and reject old responses.
Explicit committed workflow focus still precedes generic follow.

The real app-store/handler regression fails before correction with a promoted
sibling active before the delayed FAILED frame. Its verification response is
held by a deferred API fake, so both delivery orders and ownership races are
deterministic, without needing to control every transport lane.

R2 verification, in the cache-backed working-tree snapshot:

- WS handlers plus the real workflow-focus store tests: 74 files / 717 passed
  (`r2-ws-tests.log`). Coverage includes both event orders, healthy terminal
  states, a committed workflow recipient, ownership cancellation, overlapping
  reads, and preservation of a newer cursor.
- Gateway/event-bus packages: `go test -race -count=1` passed for both
  (`r2-gateway.log`), after withdrawing the transport-only change.
- Fresh managed backend/web/plugin build, then the full desktop unread-dot
  spec: 3 passed. The full phone spec reused that build: 3 passed.
- Related desktop selection/reopen/handoff/recovery specs: 30 passed.
  All three E2E invocations used one host worker, one shard, and no retries
  (`r2-desktop.log`, `r2-mobile.log`, `r2-selection.log`).
- Production web typecheck: exit 0 (`r2-typecheck.log`).
- Final authority-helper regression after test grouping: 18 passed.
  Changed-file configured Oxlint (warnings denied), Prettier, and the
  new-code i18n ratchet passed.

### Production-fix review round 3 remediation

The terminal-frame shortcut recorded below was withdrawn in R4. The other R3
corrections remain; these results describe that earlier snapshot.

F1: with a healthy predecessor and a held verification GET, a terminal frame
could select the last eligible sibling instead of the already-promoted primary.
The real-handler/store regression seeded a third eligible session and failed
with `latest` selected instead of `next`, for both completion and cancellation.
The ended-session handler now settles a matching still-owned primary follow
before its existing fallback, aborting the read. Late responses cannot undo it.
No-pending fallback, explicit workflow focus, and FAILED focus stay unchanged.
The initial transition-observer approach also failed accepted terminal replays;
the final coordination handles accepted notifications directly, including replays.
The shared pin/terminal predicates moved unchanged into `session-selection.ts`,
so neither controller imports the other cyclically. Every consumer was migrated;
no compatibility re-export remains.

F2: scoped cancellation with neither a workflow intent nor request now returns
the same empty state. Deleting another task no longer invalidates a healthy
follow; a real newer workflow intent still does. The real task-deleted handler
reproduced the incorrect retained `old` session before this correction.

F3: both runnable verification sections now include the deterministic
`task-primary-focus.test.ts`, real workflow-focus tests, handler tests and
relocated-predicate coverage. Broad `make test` is not the only way to exercise
promotion-first delivery and ownership cancellation.

R3 focused verification:

- The focused handlers, predicate, workflow-focus, task-agent-error and read
  tracking tests passed: 9 files / 223 tests (`r3-final-units.log`).
- Changed-file configured Oxlint (warnings denied) and Prettier passed.
- Final authority-helper regression after completing the deletion event's typed
  payload: 25 passed (`r3-final-regression.log`). Web typecheck passed
  (`r3-typecheck-ready.log`).
- Fresh frontend build passed (`r3-build-ready.log`). Desktop unread-dot:
  3 passed; phone unread-dot: 3 passed; session-handoff: 1 passed. Each used
  one worker/shard and no retries (`r3-desktop.log`, `r3-mobile.log`,
  `r3-handoff.log`), with the unchanged R2 backend/plugin artifacts.
- Parent inspected the passing phone capture: failed session 1/2 stays selected,
  with the failure entry and Resume/Start fresh/Restore read-only controls visible.
  Captures are retained under `r3-mobile-results/`.
- Architecture lint and the new-code i18n ratchet passed. Earlier broad-suite
  limitations remain unchanged; no live NATS-server validation is claimed.
- The first direct frontend build lacked ignored `generated/changelog.json`
  and `generated/release-notes.json` after refreshing the overlay. The
  `pretypecheck` generators restore those inputs; run typecheck before building.
  This was a build-input failure, not a behavioral RED.

The R3 browser recipe below rebuilds the frontend, while reusing the recorded
R2 managed backend and fixture-plugin archive, whose sources are unchanged.
`--no-build` is valid only with those prepared artifacts; a fresh snapshot must
use the managed runner's full build from the main verification setup.

```bash
cd "$VERIFY/apps/web"
pnpm run typecheck
pnpm run build:vite
pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/task-unread-dot.spec.ts -- --retries=0 --output "$SNAPSHOT/r3-desktop-results"
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts -- --retries=0 --output "$SNAPSHOT/r3-mobile-results"
pnpm e2e:run --host --no-build --shards 1 --project chromium tests/session/session-handoff.spec.ts -- --retries=0 --output "$SNAPSHOT/r3-handoff-results"
```

### Production-fix review round 4 remediation

F1: an accepted terminal snapshot can still precede the server's resume failure.
It now defers the legacy list fallback and starts fresh no-store verification,
never selecting the projected sibling by itself. A denied/uncertain read keeps
the view-owned guard, so a later old snapshot cannot bypass a completed FAILED
read. Navigation, a newer pin/workflow intent, restart, task departure and a
successful selection still release it. There is no retry timer.
The unseen-sibling adoption branch also defers to an owned check and does not
replace a cached FAILED view. Two additional RED cases (`r4-adoption-red.log`)
exposed those bypasses; healthy completion/cancellation/idle adoption controls
still passed.

F2: every accepted primary projection reconciles an already-owned target using
the original displayed predecessor, before considering equality with the
immediately previous cached primary. `task.state_changed` can start a related
follow when it actually changes that primary; creation snapshots only retarget
existing follows. Superseding a target aborts its old read and starts a fresh
one after the new projection. Duplicate updates do not create another follow.

F3: on a session GET 404, a complete authenticated task-scoped membership read
must exclude the predecessor and contain exactly one projected replacement,
with matching task IDs and total count. Only then does the controller follow
the still-current target, retire the old session's reads, and evict its caches.
Generic errors, unreadable/incomplete/foreign membership, a still-present
predecessor and newer user ownership all preserve the view. This does not rely
on `session.removed` versus promotion delivery order: deletion commits before
the promotion, and the query proves the current membership.

The real-store RED run had 10 failures/30 passes (`r4-red.log`): old-snapshot
selection, stranded retargeting, absent-session follow, and corrected
terminal-before-read contract assertions. The new two-context browser case
also failed against the R3 frontend: after the other client deleted the FAILED
primary and the backend promoted its waiting sibling, the observer still
displayed the removed ID (`r4-removal-red.log`). The test also requires a
stable document time origin on success, excluding a reload as the repair.

The runnable desktop commands include
`tests/session/session-primary-removal.spec.ts`. R4 uses a fresh frontend with
the unchanged recorded backend/plugin artifacts; regenerate ignored JSON through
`pnpm run typecheck` before `pnpm run build:vite` when refreshing the snapshot.

R4 verification:

- The WS handlers plus workflow-focus, agent-error and focused read regressions
  passed: 78 files / 804 tests (`r4-final-unit.log`). The final authority file
  passed all 50 tests after fixture/lint cleanup (`r4-final-authority.log`).
- Configured changed-file Oxlint with warnings denied, Prettier, typecheck,
  fresh Vite build, i18n ratchet and architecture lint passed
  (`r4-lint.log`, `r4-format.log`, `r4-typecheck.log`, `r4-build.log`,
  `r4-i18n.log`, `r4-architecture.log`).
- Docs catalog validation and full spec lint passed. Broad backend/frontend
  suites and live NATS-server ordering are not certified by these checks.

The exact expanded unit invocation was:

```bash
cd "$VERIFY/apps/web"
pnpm exec vitest run lib/ws/handlers \
  lib/state/workflow-session-focus.test.ts lib/task-agent-error.test.ts \
  components/task/chat/use-session-read-tracking.test.ts \
  components/task/task-unread-dot-visibility.test.ts \
  lib/session-unread-divider.test.ts
```

Desktop E2E passed all 34 cases (`r4-desktop.log`), including the two-client
removal regression, unread-dot, handoff, tab management, multi-session UX and
session recovery. Phone unread-dot passed all 3 cases (`r4-phone.log`).
Both used one worker, no retries, a fresh R4 frontend and unchanged recorded
backend/plugin artifacts. The retained `phone-failed-resume.png` was inspected:
the failure entry and recovery controls remain visible within the viewport.

```bash
cd "$VERIFY/apps/web"
export TMPDIR=/home/clem/.cache/tmp GOTMPDIR=/home/clem/.cache/tmp
pnpm e2e:run --host --no-build --shards 1 --project chromium \
  tests/task/task-unread-dot.spec.ts \
  tests/session/session-primary-removal.spec.ts \
  tests/session/session-handoff.spec.ts \
  tests/session/session-tab-management.spec.ts \
  tests/session/multi-session-ux.spec.ts \
  tests/session/session-recovery.spec.ts \
  -- --retries=0 --output "$SNAPSHOT/r4-desktop-results"
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome \
  tests/task/mobile-task-unread-dot.spec.ts \
  -- --retries=0 --output "$SNAPSHOT/r4-phone-results"
```

### Production-fix review round 5 remediation

F1: lifecycle fallback now enters the same view-owned authority controller before
any promotion exists. Persisted `COMPLETED`/`CANCELLED` is required for terminal
fallback; new-sibling adoption also permits persisted `IDLE`. Cached `STARTING`
does not establish completion. Initial adoption without a same-task predecessor,
explicit workflow routing and protected pin/restart behavior retain their roles.
A later promotion replaces the lifecycle check even for the same candidate.

F2: the core registry now observes `session.removed` for the displayed predecessor,
with active-task scoping when `task_id` is absent. It verifies current membership
even if a failed predecessor is already non-primary and no task update follows.
Complete scoped absence plus exactly one primary proves the recipient, including
when the notification beats the primary projection. An unindexed recipient gets
its proven identity and required lifecycle fields before focus; existing rows and
cursors are never overwritten. Newer user selection still wins.

F3: confirmed removals invalidate the keyed membership generation before cache
eviction. Mounted consumers remain retained; stale transports, admitted but
unpublished results and queued refreshes are rejected. The removal-dialog loader
also joins that generation, retaining its independent latest-forced-read guard.
All existing explicit session-eviction callers use the shared invalidation entry.
Comment-conflict refresh already has a permanent routing-change guard including
membership IDs. No generic task-navigation ownership or unread contract changed.

Evidence retained under `~/.cache/tmp/unread-verification.zWhGjx`:

- `r5-red-boundaries.log`: 12 behavioral failures/5 controls passed. The earlier
  `r5-red.log` included a fixture arity error and is not the new-boundary RED proof.
- `r5-red-admitted.log` and `r5-red-dialog.log`: accepted-but-unpublished and
  removal-dialog responses each recreated the deleted row before their fences.
- `r5-red-local-removal.log`: a local successful deletion left its old shared
  membership read fulfilled before the invalidation entry was wired in.
- `r5-removal-red.log`: one real two-client case against the retained R4 frontend
  failed with deleted A still selected after B was already primary and the page
  received removal. This is not a no-promotion mock.
- Initial GREEN: lifecycle 15 tests, membership/read owners 25, expanded owners
  221, then removal callers 40. Final change-aware checks are recorded below.
- `r5-red-recipient.log`: a proof arriving before the recipient's lifecycle row
  left route ownership unresolved. Minimal canonical-row initialization repairs
  that boundary without copying the membership response's cursor.
- `r5-config-chat.log`: the nearest config-chat retirement consumer passed 22 tests.
- `r5-desktop.log` / `r5-phone.log`: full R5 desktop selection/unread/removal
  35 and phone unread 3 passed, one worker, retries 0. The retained phone capture
  shows the failed session, failure cause and recovery controls still visible.
  These runs precede the final unindexed-recipient correction; its affected
  final-bundle replay is recorded below.
- Final logic checks: `r5-final-unit-v4.log`, 98 files/1019 tests passed;
  `r5-config-chat.log`, 22 retirement-consumer tests passed. Configured changed-file
  Oxlint, final formatting, typecheck (8 GiB Node heap) and fresh Vite build passed
  (`r5-final-{lint,format,typecheck,build}-v4.log`). The intermediate v2 complexity
  failure and v3 canonical-row type error were repaired, not waived.
- Final-bundle replay: `r5-final-desktop.log`, 5 passed; `r5-final-phone.log`,
  3 passed. Both full unread files and the full removal file ran with one worker,
  retries 0, after the final canonical-row correction. The new two-client case
  proves a deleted, retained non-primary failure leaves the tab without reload.
- Final i18n ratchet and architecture checks passed (`r5-final-i18n.log`,
  `r5-final-architecture.log`). Ratchet rename-detection warnings remain reported;
  they are not broad-suite certification. Documentation uses the catalog and
  full specification-lint commands above, including the design's size budget.

Expanded final unit invocation, from the cache snapshot's `apps/web`:

```bash
pnpm exec vitest run lib/ws/handlers \
  lib/state/workflow-session-focus.test.ts lib/task-agent-error.test.ts \
  components/task/chat/use-session-read-tracking.test.ts \
  components/task/task-unread-dot-visibility.test.ts lib/session-unread-divider.test.ts \
  hooks/use-task-sessions hooks/use-task-removal \
  hooks/domains/session/use-session-actions.test.ts \
  lib/state/task-session-reads.test.ts lib/state/shared-resource-reads.test.ts \
  hooks/journey-metadata-resources.test.ts hooks/use-workflow-snapshot.test.ts \
  hooks/domains/kanban/use-all-workflow-snapshots
```

Final browser replay used the fresh `r5-final-build-v4.log` frontend and the
unchanged recorded R02 backend/plugin binaries. From the same cache snapshot:

```bash
pnpm e2e:run --host --no-build --shards 1 --project chromium \
  tests/task/task-unread-dot.spec.ts tests/session/session-primary-removal.spec.ts \
  -- --retries=0 --output "$SNAPSHOT/r5-final-desktop-results"
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome \
  tests/task/mobile-task-unread-dot.spec.ts \
  -- --retries=0 --output "$SNAPSHOT/r5-final-phone-results"
```

The state/read fixes are shared across viewports. Existing phone unread tests
exercise failure retention and persisted acknowledgement; the new desktop
two-client case adds the distinct committed-removal boundary. No layout, touch
surface or copy changed. R5 uses a fresh frontend over the recorded unchanged
backend/plugin artifacts. No broad-suite or live NATS certification is implied.

### Production-fix review round 6 remediation

Accepted source-traced races extend the same .6/.8/.9/.12/.17 repair:

- Fence confirmed-deleted IDs at session admission, including late lifecycle
  frames and optional route enrichment. Preserve ordinary cache eviction,
  in-place session reincarnation, surviving siblings and store/backend scope.
- Bind removal proof to the task membership generation; a second confirmed
  deletion must refresh the still-owned original proof before choosing a recipient.
- Preserve recipient-owned readiness/execution after a delayed predecessor GET.
  Inheritance is only a fallback, never authority over an independent execution.

Owners: session slice/types and admission guard, hydration, session WS handlers,
primary-focus controller and confirmed-deletion callers. Add real-store ordering
regressions and the existing SPA route-enrichment consumer test. No layout, touch
composition, server promotion, unread predicate, cursor writer or wire change.

Narrow regression command (cache snapshot, `apps/web`):

```bash
pnpm exec vitest run lib/ws/handlers/task-primary-removal-admission.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts lib/ws/handlers/agent-session.test.ts \
  lib/ws/handlers/tasks.deleted.test.ts \
  hooks/domains/session/use-session-actions.removal-proof.test.ts \
  src/task-detail-route-recovery.test.tsx
```

R6 implementation and evidence:

- Confirmed backend deletion, not ordinary cache eviction, records a client-only
  UUID fence scoped to the store/backend. Session UUID deletion cannot be undone
  by an unproven queue-incarnation change. New backend admission and ordinary
  in-place reincarnation remain usable. Canonical/list/single/event/route writers
  share the guard; hydration cannot replace the evidence. A stale membership
  merges surviving rows instead of dropping siblings learned since its snapshot.
  Per-reader generations alone cannot fence an admitted navigation identity or
  late lifecycle snapshot, so confirmed absence also gates final admission.
- Removal proof retains the task read key and checks its generation in the same
  continuation that publishes selection. Invalidation refreshes under the original
  still-owned removal, without a timer or steady-state error retry.
- Delayed readiness preserves the recipient's accepted projection, including its
  error and execution. Independent execution or environment metadata blocks fallback.
  An unknown recipient execution never receives the predecessor's execution ID.
  Initial adoption no longer copies readiness from a foreign task.
- `r6-red.log`: 10 behavioral failures/17 controls passed across late admission,
  stale proof and execution overwrite. `r6-red-inactive-delete.log` proves the
  real inactive-session deletion hook selected the deleted B after A's old proof.
  `r6-red-execution.log` and `r6-red-initial-execution.log` isolate predecessor
  identity leakage. Recipient assertions preserve the accepted reducer projection:
  a starting observation can legitimately derive ready from cached live state.
- `r6-red-task-delete.log`: one late-admission failure after task deletion and
  one new-backend control passed. The task deletion caller now records confirmed
  deletion for every known session source. Its old mock-call test was replaced
  with real-store eviction/admission and untouched-foreign-session assertions.
- Intermediate GREEN: admission/nearby store 61, route/admission 13, authority plus
  public deletion hook 52, removal cases 9, then six focused files/149 tests.
  Final change-aware checks are recorded below; intermediate runs are not their
  substitute. Two obsolete internal call-signature assertions were deleted, not
  repinned. No user-visible copy, layout or pointer/touch composition changed.
- Additional boundary proof: `r6-red-environment.log` rejected readiness borrowed
  into a different recipient environment. `r6-red-selection.log` proved recovered
  enrichment could restore the deleted active ID even with clean session indexes.
  Task-UI fragments targeting a confirmed-deleted ID are now rejected while other
  hydration continues. The readiness positive control starts with an unobserved
  recipient so it exercises actual fallback, not pre-existing recipient readiness.
- Final unit/type/build: `r6-final-unit-v5.log`, 154 files/1620 tests passed;
  `r6-final-typecheck-v5.log` passed with the documented 8 GiB Node heap;
  `r6-final-build-v3.log` is a fresh frontend build. Configured changed-file
  Oxlint and formatting passed after extracting task-selection hydration and
  splitting the admission test group to meet the existing complexity/size limits.
  The intermediate lint and type failures were corrected, not waived.
- Final i18n ratchet and architecture checks passed (`r6-final-i18n.log`,
  `r6-final-architecture.log`). Rename-detection warnings remain reported;
  neither check certifies the known-failing broad suites.
- The first normal pre-commit attempt applied a lifecycle-test Prettier wrap.
  The working tree retained that formatting for the retry; no logic changed.
- Final runtime: `r6-final-desktop.log`, 35 passed; `r6-final-phone.log`,
  3 passed. Both full unread files, removal, tab management, handoff, multi-session
  UX and recovery ran with one worker and retries 0 against the fresh frontend.
  The phone capture was inspected: the failed session, resume failure entry and
  recovery controls remain visible; the card reflects read-only restoration.
  These checks do not reproduce the deleted historical incident or certify
  broad backend/web suites. List readback remains convergence evidence, not
  isolated cursor-event delivery proof.

Expanded final unit invocation, from the cache snapshot's `apps/web`:

```bash
pnpm exec vitest run lib/ws/handlers lib/state/slices/session \
  lib/state/hydration lib/ssr/session-page src/task-detail-route \
  hooks/use-task-sessions hooks/use-task-removal \
  hooks/domains/session/use-session-actions \
  hooks/domains/session/use-workspace-restoration.test.ts \
  lib/state/task-session-reads.test.ts lib/state/shared-resource-reads.test.ts \
  lib/state/workflow-session-focus.test.ts lib/task-agent-error.test.ts \
  components/task/chat/use-session-read-tracking.test.ts \
  components/task/task-unread-dot-visibility.test.ts lib/session-unread-divider.test.ts
```

Runtime invocation after that fresh frontend build, from the same cache snapshot.
`--no-build` reuses the recorded R02 backend/plugin artifacts; it is not a new
backend build or live NATS validation. A clean snapshot must first build through
the managed recipe above.

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  tests/task/task-unread-dot.spec.ts tests/session/session-primary-removal.spec.ts \
  tests/session/session-tab-management.spec.ts tests/session/session-handoff.spec.ts \
  tests/session/multi-session-ux.spec.ts tests/session/session-recovery.spec.ts \
  -- --workers=1 --retries=0
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```



## New reported incident: existing cursor behind a failed resume

The user supplied task `eae34524-3f77-45ef-9d72-677f11d03cd4` ("Add message
turn metadata setting") on 2026-11-18 and explicitly requested investigation
followed by an Opus implementation child. It is a task ID, not a session ID.
This is new evidence; the earlier cursorless fixture does not cover its shape.

Read-only queries against `/home/clem/.kandev/data/kandev.db` (`mode=ro`):

- Primary `fed03818-f576-4480-b996-f37cb669ca32`: WAITING_FOR_INPUT, cursor
  `45b737b8-f4f9-478d-a5f8-cc3128cba86c` equals its newest stored row.
- Failed sibling `b4ff216d-f490-4659-8be7-9364ad6b204f`: existing cursor
  `f4dc8726-872e-5bf5-b695-f7c1228d20e6`, an older recovery status from
  2026-10-10 20:29:36Z. Session updated_at is 2026-11-15 16:12:34Z; this
  timestamp alone does not prove which UI session was displayed.
- Later row `1a81131d-04e4-40f9-8f72-37fb5ef33747` is script_execution,
  `script_type=agent_boot`, `is_resuming=true`, `status=failed`, created
  2026-10-11 12:38:08Z. It qualifies under the current backend predicate:
  only setup scripts are excluded, not resume-bootstrap rows.
- Newest row `c6437e39-9a32-594d-92b8-5cf444b14f2b` is a recovery status
  created 2026-10-11 12:38:13Z, with `recovery_actions=true` and action buttons.
  It does not itself qualify as unread. The old cursor also targets a recovery
  status; both recovery statuses follow distinct stored turns.
- Summary revision 3909 lists exactly the failed sibling as unread and
  `has_running_session=false`. Both sessions share task environment
  `7ff3ab43-f465-4a44-afce-182e257095a8`.
- The failed session has 1124 messages. Its two latest rows each have a distinct
  turn ID and prompt_seq=0. This differs from the short cursorless fixture.

The aggregate's cause is confirmed: a qualifying resume-bootstrap row ranks
after the stored cursor. The precise read-after-view failure remains unproven,
not dismissed. The user's observation is the reported failure; opening the
primary alone must not be silently substituted for viewing the failed session.
Browser relay observation was attempted but the relay extension was disconnected.
Retained backend log matches for the task were periodic PR group-owner routing,
not read-tracking evidence. No live cursor was written, transcript opened,
session restarted, or task-list API invoked to reconcile away the baseline.

### Incident-specific implementation assignment

Continue the existing .6/.8/.9/.12/.17 contract. Before production changes,
reproduce this exact existing-cursor/failed-resume/recovery-status shape in a
disposable environment, with history exceeding the initial 100-row page and
distinct turns. Preserve meaningful metadata and order without copying private
transcript content. Compare chronological stored tail, HTTP/WS history page,
raw frontend message tail, displayed session/route identity, history readiness,
mark-read dispatch/response, persisted cursor, and unread summary.

Trace `newestAcknowledgeableMessageId` and `TaskChatPanel` against the actual
loaded page: recovery rows are excluded by the server's unread predicate but
are eligible acknowledgement targets; resume boot rows can be hidden by
render-time filtering. The current code has no explicit FAILED-state guard.
Check paging, stored-turn coverage, cached-history/session activation and
request freshness rather than introducing a hypothetical lifecycle check.

Once a deterministic violation identifies the first broken boundary, reconcile
this order's exact production owner and narrow regression command, change it to
in_progress, and implement the smallest correction with failing-before/passing-
after proof. Keep earlier regressions. Use the existing cache-safe checks and
desktop/phone runtime smoke; add the exact newly implicated boundary's test.
No lifecycle-based unread suppression or implicit task-wide mark-read is
authorized. If evidence requires changing the product contract, report that
boundary to the parent rather than changing it silently. This incident authorizes
continued investigation and implementation, not a fabricated root cause.

### Causal reproduction and reconciled scope

Disposable reproductions ran in a cache-drive snapshot of this working tree.
No live database row was written; the live task's two sessions were deleted
by another actor on 2026-11-19 before this diagnosis finished.

Explicit reads of the exact stored shape conform. A failed sibling with more
than 100 rows, an old recovery-card cursor, a later `agent_boot` resume row
(`is_resuming=true`, `status=failed`, distinct lifecycle-only turn), and a
newer recovery card is acknowledged through the recovery card and leaves
`unread_session_ids` empty when it is selected by desktop tab, after a cached
same-task or cross-task return, with a shared task environment, and through
the phone session picker. A live failure delivered while the session stays
displayed is also acknowledged (cursor on the boot row; the card never counts).

The violation is the real failed-resume flow. Opening the task resumes the
displayed session automatically. With the mock agent's `--fail-on-resume`, the
session goes `WAITING_FOR_INPUT -> STARTING -> FAILED` in about two seconds and
writes exactly the incident's tail: the qualifying resume `agent_boot` row and
a non-qualifying recovery card. On `STARTING -> FAILED`,
`maybeAdoptSessionOnTransition` in `apps/web/lib/ws/handlers/agent-session.ts`
hands the active session to the waiting sibling about 130 ms later. The failed
panel stops being visible before its history recovery lands the boot row
(observed: `message.list` response one second after the handoff), so nothing
acknowledges it. The cursor stays on the old row, the session stays unread,
and its recovery card is hidden from the user who was looking at it. When the
failure takes six seconds instead, history lands while the session is still
displayed and the existing read path acknowledges the boot row and the card.
The incident data fits the race: its boot row was written 5.5 s before
`FAILED`, the cursor never reached it, the waiting sibling was primary
afterwards, and several sessions failed resume in the same two minutes as the
user moved between tasks.

With only the state handoff removed, a second path still moved focus: after the
failure the backend promotes the waiting sibling to primary
(`maybePromotePrimary`), and `maybeFollowUpdatedTaskPrimary` in
`apps/web/lib/ws/handlers/tasks.ts` follows `task.updated` to it. Both rules
follow workflow switches; neither applies to a session that failed.

Reconciled owners: `apps/web/lib/ws/handlers/agent-session.ts` and
`apps/web/lib/ws/handlers/tasks.ts`, stored-session selection after a failure.
A session entering `FAILED` keeps focus, and a `FAILED` previous primary is not
followed to its replacement. Handoffs when the active session completes or is
cancelled are unchanged. Read tracking, the unread predicate, and the
unread contract are unchanged. Narrow commands, from `apps/web`:

```bash
pnpm exec vitest run \
  lib/ws/handlers/task-primary-focus.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-removal-admission.test.ts \
  lib/ws/handlers/tasks.deleted.test.ts \
  lib/ws/handlers/agent-session.test.ts \
  lib/ws/handlers/agent-session-pure.test.ts \
  lib/ws/handlers/tasks.test.ts \
  lib/state/workflow-session-focus.test.ts \
  lib/state/shared-resource-reads.test.ts \
  lib/state/task-session-reads.test.ts \
  hooks/use-task-sessions.removal.test.ts \
  hooks/use-task-removal \
  hooks/domains/session/use-session-actions.test.ts \
  hooks/domains/session/use-session-actions.removal-proof.test.ts \
  src/task-detail-route-recovery.test.tsx \
  lib/task-agent-error.test.ts
pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts tests/session/session-primary-removal.spec.ts -- --retries=0
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts -- --retries=0
```

### Production-fix review round 7 remediation

Accepted source-derived races extend .6/.8/.9/.11 without changing unread semantics:

- An unchanged primary identity must preserve the pending lifecycle predicate.
  Only a changed primary projection supersedes it; explicit workflow focus is
  still separate. Prove a title update cannot turn durable STARTING into terminal
  handoff authority, with genuine promotion and settled-predecessor controls.
- Revalidate lifecycle recipients at publication. A captured recipient that has
  since failed or left the task is not eligible; reuse the current live fallback.
  Retain predecessor, pin, restart and workflow ownership.
- Complete, task/backend-scoped, generation-current absence proves retirement
  independently of focus intent or a successor. Zero-primary membership retires
  the deleted ID without choosing a guess. Clear a still-selected deleted ID;
  preserve a newer surviving pin and its cursor. Late snapshots remain fenced.
  Incomplete/ambiguous lists and generic read failures remain fail-closed.

Owners: primary-focus controller, shared session candidate policy and its existing
consumers/tests. Reuse the shipped empty-session surface on desktop and phone:
`deleted A -> no active transcript`, or `newer pin B -> B remains displayed`.
No layout, touch target, protocol, backend promotion or cursor change.

Regression owners: `task-primary-lifecycle.test.ts`, candidate policy tests, and
the real two-client removal scenario on desktop/phone. Run the focused handler
and policy files, then the affected selection/unread/removal E2Es from the cache
snapshot with one worker/retries 0 after a fresh frontend build. Final invocations
and outcomes are recorded below.

R7 evidence and ownership:

- `r7-red-unit.log`: 8 behavioral failures/24 controls passed. An unchanged
  primary selected C over both the failing and healthy lifecycle candidates;
  a newly failed recipient was activated; newer pins/navigation and zero-primary
  proof prevented retirement. The strengthened pin cases require actual eviction.
- `r7-red-desktop.log` and `r7-red-phone.log`: each new two-client last-session
  case failed against the retained R6 frontend after persisted membership was
  empty: the observer still displayed the deleted session ID. No reload or
  fixture mark-read was used. The desktop deleting client uses the shipped
  context menu and confirmation in both viewport cases.
- `r7-partial-unit.log`: 58 controls passed; one additional background-removal
  regression still failed. Retirement must not replace/cancel a healthy follow
  owned by another active session, not merely survive a pin after it started.
- Reuse keyed backend/auth/workspace membership ownership for retirement, but
  keep view ownership for focus only. Coupling both to navigation cancellation
  loses committed deletion; suppressing all selection guards would overwrite
  newer user intent. Existing requirements/design/tests preserve this local
  rationale; no separate ADR, new endpoint or persistence boundary is needed.

Narrow invocation, from cache-backed `apps/web`:

```bash
pnpm exec vitest run lib/ws/handlers/task-primary-focus.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-removal-admission.test.ts \
  lib/ws/handlers/agent-session.test.ts lib/ws/handlers/agent-session-pure.test.ts \
  lib/ws/handlers/tasks.test.ts lib/ws/handlers/tasks.deleted.test.ts \
  lib/state/task-session-reads.test.ts lib/state/shared-resource-reads.test.ts
```

- The expanded first run (`r7-final-unit-v3.log`) passed 1626 tests and exposed
  two retarget-order regressions plus one obsolete zero-primary negative case.
  Changed primary identity now invalidates membership and starts fresh scoped
  proof; old responses cannot choose the prior recipient. These original
  old-first/new-first controls remain unchanged.
- Zero-primary complete absence is no longer classified as ambiguous. Its old
  negative table entry was removed in favor of the positive empty/terminal-only
  retirement cases. Incomplete, foreign, duplicate and generic-failure controls
  remain. `r7-owner-green.log`: 109 owner/lifecycle/policy tests passed.
- The existing candidate policy moved from `agent-session.ts` to the shared
  session-selection module, with every caller migrated and no re-export shim.
  Lifecycle publication consults current canonical ownership/state before fallback.

- Final logic/type/build: `r7-final-unit-v6.log`, 154 files/1628 tests passed;
  `r7-typecheck-v6.log` passed with the documented 8 GiB Node heap;
  `r7-build-v6.log` is a fresh frontend build. The fixture now copies the real
  stored recipient row rather than casting an incomplete branded-ID shape.
  Configured changed-file lint/format passed; complexity and duplicate-string
  warnings and the intermediate fixture type errors were corrected, not waived.
- Final i18n ratchet and architecture passed (`r7-final-i18n.log`,
  `r7-final-architecture.log`); rename-detection warnings remain qualified.
  Broad backend/web and live NATS certification remain excluded.

The expanded final unit invocation is the R06 command above, with the complete
`lib/ws/handlers` owner rather than only the narrow R7 files. Runtime uses a fresh
R7 frontend and explicitly retained R02 backend/plugin artifacts (`--no-build`),
one worker and retries 0; no R7 Go or protocol change.

- First final-bundle removal run: two original cases passed; the new case's
  negated attribute assertion failed because the corrected UI removed the element
  entirely. It now requires zero transcript nodes for the deleted ID, not an
  existing node with a different attribute.
- Assertion repair proof: `r7-assertion-mutant.log` fails with expected count 0,
  received 1 after a cache-snapshot-only guard rejects zero-primary retirement.
  The production working tree was not mutated. The guard was restored through
  an anchored edit before rebuilding the real final frontend.

Final runtime invocation, from the prepared cache snapshot's `apps/web`:

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  tests/session/session-primary-removal.spec.ts -- --workers=1 --retries=0
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  tests/task/task-unread-dot.spec.ts tests/session/session-tab-management.spec.ts \
  tests/session/session-handoff.spec.ts tests/session/multi-session-ux.spec.ts \
  tests/session/session-recovery.spec.ts -- --workers=1 --retries=0
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```

- Final restored bundle: `r7-final-removal-v2.log`, 3 passed;
  `r7-final-related.log`, 33 passed; `r7-final-phone.log`, 4 passed. All three
  commands above used one worker/retries 0, after `r7-final-build.log` rebuilt
  the unmutated frontend. `r7-final-format.log` and `r7-final-lint.log` passed.
- The final phone image `phone-last-session-removed.png` was inspected: the
  same task remains open with the shipped empty-conversation surface, no deleted
  transcript or error card, and normal input/bottom navigation. The two-client
  test also requires persisted empty membership and unchanged document origin.
  This is actual UI proof, not injected page state.
- `r7-docs.log`: catalog validation, all 36 spec-linter tests and full spec lint
  passed. Only a scope doc-comment changed after the final unit/type run; no
  functional code changed. Historical-incident, backend-artifact, broad-suite
  and list-convergence limitations above remain in force.

### Production-fix review round 8 remediation

Accepted source-derived schedules preserve .6/.8/.9/.11 and explicit workflow focus:

- Retirement clears only the deleted selection/pin, not an independent newer
  workflow move. A request whose own destination is retired is cancelled.
- A still-owned deleted view can become an empty selection slot. A genuinely
  changed primary then re-proves membership before filling it; navigation,
  pins, restart and explicit workflow ownership still supersede the continuation.
  No later promotion means the slot remains empty, without polling.
- A trusted, task-matched removal notification immediately invalidates older
  membership proof and excludes that ID from automatic candidate publication.
  Final UUID retirement still requires complete scoped absence, not an error.
- Unknown projected recipients retain the original predecessor authority until
  real scoped admission proves their current ownership/eligibility. A newer
  predecessor state or user intent invalidates that authority.
- Rank eligible recipients by parsed `started_at`, with a deterministic ID tie,
  rather than HTTP/event insertion direction. Preserve both load orders.

Owners: primary-focus controller, its session-selection policy, and existing
workflow/cursor/retirement boundaries. No new endpoint, wire field, persistence,
unread predicate, cursor writer, layout or touch composition. Unit owner:
`lib/ws/handlers/task-primary-continuation.test.ts` using real handlers/store
and deferred external reads. Seven original cases failed before the fixes; the
five-file owner/control run then passed 140 tests. Desktop/phone removal tests
also hold real successor notifications and expose a controlled zero-primary HTTP
snapshot before releasing the unchanged promotion frame. This models the valid
commit-before-promotion interval without changing page state. The retained
pre-R08 frontend failed the phone case after release: the conversation stayed
empty instead of selecting the proven survivor (`r8-empty-slot-red.log`).
Run full affected checks and fresh-frontend E2Es below; retain R02 backend/plugin
provenance and one worker/retries 0. No SQL timing or live NATS claim.

R08 verification commands (same cache snapshot/environment; no source build):

```bash
cd "$VERIFY/apps/web"
pnpm exec vitest run lib/ws/handlers lib/state/slices/session \
  lib/state/hydration lib/ssr/session-page src/task-detail-route \
  hooks/use-task-sessions hooks/use-task-removal \
  hooks/domains/session/use-session-actions \
  hooks/domains/session/use-workspace-restoration.test.ts \
  lib/state/task-session-reads.test.ts lib/state/shared-resource-reads.test.ts \
  lib/state/workflow-session-focus.test.ts lib/task-agent-error.test.ts \
  components/task/chat/use-session-read-tracking.test.ts \
  components/task/task-unread-dot-visibility.test.ts lib/session-unread-divider.test.ts
pnpm run typecheck
pnpm run build:e2e
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  tests/session/session-primary-removal.spec.ts tests/task/task-unread-dot.spec.ts \
  tests/session/session-tab-management.spec.ts tests/session/session-handoff.spec.ts \
  tests/session/multi-session-ux.spec.ts tests/session/session-recovery.spec.ts \
  -- --workers=1 --retries=0
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```

The existing public recovery guidance (`docs/public/operations.md`) already
promises the failed transcript stays displayed. R08 repairs its selection and
retirement boundaries without a new user action or public interface; the owning
design and this work order record the internal changes.

R08 retained author evidence in `~/.cache/tmp/unread-verification.zWhGjx`:

- `r8-red.log`: original owner/control run, 7 behavioral failures / 8 passes.
  `r8-owner.log`: 5 files / 140 passed after repair.
- `r8-unit-final.log`: 155 affected files / 1644 passed. Typecheck then rejected
  a test-only `started_at` on a lifecycle payload. The ranking fixture now uses
  the real event-admission action instead of inventing a wire field; its final
  16 cases passed (`r8-final-continuation.log`). Production code did not change.
- `r8-typecheck-final.log`, `r8-build.log`: typecheck with the recorded 8 GiB heap
  and fresh E2E frontend build passed. `r8-green-bundle.txt` records asset hashes.
  Configured changed-file lint and formatting passed.
- `r8-desktop.log`: 36 existing desktop cases passed. The new pre-release
  assertion initially imposed a blank desktop surface. Dockview may expose an
  existing tab without app selection: `setupSessionTabSync` rejects activations
  without user intent. Preserve that layout contract. Require the empty surface
  on phone; desktop checks the final proven successor/transcript/no-reload
  outcome. Real-handler tests separately verify selected-ID ownership throughout.
- `r8-i18n.log`, `r8-sleeps.log`, `r8-architecture.log`, `r8-docs.log`:
  i18n ratchet, causal-wait policy, architecture, docs catalog, 36 spec-linter
  tests and all-spec lint passed.

No broad backend/web suite or live NATS certification is added. The frontend is
fresh; backend/plugin artifacts retain the earlier R02 provenance. HTTP absence
proof is convergence evidence, not isolated cursor-event delivery. The historical
incident attribution and seeded-RUNNING/live-execution distinction remain unchanged.

Final viewport receipts: `r8-desktop-continuation.log` passed the corrected new
desktop case (1/1); `r8-phone.log` passed the complete phone file (5/5).
Both used one worker, retries 0 and the same fresh R08 frontend. The desktop
37-case run is not relabelled fully green: retain its 36 passes plus the separate
corrected-case replay. Exact replay:

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  tests/session/session-primary-removal.spec.ts \
  -- --workers=1 --retries=0 --grep 'fills a retired session slot'
```

Inspected the preserved phone images in `r8-phone-artifacts`: failed-resume cause
and recovery controls remain visible; last-session removal leaves no deleted
conversation; delayed promotion shows the surviving stored conversation without
reload. The controlled empty window is phone-only; no new desktop layout policy.

### Production-fix review round 9 remediation

Four accepted source-derived schedules refine existing .6/.8/.9/.11:

- Preserve a displayed STARTING attempt against an older genuine primary
  promotion, whether STARTING comes from the scoped GET or a newer accepted
  cached lifecycle observation. A later healthy accepted lifecycle projection
  can re-prove under the same selection owner, without timers.
- Apply scoped observed-removal exclusions before lifecycle candidate choice
  and at publication. Capture the predecessor even when chronology/recipient
  admission is pending; a removed projected ID is not an unknown valid recipient.
- Capture local deletion ownership before the WS request. A successful
  task-scoped acknowledgement retires that ID through the shared removal
  boundary, without a raw-list selection. Select a successor only from complete
  current membership; preserve newer pins and independent workflow intent.
- Real lifecycle rows omit `started_at`. When an eligible partial row can affect
  ranking, retain predecessor authority until normal membership hydration can
  establish chronology. Never use `updated_at` as a start-time substitute.

Regression owners: `task-primary-focus.test.ts`,
`task-primary-continuation.test.ts`, and
`hooks/domains/session/use-session-actions.removal-proof.test.ts`.
Use deferred external reads/WS acknowledgements and real store/handlers; cover
both arrival orders, newer selection/workflow controls, real partial payloads,
and unchanged survivor cursors. Existing desktop/phone composition and cursor,
unread, backend promotion, transport and persistence contracts remain unchanged.
Run those tests RED before implementation, then the focused affected checks
and fresh-frontend desktop/phone scenarios recorded for this work order.

R09 evidence and test-boundary reconciliation:

- `r9-red.log`: 11 behavioral failures / 70 controls across the real-handler and
  local-action owners, before production changes. Durable STARTING and newer
  cached STARTING displaced A; observe-before-terminal stranded A; local ACK
  selected observed-deleted B; real untimed lifecycle admission selected older C.
- `r9-starting.log`: 55 primary-focus tests passed after startup protection and
  event-driven re-proving. Generic healthy fixtures now default to waiting;
  startup negatives explicitly select STARTING. No synthetic cursor hydration.
- `r9-candidates.log`: 48 candidate/continuation/pure cases passed. R09 partial
  chronology uses the real lifecycle payload without `started_at`, a deferred
  complete membership response and real slice admission. Normal hook loading
  remains activated by the existing partial-row invalidation; no new fetch loop.
- `r9-owner.log`: 107 controls passed; two older positive fixtures had no durable
  start times for competing candidates. The complete-list fixture now has valid
  start times rather than an insertion-order expectation. Explicit partial-row,
  HTTP-order and ID-tie controls remain.
- Local deletion's obsolete raw-list handoff and callback-only/wording tests are
  replaced by real-store ACK, membership-generation, last-conversation and error
  outcomes in `use-session-actions.removal-proof.test.ts`. Keep Quick Chat task
  deletion and restart coverage separate and unchanged.
- `r9-existing-intent-red.log` additionally exposed a workflow already in flight
  before deletion admission. Automatic focus must defer to current task-owned
  intent/request, not just detect a later reference change. Shared-entry controls
  retain explicit committed workflow selection as the positive winner.

The added phone scenario uses the shipped Sessions sheet, touch action menu and
delete confirmation, then requires the surviving row to become current while
the picker stays open. It verifies the local `useSessionActions` entry, not just
an observer of another client. No new layout/control or touch-specific logic.
Its concurrent failure boundary is already RED at the real-handler/hook level;
the browser check is integration/viewport proof after the minimal state fix.

R09 affected-suite/browser commands (same cache-backed working-tree environment):

```bash
cd "$VERIFY/apps/web"
pnpm exec vitest run lib/ws/handlers lib/state/slices/session \
  lib/state/hydration lib/ssr/session-page src/task-detail-route \
  hooks/use-task-sessions hooks/use-task-removal \
  hooks/domains/session/use-session-actions \
  hooks/domains/session/use-workspace-restoration.test.ts \
  lib/state/task-session-reads.test.ts lib/state/shared-resource-reads.test.ts \
  lib/state/workflow-session-focus.test.ts lib/task-agent-error.test.ts \
  components/task/chat/use-session-read-tracking.test.ts \
  components/task/task-unread-dot-visibility.test.ts lib/session-unread-divider.test.ts
pnpm run typecheck
pnpm run build:e2e
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  tests/session/session-primary-removal.spec.ts tests/task/task-unread-dot.spec.ts \
  tests/session/session-tab-management.spec.ts tests/session/session-handoff.spec.ts \
  tests/session/multi-session-ux.spec.ts tests/session/session-recovery.spec.ts \
  -- --workers=1 --retries=0
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```

Frontend must be freshly built after the final production edit. Retained R02
backend/plugin artifacts remain an explicit reuse, not a fresh backend or live
NATS claim. Keep prior broad-suite and historical-attribution limits.

R09 scoped check receipts:

- `r9-explicit-intent-red.log`: all five preceding-workflow entry cases failed
  before the shared precedence guard (primary, terminal, new-session, initial
  adoption and local ACK). Committed workflow reconciliation remains the winner.
- `r9-owner-green.log`: 115 owner/action controls passed before the lint-only
  unavailable-recipient extraction and shared literal constants.
- `r9-unit-final.log`: 155 affected files, 1658 passed / 2 failed. Both failures
  were legacy no-successor fixtures with an incomplete mocked AppState. Those
  spy-only checks were replaced with real-store terminal outcomes, without a
  production fallback for missing `kanban`.
  `r9-agent-session-final.log`: all 80 handler cases then passed.
- Typecheck rejected a test-only null cursor on an optional string field.
  Each unaffected row's cursor was preserved instead; no wire type or
  production cursor code changed. Final continuation/typecheck receipts follow.
- Configured changed-file lint passed after the small complexity/literal
  extractions. `r9-i18n.log`, `r9-sleeps.log`, `r9-architecture.log` and
  `r9-docs.log` passed (catalog, 36 spec-linter tests and all-spec lint included).

These are targeted receipts, not a relabelled all-green 1660-test run or broad
web/backend certification. Source behavior was unchanged by both fixture repairs.

R09 first browser pass and replacement-panel regression:

- `r9-continuation-final.log`: 26 passed. `r9-typecheck-final.log` passed
  with the recorded 8 GiB heap. `r9-build.log` and `r9-green-bundle.txt`
  identify the fresh frontend used by this pass.
- `r9-phone.log`: 6 passed, including the shipped phone picker, row Delete
  action and confirmation. `r9-desktop.log`: 36 passed / 1 failed. The existing
  only-session deletion/workspace-reuse case found a replacement response in
  a mounted but hidden chat; the assertion was not weakened.
- The cache-only observational probe in `r9-replacement-diagnostic.log`
  recorded the replacement selected and pinned in AppState, with no workflow
  intent. Creating it selected its real Dockview panel. The subsequent tab-order
  pass moved the selected tab before Files/Changes and made Changes active.
  `shouldActivate=false` left the replacement hidden because the preceding
  session was null. This is an observed UI ordering failure, not a cursor bug
  or evidence that the original incident recurred.
- `activateSessionPanel` now restores the effective session only if it was the
  active panel before reordering and reordering displaced it. Existing
  non-agent-panel preservation and workflow activation rules are unchanged.
  The existing consumer-visible E2E is the RED regression coverage; no source-text
  or call-count test was added. The diagnostic script observed state only and
  was confined to the disposable cache copy.
- The phone local-delete capture now waits for the successful-delete toast to
  disappear so it does not obscure the surviving selected row.

Final verification uses the normal source overlay (without the diagnostic
script), the Dockview activation/tab/sync/dialog tests, typecheck, a fresh
frontend, and both full selected viewport runs with one worker and no retries.

Final owning-file commands after the fixture repairs and UI correction:

```bash
cd "$VERIFY/apps/web"
pnpm exec vitest run lib/ws/handlers/agent-session.test.ts
pnpm exec vitest run lib/ws/handlers/task-primary-continuation.test.ts
pnpm exec vitest run components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-tab-activation.test.ts \
  components/task/dockview-session-tab-sync.test.ts \
  components/task/new-session-dialog.test.tsx
```

The first two owning-file runs passed before the UI-only correction; the last
run and the fresh frontend/browser results are recorded separately below.

R09 final results:

- `r9-dockview-final.log`: 4 files / 82 tests passed after the UI correction.
  The earlier final owning-file receipts remain 80 handler and 26 continuation
  tests, after their test-only fixture repairs. Do not sum overlapping runs
  or relabel the affected-suite's initial 1658/1660 result.
- `r9-ui-format.log` and `r9-ui-lint.log` passed for the final UI/capture edits.
  `r9-typecheck-ui-final.log` passed with the recorded 8 GiB heap.
  `r9-build-final.log` and `r9-final-bundle.txt` identify the final fresh frontend.
- `r9-phone-final.log`: 6 passed. `r9-desktop-final.log`: 37 passed.
  Both runs used one worker and zero retries against that final frontend.
  The existing replacement-transcript visibility assertion passed unchanged.
- Inspected `desktop-replacement-visible.png`: selected replacement, actual
  response and composer visible beside the preserved workspace.
  Inspected `r9-phone-final-artifacts/**/phone-local-session-removed.png`:
  one surviving selected row, no deleted row or obscuring toast.
  The failed-resume recovery, last-removal empty surface and later-primary
  phone captures were also inspected in the first six-case run.
- Cache-only diagnostic instrumentation was overwritten by the normal actual
  source overlay before final verification. No diagnostic script, app-state
  injection, generated artifact or dependency alias belongs in the commit.

Task 02 implementation is done; the production review loop remains open.
Parent delivery uses the selective commit API with normal hooks, verifies the
exact fork push, then dispatches the next independent Sol review. No broad
web/backend or live NATS certification is implied. Backend/plugin artifacts
remain the retained R02 build. Original incident rows were deleted before live
tracing; historical attribution stays qualified. Task-list readback proves
convergence, not isolated cursor-event delivery; seeded RUNNING is not live
execution continuity. No unread predicate, cursor, backend promotion, wire,
persistence or Go change was introduced in R09.

### Production-fix review round 10 remediation

Three accepted source-derived schedules refine existing .6/.9/.11 and the
failed-session focus design. They do not change unread eligibility or cursors:

- Preview must render the shared selected/empty state and let the scoped
  deletion owner choose a successor. Delete completion is not a user tab click.
- A genuine primary retarget may change predecessor validation, but must retain
  lifecycle recipient eligibility from the original terminal handoff.
- Dockview must carry the previously visible conversation's owned removal
  continuation across its null interval. A proven recipient must replace an
  internal sibling fallback, without overriding independent tab/Plan focus.

```text
Preview:   A -> Delete -> [owned empty, no guessed B] -> proven C
Dockview:  A -> Delete -> [internal B while proof held] -> visible proven C
User:                    [explicit Changes/Plan] ------> stays Changes/Plan
Phone: existing Sessions sheet -> Delete -> shared proof -> selected survivor
```

No new composition, control, touch branch or public contract is planned.
Phone board clicks keep direct task navigation rather than mounting Preview;
the existing six-case phone unread/removal spec covers the shared state path.
Desktop Preview/tab proof needs its own real rendered entry, not a callback
mock. Lifecycle retargeting uses deferred real handler/store observations.
Keep restart, navigation, explicit workflow and non-agent-panel controls.

R10 retarget evidence:

- `r10-retarget-red.log`: two behavioral failures / 35 controls. Both durable
  CANCELLED and COMPLETED predecessors published the now-FAILED recipient after
  its genuine promotion. Both healthy-recipient controls passed.
- `r10-retarget-green.log`: four authority files / 128 tests passed after
  preserving lifecycle recipient eligibility independently of the retarget's
  predecessor predicate. Independent direct-primary following is unchanged;
  authoritative removal membership remains its own admission boundary.
- The first pre-browser typecheck rejected new test-only unbranded row IDs and
  a task-updated message factory used for a session-state frame. The fixture now
  spreads the real stored recipient and emits the real session-state envelope.
  This compile failure is not product RED. Later typed/browser receipts follow.

Preview retains its initial primary/URL discovery only until its first
shared session adoption. Thereafter the store's selected ID or owned null slot
is authoritative; neither render nor delete completion may guess from a list
or feed automatic state back through the user pin/navigation action.

Dockview surface ownership is distinct from the data proof: capture the
visible predecessor before its blank transition, require the matching scoped
removal handoff (including a healthy retarget before paint), and cancel surface
activation on later input/context change. An arbitrary null-to-session update
is not activation authority. Explicit Plan/Changes and committed workflow focus
remain independent.

R10 consumer evidence and controls:

- `r10-consumer-red-summary.log` / `r10-quiet/kandev-run.e2e.lXvYw6YT.log`:
  Preview published the unproven sibling/pin, and the Plan control never reached
  the proven selection. The initial Dockview fixtures booted the recipient
  instead of the deleted conversation; those precondition failures are not RED.
- `r10-consumer-third-summary.log` /
  `r10-quiet/kandev-run.e2e.nx5SQX8d.log`: all three Preview controls and the
  independent Changes control passed. Correctly booting/pinning A before
  promoting C reproduced the remaining surface bug: AppState selected C but
  B stayed visible. This is the Dockview behavioral RED.
- `r10-surface-units-red.log`: a second healthy primary change before paint
  lost the removal surface lease (one failure / eight controls). The proof
  receipt now carries that continuation only through the same scoped
  navigation/workflow owner. Pointer/keyboard input, disposal and unproven
  null-to-session transitions cannot authorize activation.
- `r10-final-typecheck.log`: the new Dockview test's index-only position lacked
  its required reference group. The fixture now uses the real center-group
  position; this compile failure is not product RED.

The Preview consumer cases now live in
`e2e/tests/kanban/preview-session-removal.spec.ts`, rather than the original
`preview-session-tabs.spec.ts` placement used by the retained RED logs.
The newer-workflow control holds authentic deletion membership and a real
workflow-move response; it never creates a synthetic committed recipient.

Current focused commands, from the cache snapshot prepared above:

```bash
cd "$VERIFY/apps/web"
pnpm run typecheck
pnpm exec vitest run \
  lib/ws/handlers/task-primary-*.test.ts \
  lib/ws/handlers/agent-session.test.ts lib/ws/handlers/tasks.test.ts \
  components/task/dockview-session-tab*.test.ts* \
  components/task/dockview-session-removal-focus.test.ts \
  components/task/preview-session-tabs.test.tsx components/task/session-sort.test.ts \
  components/kanban-with-preview*.test.tsx
pnpm run build:e2e
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/kanban/preview-session-removal.spec.ts \
  e2e/tests/session/session-primary-removal.spec.ts -- --workers=1 --retries=0
```

The first configured lint pass (`r10-lint-changed.log`) required splitting the
expanded functions/test group and removing nested test ternaries. The remaining
four-line file-budget excess is recorded in `r10-remaining-lint.log`.
The completed handoff receipt is now owned by
`lib/state/session-removal-handoff.ts`, not a component-to-transport import.
Preview adoption, panel activation and preservation are separated at their
existing responsibility boundaries. No new lint waiver is introduced.

The first consumer GREEN, before these lint-only extractions, was 16 files /
353 tests plus ten Chromium cases (`r10-focused-final.log`,
`r10-consumers-green-summary.log`). The final delivery receipts below supersede
that bundle; overlapping test runs must not be summed.

Final R10 delivery evidence (same prepared cache snapshot):

- `r10-delivery-units.log`: 16 files / 353 tests passed after the extractions.
- `r10-delivery-desktop-summary.log` /
  `r10-quiet/kandev-run.e2e.S6iLS5zf.log`: 32 passed / one failed. All ten
  deletion/Preview consumer cases passed. Moving those cases had also removed
  the original menu spec's still-used `watchWs` import; that is a test setup
  regression, not product RED. Restoring the import is the only later test edit.
- `r10-delivery-rename-summary.log` /
  `r10-quiet/kandev-run.e2e.PVb8r3ux.log`: the repaired original menu case
  passed (one worker, zero retries). Do not describe these separate receipts as
  one fresh 33-case GREEN run.
- `r10-delivery-phone-summary.log` /
  `r10-quiet/kandev-run.e2e.IFBt1b2a.log`: six phone cases passed, one worker,
  zero retries, against the same fresh frontend.
- `r10-delivery-typecheck.log`, `r10-delivery-build.log` and
  `r10-delivery-bundle.txt`: typed 8-GiB check, fresh frontend and artifact hashes.
  Configured changed-file lint, causal-wait lint, i18n ratchet, architecture,
  docs catalog and specification checks passed. The ratchet printed Git rename
  detection warnings but exited zero; no Git rename-limit setting was changed.
- Inspected the retained desktop proven-conversation and Plan captures under
  `r10-delivery-desktop-artifacts`; earlier owned-Changes/workflow captures are
  under `r10-consumer-artifacts`. Seeded conversation fixtures allocate no
  execution. Their workspace-loading placeholders are not runtime-readiness
  evidence.

Related desktop and phone commands, in addition to the focused recipe above:

```bash
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/kanban/preview-session-removal.spec.ts \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/kanban/preview-session-tabs.spec.ts \
  e2e/tests/workflow/workflow-session-focus.spec.ts \
  e2e/tests/session/session-tab-management.spec.ts \
  e2e/tests/session/session-handoff.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- --workers=1 --retries=0
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/kanban/preview-session-tabs.spec.ts -- --workers=1 --retries=0 \
  --grep 'opens the lifecycle menu on right-click and renames through it'
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```

R10 implementation is complete; adversarial production review remains open.
No rebase was repeated. No R10 production Go, wire, cursor, unread predicate,
promotion or persisted schema change was made. Backend/plugin artifacts remain
the retained R02 build. Broad-suite and live-NATS certification, isolated
cursor-event delivery and historical live-incident attribution remain limited
as recorded above. No live database mutation was performed.

### Production-fix review round 11 remediation

Four source-derived P2 schedules are accepted for targeted reproduction under
the existing .6/.9/.11/.17 contract:

- Keep only the removed view's unowned Dockview group blank until explicit or
  proven conversation ownership. Do not change acknowledgement eligibility or
  disable independently visible split chats. A real unread row seeded after
  returning to A, authentic held membership, persisted cursor and mark-read
  traffic prove the boundary; explicit B and independent Changes are controls.
- Merge an accepted predecessor ending into an already-owned primary check's
  recipient eligibility in either delivery order. Retain its projected target
  and durable predecessor predicate. Real-handler reversed FAILED/healthy cases
  extend `task-primary-lifecycle.test.ts`.
- The local delete ACK retains a current same-ID removal view in its captured
  backend/auth/workspace, even if that view was selected after dispatch. It must
  not replace a surviving selection or independent workflow. Deferred real
  `useSessionActions` tests cover event/ACK order; Preview proves the consumer.
- Validate initial Preview route/discovery IDs against authoritative same-task
  membership before adoption or pinning. Missing IDs must fall back to a valid
  primary/unavailable surface, not become a live owned-null interval. Fresh
  deleted/valid non-primary links and the existing held-deletion cases prove
  those distinct paths.

```text
Desktop group A: Delete -> [blank, no implicit B transcript/read] -> proven C
Other split D:           [still visible and readable]
Explicit B:              [select B -> visible and acknowledged]
Preview stale A link:    [validate membership -> C, never pin A]
Phone: Sessions sheet -> existing shared deletion/selection proof
```

The desktop group fence belongs to the existing surface owner, not the unread
predicate. Preview remains desktop-only; phones retain direct task navigation
and the shipped Sessions sheet. No new control, breakpoint, persisted setting,
wire field, backend promotion rule, timer/retry or live-data mutation is planned.
Reproduction receipts (cache-backed snapshot, no source-checkout build output):

- `r11-gap-red-summary.log` / `r11-quiet/kandev-run.e2e.WyTVnqlS.log`:
  both held-gap cursor cases advanced B's persisted cursor to its newly seeded
  unread row. The explicit-B control also exposed that clicking Dockview's
  already-active fallback did not claim data selection. The correction makes
  only the affected group's implicit transcript hidden/detail-inactive while
  preserving its mounted draft/history; real tab input can explicitly claim it.
- `r11-authority-red.log`: promotion-first FAILED-recipient cases and both
  late-ACK current-view orders failed. `r11-authority-green.log`: the minimal
  controller correction passed five files / 145 tests, including healthy,
  surviving-pin and both notification-order controls.
- `r11-discovery-red-summary.log` / `r11-quiet/kandev-run.e2e.mQNj9njv.log`:
  a fresh deleted-session URL failed to show C; a valid non-primary link passed.
  The first unit attempt used an incomplete external WS mock and failed on
  `subscribe`, not behavior. Replacing it with a real disconnected
  `WebSocketClient` and mocking only its request boundary produced the behavioral
  RED in `r11-discovery-unit-red-real-client.log`: one missing-ID failure,
  four existing/valid-link controls passing. No connection was opened.

UI RED used the retained R10 frontend, whose affected UI was unchanged before
those reproductions. The GREEN browser receipts below use a fresh R11 frontend.

The real Preview consumer fixture now uses a complete disconnected WS client,
not an owned-store/router/hook replacement. Its ownership cases use a bounded
15s whole-render budget; individual `waitFor` assertions keep the default
deadline. Intermediate 5s-budget runs timed out in otherwise healthy full-chat
renders (6.3s, then 5.6s/9.0s), not in the invalid-ID assertion. The budgeted
four-file UI run passed 34 tests and the fresh R11 frontend build succeeded.
The prior typecheck passed; final related-file, browser and lint receipts follow.

Fresh-browser receipts:

- `r11-desktop-summary.log` / `r11-quiet/kandev-run.e2e.25rslW9N.log`:
  both complete owning Chromium files passed, 14 cases, one worker, no retries.
  This includes the real deferred delete ACK after selecting A, stale/valid
  initial links, all three held-gap owners, Plan and workflow precedence.
- `r11-phone-summary.log` / `r11-quiet/kandev-run.e2e.A0REZnVa.log`:
  the complete mobile unread-dot file passed, 6 cases, one worker, no retries.
- `r11-oxlint.log`, `r11-e2e-sleeps.log`, `r11-final-typecheck.log`: exit 0.
  Initial Oxlint covered 18 changed web files; the two-fixture lint also covers
  `kanban-with-preview.rendered.test.tsx`. Typecheck uses the cache setup's
  8GiB Node heap. The replaced prop-echo test is documented below.
- Inspected the new held-independent-Changes capture: the removed group's
  conversation is blank while Changes stays visible. The proven-C capture
  displays its exact stored text; the fresh stale-link Preview displays C;
  the phone failed-resume capture retains its failure and recovery controls.
  Captures are under `r11-desktop/` and `r11-phone/`, not the source checkout.

The fresh frontend's bundle hashes are retained in `r11-frontend.sha256`.
R02 backend/plugin artifacts are reused intentionally; no backend production,
wire, cursor writer, unread predicate, or primary promotion code changed.
Broad web/backend suites and a live NATS deployment are not certified green.
Historical incident attribution, seeded-RUNNING, and list-convergence limits
from the earlier results still apply.

Exact related-file gate, from the refreshed cache-backed `$VERIFY/apps/web`
after the existing safe setup (not from committed HEAD alone):

```bash
pnpm run pretypecheck
VITEST_MAX_WORKERS=2 pnpm exec vitest run \
  lib/ws/handlers/task-primary-focus.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-continuation.test.ts \
  lib/ws/handlers/task-primary-removal-admission.test.ts \
  lib/ws/handlers/agent-session.test.ts \
  lib/ws/handlers/tasks.test.ts \
  lib/state/workflow-session-focus.test.ts \
  hooks/domains/session/use-session-actions.removal-proof.test.ts \
  components/task/dockview-session-removal-focus.test.ts \
  components/task/dockview-session-tab-sync.test.ts \
  components/task/dockview-session-tab-activation.test.ts \
  components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx \
  components/task/dockview-panel-content.diff.test.tsx \
  components/task/dockview-panel-content.todos.test.tsx \
  components/task/dockview-desktop-layout.test.ts \
  components/task/session-tab-activation-intent.test.ts \
  components/task/use-session-tab-user-activation.test.tsx \
  components/kanban-with-preview.session-ownership.test.tsx \
  components/kanban-with-preview.rendered.test.tsx \
  components/kanban-with-preview.copilot.test.tsx \
  components/kanban-with-preview.test.ts \
  components/task/preview-session-tabs.test.tsx \
  components/task/chat/use-session-read-tracking.test.ts \
  components/task/task-unread-dot-visibility.test.ts
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/kanban/preview-session-removal.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r11-desktop"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r11-phone"
```

`--no-build` requires the recorded fresh R11 build; it is not permission to use
an older frontend after a production edit.

Final related-check receipts:

- `r11-related.log`: 25 files, 455/456 cases passed. The failure was an obsolete
  fake Preview panel echoing the passed session prop, with no authoritative
  membership fixture. Its unmocked list request hit the unit server's 404.
  It was not treated as a green gate or re-pinned to an unavailable session.
- The prop-echo test and its fake selector were removed. Its rollback behavior
  moved into the existing real Preview consumer fixture: real tabs, shared
  membership and store, visible session chat, task-removal release, and URL
  assertions. The unrelated width test retains its contract with a bounded
  external empty-membership response.
- `r11-related-preview-final.log`: the first real-consumer attempt sent only
  `click`, which does not activate Radix's mouse-down tab trigger. After sending
  the actual mouse-down/up/click sequence, `r11-preview-consumer-final.log`
  passed both owning files / 7 cases. The other 23 related files / 449 cases
  already passed against this production code. This is combined scoped
  evidence, not a fresh single 25-file green run.
- `r11-related-preview-lint.log` and `r11-related-preview-typecheck.log`: exit 0.
  The final mouse-input fixture edit changed no production code or browser
  bundle; the 14 desktop / 6 phone receipts above remain applicable.
- `r11-final-static.log`: final typecheck after the mouse-input fixture change,
  all 19 changed web files' Prettier checks, i18n ratchet, docs catalog and
  specification lint passed. The snapshot ratchet reported rename-detection
  warnings over its older Git baseline; the normal source commit hooks retain
  their own exact staged scope. No Git configuration was changed for this.

R11 remediation is implemented and scoped verification is complete. Delivery
and the next independent production review remain parent-owned. No broad-suite,
live-NATS, historical-trigger, or isolated cursor-event certification is implied.

Normal-hook delivery correction:

- The first selective commit was rejected by `web-lint`, with no bypass.
  Earlier direct Oxlint had exited 0 with warnings; the normal hook denies
  warnings. It found duplicate test literals, one stale unused E2E import,
  and `followSessionTransition` complexity 16 against the limit of 15.
- Named test constants and the unused-import removal retain the assertions.
  Extracting the existing synchronous pending-lifecycle recheck into a private
  helper lowers complexity without changing the removed-owner early return,
  recipient eligibility, cancellation, or authoritative read.
- Earlier 14/6 browser results and related counts are pre-extraction evidence.
  A fresh post-hook frontend and affected-owner revalidation follow; no older
  bundle is silently described as the final one. Normal source hooks must
  succeed before delivery.
- `r11-hook-strict-lint.log` then exposed the controller's 606-line budget
  against 600. Its unchanged pure task-scope predecessor predicate moved into
  the existing `session-selection.ts`; the controller imports it, with no
  alias, new state, or ownership-policy change.
- `r11-hook-strict-lint-final.log`: all 20 changed web files passed with
  `--deny-warnings`. `r11-hook-unit.log`: 8 affected files / 160 cases passed,
  including both notification orders and real Preview rollback. The post-hook
  typecheck and fresh frontend build passed; final hashes are retained in
  `r11-hook-frontend.sha256`. That build differs from the first R11 bundle,
  so the affected desktop/phone failed-resume flows are re-exercised below.
- `r11-hook-desktop-summary.log` / `r11-quiet/kandev-run.e2e.ikG53R0P.log`:
  the final-bundle desktop failed-resume case passed, 1/1.
- `r11-hook-phone-summary.log` / `r11-quiet/kandev-run.e2e.ATIF3Ns6.log`:
  the final-bundle phone failed-resume case passed, 1/1. Both use one worker,
  no retries and the actual stored failure/promotion path. They do not imply
  a second full 14/6 run.

Post-extraction commands, using the same cache-safe setup:

```bash
VITEST_MAX_WORKERS=2 pnpm exec vitest run \
  lib/ws/handlers/task-primary-focus.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-continuation.test.ts \
  lib/ws/handlers/task-primary-removal-admission.test.ts \
  hooks/domains/session/use-session-actions.removal-proof.test.ts \
  components/task/dockview-session-removal-focus.test.ts \
  components/kanban-with-preview.session-ownership.test.tsx \
  components/kanban-with-preview.rendered.test.tsx
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/task/task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --grep 'whose resume fails' \
  --output "$SNAPSHOT/r11-hook-desktop"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --grep 'whose resume fails' \
  --output "$SNAPSHOT/r11-hook-phone"
```

### Production-fix review round 12 remediation

Five source-derived schedules retain the existing .6/.9/.11/.12/.17 contract:
direct remote removal and hidden-primary resolution must not expose or acknowledge
an unowned neighbor; initial explicit Preview discovery and committed workflow
focus must survive pending membership; authoritative ended predecessor reads must
also constrain recipients when lifecycle notification is late or absent.

```text
Remote A removed: G1[A,B] -> G1[blank B], G2[proven C visible]
Hidden proven C:  G1[A,B] -> G1[blank B] -> explicit B -> B visible/read
Preview URL B:    list held + passive A -> validate -> B (or reject stale ID)
Preview move C:   list held -> committed C -> list ready -> C, never C -> null
Ended A GET:     B already FAILED -> retain view -> healthy C, never select B
```

Use existing surface leases and selection ownership, not new cursor predicates,
backend fields, timers or retries. Shared handler changes retain the shipped
phone task route/Sessions sheet; Dockview split/hidden cases and board Preview
remain desktop-only. No layout, breakpoint, touch target or new copy is added.
Controlled real-handler/store and real Preview-consumer regressions precede
production changes. Final disposable browser proof must use a fresh frontend.

Controlled verification:

- `r12-red.log`: 9 behavioral failures / 60 passing controls across the surface,
  real Preview consumer and lifecycle files. Both direct-removal split variants
  exposed the unowned neighbor; hidden-primary resolution chose it; passive
  adoption replaced the valid URL; committed workflow focus became null; all
  four late/omitted terminal-notification cases activated the failed recipient.
- `r12-green.log`: 67 passed / 2 failed. The remaining failures showed that
  reordering an existing recipient briefly activates Files. That programmatic
  fallback must not be mistaken for independent user focus. The existing
  pointer/key-observed cancellation flag now distinguishes it.
- `r12-surface-green.log`: the complete 11-case surface file passed after that
  correction. Direct removal keeps the recipient visible in either initial
  split-focus arrangement; hidden-primary proof retains the unavailable group;
  explicit neighbor selection and independent panel ownership remain controls.
- `r12-related-unit.log`: a fresh 17-file run passed all 373 cases, including
  newer explicit Preview pins, workflow focus with an initial non-primary URL,
  both omitted/late terminal orders and the neighboring selection/removal owners.
- `r12-strict-lint.log`: the first deny-warnings check found five repeated DOM
  attribute literals in the real Preview tests. A named attribute constant
  preserves the assertions; `r12-strict-lint-final.log` passed all ten changed
  web files. `r12-typecheck.log` passed before that test-only constant change.
- An accidentally unscoped Vitest invocation was cancelled. No results from it
  are used; the explicit 17-file run above is the scoped unit receipt.

The authoritative ended read sets the existing recipient eligibility flag only
after predecessor freshness and ownership checks. Direct remote removal acquires
the existing group lease through its proven predecessor receipt, and hidden
recipient handling cannot replace that owner through a cached visible fallback.
Preview validates initial discovery ahead of passive adoption, while preserving
newer explicit pins and adopting committed workflow focus before acknowledging it.
No new store flag, timer, backend field, cursor rule or API is introduced.

`r12-code-static.log` passed all ten code files' Prettier checks, the E2E sleep
ratchet and the i18n ratchet. The snapshot's older Git baseline produced
rename-detection warnings and a larger ratchet scope; no Git configuration was
changed. Normal commit hooks still check the actual staged source changes.
`r12-frontend-build.log` passed; `r12-frontend.sha256` records the fresh asset
tree. The browser commands below reuse the retained R02 backend/plugin binaries,
not an older frontend. A fresh verification checkout must first build those
artifacts through the cache-safe setup above.

Executed R12 scope, from `$VERIFY/apps/web` in the retained cache-backed checkout:

```bash
pnpm run pretypecheck
VITEST_MAX_WORKERS=2 pnpm exec vitest run \
  components/task/dockview-session-removal-focus.test.ts \
  components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-tab-activation.test.ts \
  components/task/dockview-session-tab-sync.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx \
  components/kanban-with-preview.session-ownership.test.tsx \
  components/kanban-with-preview.rendered.test.tsx \
  components/kanban-with-preview.copilot.test.tsx \
  components/kanban-with-preview.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-continuation.test.ts \
  lib/ws/handlers/task-primary-focus.test.ts \
  lib/ws/handlers/task-primary-removal-admission.test.ts \
  lib/ws/handlers/agent-session.test.ts \
  lib/ws/handlers/tasks.test.ts \
  lib/state/workflow-session-focus.test.ts \
  hooks/domains/session/use-session-actions.removal-proof.test.ts
pnpm run build:e2e
export KANDEV_RUN_QUIET_DIR="$SNAPSHOT/r12-quiet"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/kanban/preview-session-removal.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r12-desktop"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/task/task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r12-desktop-unread"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r12-phone"
```

`r12-desktop-summary.log` / `r12-quiet/kandev-run.e2e.62HcnHrl.log`:
both complete owning desktop specs passed, 18/18, one worker and no retries.
The split uses a native tab drag and another browser client's real Delete action;
the unrelated neighbor's persisted cursor and new-tail mark-read traffic stay
unchanged while the proven primary is visible in the other split. Hiding that
primary instead retains the unavailable group and both cursor values until an
explicit neighbor activation acknowledges its exact new tail.

The two new Preview cases use authentic held membership responses, a real
mock-agent launch and a real committed step move. The initial non-primary URL,
destination transcript, URL sync and navigation/selection continuity all passed.
Inspected `remote-split-neighbor-unread.png`, `hidden-primary-owned-empty.png`,
`preview-initial-explicit-over-passive.png` and
`preview-workflow-before-membership.png`: they show the protected split and
hidden-primary surfaces plus the requested and committed Preview transcripts.
No user browser or live database was used.

`r12-desktop-unread-summary.log` /
`r12-quiet/kandev-run.e2e.omXGwYaI.log`: all three original desktop unread cases
passed. `r12-phone-summary.log` / `r12-quiet/kandev-run.e2e.Mi1ejiQj.log`:
all six phone cases passed. Each run used one worker, no retries and the same
fresh R12 frontend; the total is 21 desktop and 6 phone cases across sequential
owning-file runs, not a new broad-suite result.

R12 corrections are implemented and scoped runtime verification is complete.
Normal-hook commit, exact fork push and the next independent review remain
parent-owned. Completed production reviews: 12; consecutive clean: 0.
No broad-suite, live-NATS, historical-trigger or isolated cursor-event
certification is implied. The unread predicate, cursor writers and backend
promotion policy remain unchanged.

`r12-docs-static.log` passed the documentation catalog and full specification
lint. Inspected the final phone failed-resume and last-session-removal captures:
the failed transcript/recovery controls remain visible, and the retired route
has no deleted conversation. These are disposable Chromium observations, not
historical incident or live-NATS verification.

### Production-fix review round 13 remediation

Retain .6/.9/.11/.12/.17 across hidden panel admission and the phone's owned
empty removal slot. Reuse existing hide state and scoped removal receipts;
do not change cursors, unread eligibility, primary promotion or initial discovery.

```text
Desktop remote delete: A -> hidden proven C -> no C panel/read -> explicit reopen C
Phone local delete:    route B, selected A -> ACK [owned blank] -> proven C
                       B stays unread until an explicit picker selection
```

The phone reuses the shipped Sessions pill, picker and Delete confirmation;
the conversation remains the sole scroll owner. No geometry, copy, breakpoint
or touch control changes. Tests hold authentic membership after ACK and dismiss
the picker without selecting a row. Independent splits and explicit choices
remain eligible. Controlled regressions precede production changes.

Reproduction and correction:

- `r13-red.log`: four behavioral failures / 19 passing controls. Direct proof
  rematerialized hidden C, double-rAF recovery recreated C hidden between frames,
  and both owned-null route cases rendered unrelated B.
- `r13-activation-red.log`: both direct hidden-recipient variants failed.
  Isolating the internal activation path showed it selected B even without
  running panel reconciliation; this is separate from C rematerialization.
- `r13-desktop-red-summary.log` /
  `r13-quiet/kandev-run.e2e.4OpQREno.log`: the real second-client Delete recreated
  the hidden C tab. The deleting client uses cursor-read-only Preview so it
  cannot accidentally acknowledge the target while performing the deletion.
- The first phone attempt (`r13-phone-red-summary.log`) stopped at an overlay
  dismissal assertion and is not behavioral RED. Using the supported Escape
  dismissal, `r13-phone-red-escape-summary.log` /
  `r13-quiet/kandev-run.e2e.731yNn9a.log` failed on the persisted B cursor:
  it advanced to B's newly seeded tail after A's ACK while membership was held.
  Picker/delete actions use actual touch input; no row selection dismissed it.
- Panel creation now consults the existing hide state at `ensureSessionPanel`
  and delayed recovery. Implicit activation cannot replace a hidden recipient
  owned by a scoped removal proof; explicit tab input and reopen remain eligible.
- The existing removal receipt can represent a null recipient. Local ACK and
  successor-less retirement record the current scope/navigation/workflow owner
  before publishing that empty slot. Route projection treats it as unresolved,
  not discovery; proven C or newer explicit selection/navigation releases it.
  No new persistent store field, unread filter or cursor suppression is added.
- `r13-green.log`: all four initial owning files / 80 cases passed.
  `r13-strict-lint.log` then found the controller's 606-line budget. Its existing
  pure follow-state classifier and discriminant moved unchanged into
  `session-selection.ts`; no lifecycle policy changed. Final revalidation also
  covers complete zero-primary membership and newer navigation.

Post-extraction scoped checks:

- `r13-related-unit.log`: all 20 files / 403 cases passed, including real route
  recovery, layout selection, initial discovery, zero-primary membership,
  newer navigation and explicit-selection controls.
- `r13-strict-lint-final.log`: all eleven changed web files passed with
  `--deny-warnings`. `r13-typecheck.log` passed. No full-module
  `session-selection` mocks required migration.

Executed from the retained cache-backed `$VERIFY/apps/web` after refresh and
`pnpm run pretypecheck`:

```bash
VITEST_MAX_WORKERS=2 pnpm exec vitest run \
  components/task/dockview-session-removal-focus.test.ts \
  components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-tab-activation.test.ts \
  components/task/dockview-session-tab-sync.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx \
  components/task/task-page-session-ownership.test.tsx \
  components/kanban-with-preview.session-ownership.test.tsx \
  components/kanban-with-preview.rendered.test.tsx \
  components/kanban-with-preview.copilot.test.tsx \
  components/kanban-with-preview.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-continuation.test.ts \
  lib/ws/handlers/task-primary-focus.test.ts \
  lib/ws/handlers/task-primary-removal-admission.test.ts \
  lib/ws/handlers/agent-session.test.ts \
  lib/ws/handlers/tasks.test.ts \
  lib/state/workflow-session-focus.test.ts \
  hooks/domains/session/use-session-actions.removal-proof.test.ts \
  hooks/use-session-layout-state.test.ts \
  src/task-detail-route-recovery.test.tsx
```

`r13-code-static.log` passed Prettier, the E2E sleep ratchet and the i18n ratchet.
As in R12, the snapshot's older Git baseline produces a wider ratchet scope and
rename warnings; no configuration was changed to hide them.
`r13-frontend-build.log` passed, with the fresh asset tree recorded in
`r13-frontend.sha256`. Backend/plugin binaries remain the retained R02 artifacts.

Full owning browser commands on that fresh frontend, one worker and no retries:

```bash
export KANDEV_RUN_QUIET_DIR="$SNAPSHOT/r13-quiet"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/kanban/preview-session-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r13-desktop"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r13-phone"
```

Intermediate browser and visual evidence:

- `r13-desktop-summary.log` / `r13-quiet/kandev-run.e2e.inJTO6N3.log`:
  all 22 desktop cases passed on the first fresh R13 bundle.
- `r13-phone-summary.log` / `r13-quiet/kandev-run.e2e.ZG2M5Tl0.log`:
  6 passed / 1 failed. The persisted B cursor and negative traffic assertions
  passed; the test incorrectly rejected even the neutral no-session chat wrapper.
  It now requires a null store selection and zero visible identified
  conversations, not absence of the shared wrapper.
- `r13-phone-final-summary.log` / `r13-quiet/kandev-run.e2e.9jOA7qfr.log`:
  the complete seven-case phone file passed after that test-only correction.
  `phone-owned-removal-gap.png` shows the neutral Session surface, not B or C;
  releasing authentic membership then shows C, and explicit B reads its new tail.
- Visual inspection of the hidden desktop case exposed another part of .12:
  the sidebar excluded auto-selected hidden C as though it were displayed.
  `r13-hidden-dot-summary.log` / `r13-quiet/kandev-run.e2e.PagX24DK.log`
  proves authoritative unread IDs contain C while its dot is absent. This is
  display ownership, not a missing cursor write or an unread-predicate change.
- The shared dot-display hook now excludes a desktop-hidden session from its
  displayed identity. It observes hide/layout changes and the environment-scoped
  hide set, preserving mobile display and explicit reopen. The reactivity test
  (`r13-dot-reactivity.log`) caught a reused API's environment change before that
  scope subscription was added. Desktop/phone transitions and disposal are controls.

The 22/7 browser receipts above precede this final dot-display correction.
Final fresh-bundle and owning-consumer revalidation follows; delivery is pending.

Final dot-display checks:

- `r13-dot-final-unit.log`: six files / 47 cases passed after the last production
  edit, including every new/changed regression and neighboring dot visibility/
  render-stability contracts. This is separate from the earlier 20-file / 403
  run, not a fresh combined full-suite result.
- `r13-dot-strict-lint.log`: all thirteen changed web files passed deny-warnings
  lint. `r13-dot-typecheck.log` passed.
- The desktop case now also requires C's authoritative unread contribution and
  visible task dot while C is hidden, then an empty contribution and no dot
  after explicit reopen reads C's exact new tail. The pure unread predicate
  and all cursor writers remain unchanged.

`r13-dot-code-static.log` passed final formatting and sleep/i18n ratchets;
the same older-snapshot rename warnings remain qualified.
`r13-dot-frontend-build.log` and `r13-dot-frontend.sha256` identify the second,
final frontend, including displayed-session ownership. Earlier bundle results
are not promoted to final-bundle certification.

Final changed-regression and adjacent-consumer command:

```bash
VITEST_MAX_WORKERS=2 pnpm exec vitest run \
  components/task/dockview-session-removal-focus.test.ts \
  components/task/task-page-session-ownership.test.tsx \
  hooks/domains/sidebar/use-unread-task-dot-display.test.tsx \
  components/task/task-switcher-unread-dot.test.tsx \
  components/task/task-switcher-render-stability.test.tsx \
  components/task/task-unread-dot-visibility.test.ts
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/kanban/preview-session-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r13-final-desktop"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r13-final-phone"
```

Final runtime receipts:

- `r13-final-desktop-summary.log` /
  `r13-quiet/kandev-run.e2e.VRyu7kxk.log`: all 22 cases passed.
- `r13-final-phone-summary.log` /
  `r13-quiet/kandev-run.e2e.PMUXO2RX.log`: all 7 cases passed.
- Both runs used the final dot-display frontend, one worker, no retries and
  retained R02 backend/plugin binaries. Inspected the final hidden-primary
  screenshot: no session panel, and the purple unread dot remains visible.
  The final phone held-gap capture shows the neutral Session surface, with
  no identified B/C conversation; cursor/traffic assertions and explicit
  selection controls passed.
- `r13-docs-static.log`: source documentation catalog and specification lint
  passed. The owning design is 32,749 bytes, within its 32 KiB limit.

R13 corrections and scoped verification are complete. Normal selective commit,
exact fork delivery and the next independent production review are parent-owned.
Completed reviews: 13; consecutive clean: 0. No broad-suite, live-NATS,
historical-trigger, real seeded-execution or isolated cursor-event certification
is implied. No user browser or live database was mutated.

### Production-fix review round 14 remediation

Preserve .6/.9/.11/.12/.17 through passive UI transitions. No new copy, layout
geometry, touch control or persistent state; reuse the existing phone task
picker/workflow move and the desktop right-pane toggle.

```text
Desktop: delete A -> owned blank -> toggle right pane -> still blank
         proof names hidden C -> hide D/toggle -> C hidden, B still unread
Phone:   route B, view A -> begin move -> remote delete A -> owned blank
         move fails -> still blank -> explicit B -> read exactly B's tail
```

R14's three source schedules are reproduced at their real store/consumer owners:

- `r14-red.log`: five failures / 27 controls. One successful-workflow control
  omitted its response entry identity/freshness and was a test fixture failure,
  not a product regression.
- `r14-red-corrected.log`: four behavioral failures / 28 controls. Same-scope
  restoration removes the group block in both null/hidden phases; hiding an
  unrelated tab selects B; cancelling the independent workflow exposes route B.
- Removal surface subscriptions retain leases while the same API/environment
  rebuilds, ignoring transient group absence. Scope/API changes and disposal
  still invalidate them. Restored selection cannot adopt a fallback over a
  proven null or hidden removal owner.
- Hide reconciliation captures the hidden session and navigation owner, checks
  them again in the microtask, and preserves a proven hidden recipient. Ordinary
  active hide and explicit tab input retain their existing controls.
- A null removal receipt survives workflow intent failure/cancellation without
  relaxing target-specific workflow authority. Actual session/navigation or
  backend/auth/workspace change still supersedes it; successful workflow focus
  selects its committed recipient.
- `r14-green.log`: 48 controls passed; the old tab-sync fake omitted its
  navigation revision and threw in the hide callback. The fixture now includes
  the real state field. `r14-green-final.log`: three files / 49 cases passed;
  `r14-typecheck.log` passed before the later E2E-only additions.

Browser RED uses the retained final R13 frontend deliberately, before rebuilding
R14. `r14-desktop-red-summary.log` /
`r14-quiet/kandev-run.e2e.THHe7Ah2.log`: both variants failed. Unrelated hide
exposed a chat; real right-pane toggle advanced B's cursor while proof was held.
The prepared fixture uses a real task environment and no agent prompt.

`r14-phone-red-summary.log` / `r14-quiet/kandev-run.e2e.HccaYhHV.log` then
reproduced the workflow-cancellation case on the R13 bundle. The phone task
picker starts the real move; its HTTP boundary returns a held 500 only after
remote deletion membership settles. The failure toast appears, intent cancels,
and the identified route conversation incorrectly reappears. No store writes
or fabricated membership are used by this test.

Final scoped commands, from the same cache-backed setup after `refresh` and
`pnpm run pretypecheck` (all outputs remain on `/dev/sdd`):

```bash
VITEST_MAX_WORKERS=2 pnpm exec vitest run \
  components/task/dockview-session-removal-focus.test.ts \
  components/task/task-page-session-ownership.test.tsx \
  components/task/dockview-session-tab-sync.test.ts \
  components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-tab-activation.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx \
  lib/ws/handlers/task-primary-continuation.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-focus.test.ts \
  lib/ws/handlers/task-primary-removal-admission.test.ts \
  lib/state/workflow-session-focus.test.ts \
  hooks/domains/session/use-session-actions.removal-proof.test.ts \
  hooks/domains/sidebar/use-unread-task-dot-display.test.tsx \
  hooks/use-session-layout-state.test.ts
pnpm run build:vite
export KANDEV_RUN_QUIET_DIR="$SNAPSHOT/r14-quiet"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/kanban/preview-session-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r14-desktop"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r14-phone"
```

`r14-strict-lint.log` passed all nine changed web files with deny-warnings.
`r14-typecheck-final.log` passed after the phone failure-path test was added.
Formatter changes are applied back through anchored edits, not by copying the
verification tree. The later desktop addition checks authoritative unread IDs
and the actual dot as well as the cursor/negative traffic.

`r14-related-unit.log`: all fourteen files / 279 cases passed, including every
new/changed regression, ordinary restored selection/active hide, independently
visible splits, lifecycle/removal continuation, and workflow/pin/navigation
controls. `r14-frontend-build.log` passed; `r14-frontend.sha256` records its
fresh asset tree. Backend/plugin artifacts remain the retained R02 build.

`r14-code-static.log` passed formatting, sleep and i18n ratchets. The snapshot's
older Git baseline still broadens the ratchet scope and emits rename warnings;
no configuration change suppresses them. `r14-docs-static.log` passed catalog
validation and full spec lint. The owning design is 32,747 bytes.

`r14-desktop-summary.log` / `r14-quiet/kandev-run.e2e.6LAbft9i.log`:
all 23 desktop cases passed on the fresh R14 frontend, one worker/no retries.
The same-environment case toggles the real right pane before and after proof;
both variants hide unrelated D and retain B's exact cursor, negative mark-read
traffic, authoritative unread contribution and actual dot. Explicit B selection
then reads exactly its new tail. Inspected both final desktop captures: the
removed group is blank with its unread neighbor still a tab, and the task dot
remains visible. Files/Terminal placeholders are prepared fixture state, not
evidence of a live agent.

`r14-phone-summary.log` / `r14-quiet/kandev-run.e2e.505Ag26E.log`:
all eight phone cases passed on the same fresh frontend, one worker/no retries.
The real failed-move path leaves selection null, shows no identified route
conversation, preserves B's exact cursor and sends no B-tail mark-read. Explicit
B selection then advances exactly to that tail. Inspected the final cancelled
workflow capture: neutral Session surface with the real failure toast, not B.

R14 correction and scoped verification are complete. At this execution
checkpoint the parent proceeds to normal selective commit, exact Fclem fork
delivery and independent Sol R15 dispatch. No repeated rebase, PR, broad-suite
or live-NATS certification. Namespace commit defaults run normal hooks without
bypass; individual hook streams are not exposed by that API. Temporary source
dependency aliases are cache-backed and removed before push. Original-incident
attribution, list-convergence and seeded-execution limits remain unchanged.
Completed production reviews: 14; consecutive clean: 0.

### Production-fix review round 15 remediation

Reuse .6/.9/.11/.12/.17 and existing desktop/phone composition. No predicate,
cursor, backend promotion, wire or schema change.

```text
Desktop: owned blank -> maximize Files / restore -> still blank
         hidden C -> workflow fails / restore -> still C hidden, B unread
         independent Plan -> proven inactive C -> C dot stays until actual read
Phone:   failed move leaves blank -> unseen sibling cannot adopt
         changed primary -> scoped absence/unique-primary proof -> recipient
```

`r15-red.log` recorded five failures: four behavior failures (two parked-group
phases, inactive-recipient dot, and unseen-sibling admission), plus one mock
scaffold error because the new visible-panel fake lacked `removePanel`.
The fake was completed rather than adding a production fallback.
`r15-green.log` and initial typecheck passed after the scoped corrections.

Passive receipts retain selection/navigation/scope ownership through workflow
failure; strict workflow authority for automatic focus remains in the controller.
Group retirement checks parked pre-maximize membership before removing a lease.
Existing scoped null receipts seed changed-primary removal verification and block
state-only discovery. Desktop dot exclusion observes the actual panel visibility,
rebinding its visibility subscription on layout replacement, while phone ignores
desktop layout. Explicit selection and committed workflow controls remain.

Browser cases extend the real right-pane case with maximize/restore and an
injected move failure, add independent Plan's exact cursor/dot assertions, and
extend the real phone cancellation case with a newly proven primary. Existing
negative cursor/traffic checks and explicit read controls are retained.

`r15-unit.log` passed ten files / 211 cases. Strict lint then found the
controller's 605-line budget. Two existing pure predicates moved unchanged
into `session-selection.ts`; no full-module mocks needed migration. Final
post-move unit/lint/typecheck/build and browser receipts follow.

`r15-static.log` and `r15-docs.log` passed sleep/i18n ratchets, documentation
catalog and full spec lint. Ratchet rename warnings remain attributable to the
older verification checkout baseline, not a configuration change. All temporary
outputs and dependency targets stay cache-backed. R02 backend/plugin artifacts
are reused intentionally; runtime proof uses the fresh R15 frontend.

Post-move `r15-unit-final.log` passed ten files / 211 cases; strict changed-file
lint, typecheck and fresh frontend build passed. The first full three-file
desktop run (`r15-desktop-summary.log`, raw `r15-quiet/kandev-run.e2e.JAfyvvuX.log`)
had 22 passes and two scaffold failures: Plan was not an existing default tab,
and the maximize control's Files group had just been hidden by the pane toggle.
The tests now open Plan through the shipped + menu and restore the right pane
before maximizing Files. No production change was made for these failures.
The corrected owning cases and full phone file are checked separately below;
do not describe the earlier complete run as 24 green.

The corrected Plan case passed (`r15-desktop-corrected-summary.log`, raw
`r15-quiet/kandev-run.e2e.GEgI1Zc0.log`). The remaining maximize case exposed
another scaffold error: a Files body locator disappears with null task/session
ownership even though its header remains. It now uses the existing
`SessionPage.clickMaximize("Files")` header path for both maximize and restore.

Final owning unit command (all changed regressions included):

```bash
VITEST_MAX_WORKERS=2 pnpm exec vitest run \
  components/task/dockview-session-removal-focus.test.ts \
  lib/ws/handlers/task-primary-continuation.test.ts \
  hooks/domains/sidebar/use-unread-task-dot-display.test.tsx \
  components/task/dockview-session-tab-sync.test.ts \
  components/task/task-page-session-ownership.test.tsx \
  lib/ws/handlers/task-primary-lifecycle.test.ts \
  lib/ws/handlers/task-primary-focus.test.ts \
  lib/state/workflow-session-focus.test.ts \
  components/task/task-switcher-unread-dot.test.tsx \
  components/task/task-switcher-render-stability.test.tsx
```

Desktop uses the same three owning files and cache-safe host/single-shard runner
as R14, with output `$SNAPSHOT/r15-desktop`. After test-only corrections:

```bash
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts -- \
  --workers=1 --retries=0 --grep 'independent-plan|layout restore: true' \
  --output "$SNAPSHOT/r15-desktop-corrected"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts -- \
  --workers=1 --retries=0 --grep 'layout restore: true' \
  --output "$SNAPSHOT/r15-desktop-maximize"
"$VERIFY/scripts/run-quiet" e2e --summary -- pnpm e2e:run \
  --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- \
  --workers=1 --retries=0 --output "$SNAPSHOT/r15-phone"
```

The prepared maximize case required the real full-width stepper header (not
Preview's minimal disclosure), and a destination different from the task's
current prepared step. Once those scaffold assumptions were corrected,
`r15-desktop-header-summary.log` / raw `r15-quiet/kandev-run.e2e.89ChH6KD.log`
exposed a real persisted B-cursor advance. Group bookkeeping alone did not cover
the render interval where the returning panel is absent from the live API.
`r15-remount-red.log` reproduces that read-admission gap in both ownership phases.
The existing group lease now retains its original panel identities so the
remount remains blocked without suppressing unrelated groups or explicit input.

`r15-phone-summary.log` / raw `r15-quiet/kandev-run.e2e.GJjMrVfL.log` passed all
eight phone cases before this final desktop-remount correction. The failed
workflow stays blank, a changed primary then re-proves membership and displays
its exact new tail, while B remains unread until explicitly selected. Final
fresh-bundle receipts are recorded separately.

`r15-remount-unit.log` passed all ten owning/adjacent files after the final
production edit. `r15-remount-typecheck.log`, `r15-remount-build.log`,
`r15-format-check.log` and `r15-remount-lint.log` passed. The real combined
maximize/failed-workflow case then passed:
`r15-desktop-remount-summary.log` / `r15-quiet/kandev-run.e2e.r13NrXfl.log`.
Inspected its capture: hidden C stays closed, B remains a tab but has no visible
transcript, the failed move's real error is shown, and the task dot remains.
The separate Plan capture shows the chosen Plan surface, not C's transcript.

Final complete desktop and phone files run serially on that same final bundle,
one worker/no retries, with outputs `$SNAPSHOT/r15-final-desktop` and
`$SNAPSHOT/r15-final-phone`; these replace earlier bundle claims, not the
retained RED/scaffold receipts.

Final runtime receipts on the final remount-aware frontend:

- `r15-final-desktop-summary.log` /
  `r15-quiet/kandev-run.e2e.5WwNzhO3.log`: all 24 desktop cases passed.
- `r15-final-phone-summary.log` /
  `r15-quiet/kandev-run.e2e.41Vspas0.log`: all 8 phone cases passed.
- `r15-remount-unit.log`: all ten files / 211 cases passed after the last
  production edit. Final strict lint, typecheck, formatting and build passed.
  Backend/plugin reuse and historical/list-convergence limits remain qualified.

Implementation and targeted verification are complete. At this execution
checkpoint the parent proceeds to normal selective commit and exact fork push,
then independent Sol R16 review. No repeated rebase or PR. Individual hook
streams are unavailable from the selective commit API; no bypass is used.
Completed production reviews: 15; consecutive clean: 0.

### Production-fix review round 16 remediation

Preserve .6/.9/.11/.12/.17 across maximize-before-removal, hide admission, empty
continuation, and terminal passthrough. Reuse existing desktop controls, phone
visibility, and Preview's no-mark-read policy. No new copy, geometry, cursor,
predicate, backend promotion, wire or schema contract.

```text
Desktop: maximize independent group -> remote delete A -> capture parked A
         prove C -> restore -> unproven B remains unread; hidden C stays hidden
Empty:   workflow fails -> unchanged/omitted C update -> still null, no new read
         genuinely changed D -> authoritative membership proof -> D
Terminal passthrough: blocked B receives tail -> cursor unchanged
         explicit B -> exact tail; Preview's later tail remains unread
```

`r16-red.log`: three behavioral failures / 44 controls: no parked predecessor
lease, hidden C inserted into the saved group, and a repeated C projection
starting another membership read. `r16-passthrough-red-summary.log` /
`r16-quiet/kandev-run.e2e.0W9BdRtp.log`: a real terminal passthrough B advanced
its cursor on a new stored tail while replacement membership was held. This
uses the retained R15 bundle deliberately for RED, not a source-text assertion.

The group owner can capture its active predecessor and members from the parked
snapshot before reconciliation trims it. Session hiding filters reconciliation
and the final saved-layout restore. Receipt-only continuation now requires an
actual accepted primary identity change; existing pending owners keep their
own projection rules. Passthrough recovery forwards host visibility/detail/
panel identity into TaskChatPanel, and Preview explicitly keeps `isVisible=false`.
Ordinary desktop, shared Dockview and phone callers propagate their own admission.

The owning unit/toolbar suites and initial typecheck passed in `r16-green.log`
and `r16-typecheck.log`. Browser coverage reverses the real split-removal order,
keeps visible and hidden recipient variants, and checks terminal passthrough
negative traffic/persisted cursors plus normal explicit-view and Preview controls.

The first fresh R16 browser run (`r16-owning-desktop-summary.log`,
`r16-quiet/kandev-run.e2e.eNlaTEjQ.log`) passed the live remote split and
terminal-passthrough/Preview cases, but was not green: the parked visible case
advanced B's cursor; the hidden fixture could not reach C's clipped close button.
Maximization had legitimately adopted the other split's C globally before
background A was removed. Group-local A still needed a lease. The new
background-parked real-store control reproduced that gap (one failure /
20 controls in `r16-background-red.log`). Confirmed retirement now captures
the active live/parked group independently of global session selection.
`r16-background-green.log` passes that control. The hidden fixture keeps C in
the wide center group, hides it through its real close control, then maximizes
the independent Files group before deletion. Its no-rematerialization, exact
cursor/traffic, dot and explicit-reopen assertions remain unchanged.

Final R16 commands from the mount-checked cache snapshot, after `refresh` and
`pnpm run pretypecheck` (`TMPDIR`/`GOTMPDIR` remain cache-backed):

```bash
cd "$VERIFY/apps/web"
pnpm exec vitest run \
  hooks/use-panel-active.test.ts \
  components/task/dockview-session-removal-focus.test.ts \
  lib/ws/handlers/task-primary-continuation.test.ts \
  components/task/passthrough-toolbar.test.tsx \
  lib/state/dockview-env-switch-maximize-session-filter.test.ts \
  components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx \
  components/task/dockview-shared.test.tsx \
  components/task/preview-session-tabs.test.tsx
pnpm run typecheck
pnpm run build:vite
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- --workers=1 --retries=0 \
  --output "$SNAPSHOT/r16-final-desktop"
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0 \
  --output "$SNAPSHOT/r16-final-phone"
```

Browser output is retained through `scripts/run-quiet`, serially and without
retries. This run rebuilds the frontend; the existing R02 backend/plugin
artifacts are retained. It is not a broad-suite or live-NATS certification.

Phase-local cursor assertions in `r16-parked-phases-summary.log` /
`r16-quiet/kandev-run.e2e.sqxCLwGS.log` isolated an additional boundary:
both parked variants had already read B during maximization, before deletion.
The background lease correction alone therefore did not make the browser cases
green. `r16-restore-admission-red.log` reproduces transient visible admission
in the shared panel hook (one failure / six controls). `usePanelActive` now
withholds read/detail visibility during layout reconstruction, then resumes the
actual selected tab's visibility. Ordinary stable independent splits remain
visible. The parked tests retain checks before deletion, before restoration,
and after restoration so these phases cannot mask one another.

The reconstruction-only gate passed 175 unit controls and strict checks but
did not fix the browser read (`r16-restore-parked-summary.log`). Read-only
traffic in `r16-parked-stimulus-summary.log` /
`r16-quiet/kandev-run.e2e.KfRFdDB2.log` confirms B's newly seeded tail, not
another delayed old acknowledgement, was read before deletion. Parked portals
can retain disposed panel APIs that still report visibility after restoration
ends. `r16-parked-portal-red.log` reproduces that stale visibility after the
restore flag clears; the shared hook now also requires the portal API to be
the current Dockview panel's API. `r16-parked-portal-green.log` covers parked
denial and later legitimate remount admission. Temporary diagnostic logging
was removed; the phase-specific cursor assertions remain.

Final receipts on the current portal-identity-aware frontend:

- `r16-portal-unit.log`: nine files / 176 cases passed.
- `r16-portal-lint.log`, `r16-portal-typecheck.log` and
  `r16-portal-build.log`: strict changed-file lint, full web typecheck and
  fresh frontend build passed. Final formatting and sleep/copy ratchets passed
  (`r16-final-{format,sleeps,i18n}.log`).
- `r16-portal-parked-summary.log` /
  `r16-quiet/kandev-run.e2e.6OPFq5bX.log`: both previously leaking parked
  variants passed, including each pre-deletion cursor assertion.
- `r16-final-desktop-summary.log` /
  `r16-quiet/kandev-run.e2e.TZ3Ky5hX.log`: all 18 cases in the complete
  desktop removal and unread-dot files passed.
- `r16-final-phone-summary.log` /
  `r16-quiet/kandev-run.e2e.IDdCLrE5.log`: all eight phone cases passed,
  after the desktop run, on the same frontend. Both runs used one worker and
  no retries. No production edits followed this build.

Inspected saved desktop parked-visible/hidden captures: B's group stays empty,
the independent visible C split remains available, hidden C has no tab, and
the unread task dot remains. Inspected final phone failed-resume and owned-gap
captures: recovery actions remain visible for the failed session; the owned
empty interval does not expose the route sibling. These are disposable managed
browser receipts, not user-browser or live-database mutations.

R16 corrections and targeted verification are complete. The parent proceeds
to normal selective commit and exact fork push, then separate Sol R17 review.
No repeated rebase, PR, hook bypass, broad-suite or live-NATS claim. Original
incident attribution, retained backend/plugin and authoritative-list convergence
limits above remain unchanged. Completed production reviews: 16; consecutive
clean: 0.

Final catalog validation and full specification lint passed
(`r16-final-docs.log`). An additional full-file Markdown Prettier check reports
style warnings in the three touched documents (`r16-final-doc-format.log`);
no broad prose/table reflow was applied. Changed-code formatting above passed.

The first normal commit attempt rejected a duplicate literal in the new hook
test. Extracting its local panel ID then exposed the test container's existing
100-line budget. Grouping restoration/remount cases separately and sharing
cleanup resolves both without suppressions. Final hook regression: eight
cases passed (`r16-hook-unit.log`); all 14 changed code/test files passed
zero-warning Oxlint (`r16-hook-lint.log`). These are test-only corrections;
the final browser bundle and production code above are unchanged.

### Production-fix review round 17 remediation

R17 reports four P2 ownership gaps and one P3 design inconsistency. Preserve
.6/.9/.11/.12/.17 with no new copy, geometry, cursor/predicate, backend, wire
or schema change. Desktop reuses existing group-header maximize, tab hide and
explicit reopen. Phone keeps the shipped picker, failed transcript and owned
empty composition; creation-event continuation shares the same state policy.

```text
Desktop: begin rebuild -> retire A -> keep original group/read lease
         restore -> unproven B stays unread; explicit B reads exact tail
         maximize A's own group -> retire -> restore -> same lease
         hide B after maximize (no environment) -> restore keeps B hidden
Empty:   cancelled workflow -> delayed creation of cached C -> still empty
         genuinely changed D -> membership proof -> D
Dot:     selected but not displayed -> remains unread/visible under .17
```

`r17-red.log`: four behavioral failures / 54 controls. Reconstruction skipped
the lease capture; serialized maximize used `group-1` instead of `group-center`;
the restored layout contained visit-locally hidden B; creation issued a second
membership GET for the cancelled empty slot. These are real store/handler and
serializer/hide-boundary tests, not source-text or wiring assertions.

Capture is now independent of the prune suspension during reconstruction.
Maximize preserves its logical group ID. The live hide registry moved intact
to the existing environment-hide module so restore and panel reconciliation
consult the same API/environment-scoped set, including the null-environment
visit. No second hide convention or storage identity was added. Task creation
may retarget an existing controller but cannot start one from a receipt-only
empty selection. The owning dot paragraph now distinguishes group-visible
desktop chat from phone selection and selected-but-undisplayed unread output.

`r17-green.log`: seven files / 172 cases passed, including pending-creation
retarget and shared-hide adjacent consumers. New rendered cases cover removal
during an authentically gated native layout frame, removal in the maximized
chat group itself, and a hide made after an environment-less snapshot is saved.
The frame helper delays native callbacks only after A leaves the live API,
never writes application state or fabricates membership, and restores/flushes
callbacks in `finally`. Each case checks exact persisted cursors/traffic and
explicit-selection/reopen positive controls.

Initial final-check attempt passed the seven owning files but strict lint
rejected handler complexity and two 600-line budgets (`r17-lint.log`).
The passive-projection predicate is now private, and the unchanged readiness
inheritance helper moved to the existing session-selection module. Existing
deferred recipient-ready/error/execution controls remain the behavioral owner.
The new maximize identity regression lives in
`lib/state/dockview-maximize-ownership.test.ts` because the existing maximize
filter suite reached its file budget. No limits were raised or suppressed.

Final narrow unit command from the mount-checked snapshot, after `refresh` and
`pnpm run pretypecheck`:

```bash
cd "$VERIFY/apps/web"
pnpm exec vitest run \
  components/task/dockview-session-removal-focus.test.ts \
  lib/state/dockview-env-switch-maximize-session-filter.test.ts \
  lib/ws/handlers/task-primary-continuation.test.ts \
  lib/ws/handlers/task-primary-focus.test.ts \
  components/task/dockview-hidden-session-panels.test.ts \
  lib/state/dockview-env-switch.test.ts \
  lib/state/dockview-preset-persistence.test.ts \
  lib/state/dockview-maximize-ownership.test.ts \
  lib/ws/handlers/task-primary-lifecycle.test.ts
pnpm run typecheck
pnpm run build:vite
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts -- \
  --grep 'parked: (overlay|rebuilding)|visit-local hide' --workers=1 --retries=0 \
  --output "$SNAPSHOT/r17-owning-desktop"
```

Runtime verification uses a freshly built frontend and intentionally retained
R02 backend/plugin artifacts; no backend production code changed. All
browser runs remain headless/disposable, single-worker and retry-free.

The moved test initially lacked `hasMaximizedGroup` in its API fake
(`r17-unit-delivery.log`: 220 passes / one scaffold failure). Restoring the
existing maximize fixture's capabilities produced nine files / 221 passes
(`r17-unit-final2.log`). Typecheck then required narrowing serialized `data`
with `Array.isArray`, not its non-discriminating `type` field. The final
identity test passes (`r17-identity-unit.log`: one case); zero-warning lint,
typecheck and the fresh build pass in `r17-{lint,typecheck,build}-final.log`.
Only that test traversal changed after the nine-file run; production did not.

`r17-owning-desktop-summary.log` /
`r17-quiet/kandev-run.e2e.j75tTXB2.log`: all three new owning browser cases
passed on that frontend. Inspected their captures: the restored unproven
neighbor's chat stays empty with its unread dot; hidden C stays absent;
visit-locally hidden B has no tab until explicit reopen. Persisted exact-tail
and negative traffic assertions, not the images alone, prove read admission.
Final changed-code formatting and copy/sleep ratchets passed
(`r17-final-{format,i18n,sleeps}.log`).

Final complete-file runtime checks on the same fresh frontend, run serially:

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- --workers=1 --retries=0 \
  --output "$SNAPSHOT/r17-final-desktop"
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0 \
  --output "$SNAPSHOT/r17-final-phone"
```

- `r17-final-desktop-summary.log` /
  `r17-quiet/kandev-run.e2e.w7iFYFOh.log`: all 21 desktop cases passed.
- `r17-final-phone-summary.log` /
  `r17-quiet/kandev-run.e2e.Il1nDAeH.log`: all eight phone cases passed.
- `r17-docs.log`: catalog validation and full specification lint passed.

No production edits followed that build. The final nine-file unit receipt
and later one-case type-only correction are separate receipts, not a new
combined run. R17 corrections and scoped verification are complete; the parent
proceeds to normal selective commit, exact fork push and separate Sol R18
review. No repeated rebase, new PR, broad-suite/live-NATS certificate or change
to historical incident/list-convergence/seeded-execution qualifications.
Completed production reviews: 17; consecutive clean: 0.

### Production-fix review round 18 remediation

R18 adds two source-derived schedules under the same .6/.11/.12/.17 and layout
restoration contract. No new copy or phone composition: the existing phone
picker/transcript path is unchanged; the changes concern desktop Dockview
restoration and shared session ownership.

```text
Desktop: A displayed, B first, C inactive in the same group
         maximize Files -> remote Delete A -> prove C -> restore
         show/read C's exact tail; B remains unread until explicit selection
Geometry: manual right width -> maximize terminal -> refresh/return
          terminal fills available width; exit restores ordinary right width
```

`r18-unit-red.log`: the verified canonical-terminal overlay is resized as the
ordinary right pane (one failure / 14 controls). `r18-red-ui-summary.log` /
`r18-quiet/kandev-run.e2e.Sy7yvHRY.log`: the real parked inactive-C scenario
has no C transcript after exit. Both are behavioral RED, not missing setup.

The auto-session effect now waits for reconstruction to settle and re-runs at
that boundary, retaining the undelivered removal receipt and explicit/non-agent
focus rules. Both persisted-overlay readers pass their verified group identity
into fixups; only a matching single non-sidebar overlay skips ordinary right
constraints, while sidebar handling and normal right-width restoration remain.
The restored maximize identity comes from the verified overlay, not a synthetic
center fallback. Tests strengthen refresh/environment-return geometry with a
pre-existing manual right width and preserve terminal input/scrollback controls.

`r18-unit-green.log`: seven files / 121 cases passed after the production
corrections. Initial strict lint rejected the expanded scenario's complexity;
the same visible-primary assertions moved into a local test helper, with no
suppression (`r18-lint-final.log`). Full typecheck and fresh frontend build
passed (`r18-typecheck.log`, `r18-build.log`).

The first three-case browser run (`r18-owning-summary.log`,
`r18-quiet/kandev-run.e2e.uijuJHvH.log`) passed the formerly blank C transcript.
Its two geometry failures were fixture errors: the actual default panel ID is
`terminal-default`, not `terminal`. Corrected the read-only measurements and
normal-width control; no production code changed. The environment-return case
now uses real sidebar navigation and checks unchanged `performance.timeOrigin`,
so it exercises the store's same-page restore rather than another full reload.

Focused unit command in the mount-checked snapshot after `refresh` and
`pnpm run pretypecheck`:

```bash
pnpm exec vitest run \
  lib/state/dockview-layout-builders-fixups.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx \
  components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-removal-focus.test.ts \
  components/task/dockview-layout-restore-maximize-session.test.ts \
  components/task/dockview-layout-restore.test.ts \
  lib/state/dockview-env-switch-maximize-session-filter.test.ts
```

All runtime checks use the fresh R18 frontend with the explicitly retained R02
backend/plugin artifacts. No Go production change, live-user mutation,
historical-trigger proof, broad-suite or live-NATS certification is implied.

Final serial runtime commands (one worker, no retries, same fresh frontend):

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-layout.spec.ts -- \
  --grep 'maximize survives page refresh|task switching preserves maximize' \
  --workers=1 --retries=0 --output "$SNAPSHOT/r18-geometry-final"
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- --workers=1 --retries=0 \
  --output "$SNAPSHOT/r18-desktop-final"
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0 \
  --output "$SNAPSHOT/r18-phone-final"
```

- `r18-geometry-final-summary.log` /
  `r18-quiet/kandev-run.e2e.TbFYBF1Q.log`: both geometry cases passed.
- `r18-desktop-final-summary.log` /
  `r18-quiet/kandev-run.e2e.M7zgnkxo.log`: all 22 desktop cases passed.
- `r18-phone-final-summary.log` /
  `r18-quiet/kandev-run.e2e.xnRQytO1.log`: all eight phone cases passed.
- `r18-static-final.log`, `r18-docs-final.log`: changed-code formatting,
  copy/sleep ratchets, documentation catalog and full spec lint passed.

Inspected the same-bundle initial captures: C is actually displayed in the
restored group while B keeps its unread contribution; the refreshed terminal
fills the workbench. Final geometry assertions use the corrected real panel ID,
prove both persisted readers, and retain normal-width/input controls.
No production edits followed that build. R18 corrections and targeted
verification are complete; normal selective commit/exact fork push and separate
Sol R19 review follow. No repeated rebase, PR, hook bypass, broad-suite or
live-NATS claim. Earlier historical-trigger, list-convergence and
seeded-execution qualifications remain. Completed reviews: 18; clean streak: 0.

### Production-fix review round 19 remediation

R19 reproduces two consumer-visible failures under .6/.9/.11/.17 and the
existing workflow/navigation ownership contract. No new layout, copy, schema,
cursor or backend behavior. Desktop retains the committed stepper destination;
phone shares current-route membership recovery with its existing direct task
surface. Preview remains read-only and uses its real full-workbench button.

```text
Restore: visible A -> gate reconstruction -> commit workflow C -> settle
         C stays selected, becomes visible, and its focus request is consumed
Route:   Preview A -> open workbench -> hold initial [A,C] membership
         remote Delete A -> prove C -> retire old list -> replace subread
         hydrate C without reload; stale A never becomes current membership
```

`r19-red.log` reproduces the current-route AbortError (plus an initial incomplete
Dockview event fake). `r19-workflow-red.log` reproduces the wrong group-active
session B instead of the committed A, with three controls passing; the first
cleanup used the wrong disposable shape and was corrected. Final assertions
cover actual selection, visible panel and consumed workflow request rather
than an incidental number of Dockview activation callbacks.

Passive restore adoption now defers to a same-task committed workflow request.
The retained membership reader captures its shared-key generation and replaces
only an AbortError accompanied by a changed generation while route/scope
remain current. Genuine cancellation, generic errors and unchanged generations
still leave the loop; no timer, retry policy or stale list publication was added.
The reader lives beside the existing membership owner to respect the navigation
file's size budget. Test fixtures moved outside oversized test containers;
limits were not raised. The old sync fake now includes the real workflow slice.

`r19-unit-delivery.log`: six files / 57 cases passed, including route scope,
retry-budget and existing route-recovery controls. Initial strict lint caught
the size budgets and an obsolete import after extraction; those were corrected.
The new browser case uses genuine stored sessions, real Preview-to-route
navigation and second-client Delete, holding only authentic pre-deletion
membership so the independent absence proof can complete.

The first owning browser attempt (`r19-owning-summary.log`,
`r19-quiet/kandev-run.e2e.S5vb608V.log`) selected the newer eligible third
sibling, not the fixture's named C. That was legitimate backend promotion,
not a failed recovery. The corrected fixture leaves C as the sole eligible
successor by completing the unrelated sibling before navigation. It also
asserts A's cursor remains null while initial membership is held, preventing
an already-hydrated route from masquerading as this scenario.

`r19-owning2-summary.log` /
`r19-quiet/kandev-run.e2e.qrHQJmK6.log`: the corrected case passed. Inspected
its saved capture: the real workbench displays C's stored transcript and
composer. Its cursor advances after proof without a document reload.
Strict changed-code lint, full typecheck and fresh build passed after removing
the obsolete import (`r19-lint-final.log`, `r19-typecheck.log`, `r19-build.log`).
Formatting, copy/sleep ratchets and catalog/full-spec lint passed in
`r19-static.log` and `r19-docs.log`.

Narrow unit command after cache-snapshot `refresh` and `pnpm run pretypecheck`:

```bash
pnpm exec vitest run \
  components/task/dockview-session-tabs.hook.test.tsx \
  components/task/dockview-session-tab-sync.test.ts \
  lib/state/task-navigation-reads.test.ts \
  lib/state/task-navigation-reads-retry.test.ts \
  lib/state/task-navigation-reads-budget.test.ts \
  src/task-detail-route-recovery.test.tsx
```

Runtime uses the freshly built R19 frontend and retained R02 backend/plugin
artifacts, with one worker and no retries. No backend production, predicate,
projector, cursor-writer, wire or schema change.

Final rendered verification, serial managed Chromium, one worker/no retries:

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- --workers=1 --retries=0
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```

- `r19-desktop-final-summary.log` /
  `r19-quiet/kandev-run.e2e.mm6AKNIE.log`: all 23 cases passed.
- `r19-phone-final-summary.log` /
  `r19-quiet/kandev-run.e2e.RvDWdDKP.log`: all eight cases passed.

These commands used the documented cache-backed snapshot, explicit
cache-backed output locations and the existing R19 build. No live-user
browser/data mutation, broad-suite or live-NATS certification. Authoritative
list convergence is not isolated cursor-event delivery evidence; seeded
RUNNING rows are not live executions. Historical incident attribution
remains qualified. Round 19 corrections are ready for selective delivery
and independent Sol round 20; the review loop is not complete.

### Production-fix review round 20 remediation

Sol child `58c572af-0f9e-49e9-b69c-86c375356f06` reviewed `fabcfa652f`
and reported a stale DTO/fresh membership pairing. A task-only route captures
view A; its task GET can still name A after removal. Fresh membership is
ordered `[B COMPLETED, C WAITING_FOR_INPUT primary]`. The old resolver ignored
membership primary and selected B. A later update can restore C, masking the
intermediate, irreversible acknowledgement of B.

Before/after (desktop task-only Preview-to-workbench transition):

```text
Before: task=A held + retired initial membership
        -> C proven by removal -> fresh [B,C] -> shell selects/reads B
After:  same independent HTTP gates
        -> C proven -> current membership primary C -> C transcript/read
        B remains unread; newer pin/workflow and an owned null slot survive.
```

Phone keeps its existing Sessions sheet/direct route composition; this changes
shared identity/selection admission, not touch targets, layout or scroll owners.
The existing eight phone unread/removal scenarios remain the rendered parity
gate.

RED, cache-backed snapshot `~/.cache/tmp/unread-verification.zWhGjx`:

- `r20-red.log`: five behavioral failures / 44 controls passed. Fulfilled
  membership survived retirement while the task DTO was pending; the shell
  chose B over membership primary C; real mounted route hydration/task-page
  sync displaced a newer pin and an owned empty slot.
- `r20-ui-red-summary.log` /
  `r20-quiet/kandev-run.e2e.4gxowx2D.log`: intentionally reused the freshly
  retained R19 production frontend before building R20. The real Preview
  button/second-client Delete flow independently held task GET and initial
  membership. Despite eventually displaying C, B acquired a persisted cursor.
  This is product RED, not a selector or fixture failure.

Correction boundaries:

- Keep a retained membership owner until both HTTP legs settle. Validate the
  accepted membership generation at identity publication; only invalidated
  generations are replaced, not generic errors or cancelled navigation.
- Prefer the unique same-task membership primary to a stale DTO's primary;
  an existing explicit requested session retains precedence.
- Route hydration carries the navigation revision captured at entry, and
  checks the current scoped removal receipt at final store admission.
  Hydrate entity data without consuming a newer pin, workflow intent/request,
  or proven removal target/empty slot.
- Ordinary task-page synchronization also respects those current owners,
  instead of undoing the hydration guard with its first fallback effect.

The mounted recovery tests now use the real StateHydrator rather than a
duplicated mock. Adjacent instrumented route mocks accept the owner option.
The new workflow control initially omitted response `entryIdentity`, so its
real reconciliation correctly refused the fixture. That single failure in
`r20-units-final.log` (297 controls passed) is not another product RED; the
corrected correlated response asserts its committed selection before releasing
the route read. Earlier partial correction results are in `r20-green.log`.

An additional ownership control releases identity only after the committed
workflow request has already been acknowledged. `r20-ack-red.log` reproduced
the loss of that recipient (one failure / ten controls). Successful workflow
focus now advances the existing navigation revision once at commit, so
acknowledgement does not erase its selection authority. The route renders
the current owned selection after a newer revision, rather than feeding its
captured fallback back into task-page synchronization. Failed/cancelled moves
do not advance that revision.

`r20-units-delivery.log`: 24 affected files / 298 cases passed before the
acknowledged-workflow extension and final budget cleanup. The strict lint gate
then required moving the membership selection regression into the existing
membership test owner, extracting the route fixtures into a test-only helper,
and using a const task DTO. Typecheck also required a complete task-selection
fixture. These were tooling/fixture corrections, not extra product REDs.

Final source checks after the acknowledged-workflow correction:

- `r20-units-final2.log`: 28 affected files / 349 cases passed, including the
  real route/StateHydrator, membership generations, explicit session admission,
  null/pin/workflow owners before and after acknowledgement, hydration,
  workflow slice/actions, step-move hook and Dockview workflow delivery.
- `r20-lint-delivery.log`: strict changed-code Oxlint passed without warnings.
- `r20-typecheck-delivery.log`: full web typecheck passed.
- `r20-build-delivery.log`: fresh R20 Vite build passed (existing chunk-size
  and dynamic-import warnings are not a clean-warning claim).
- `r20-ratchets.log`: changed-code Prettier, sleep and i18n ratchets passed.

Focused command from the documented prepared cache checkout:

```bash
cd "$VERIFY/apps/web"
pnpm run pretypecheck
pnpm exec vitest run \
  lib/state/task-navigation-reads{,-retry,-budget}.test.ts \
  lib/state/task-session-reads.test.ts lib/state/hydration \
  lib/ssr/session-page-state.test.ts \
  lib/ssr/session-page-enrichment-membership.test.ts \
  lib/ssr/session-page-navigation-bounded.test.ts \
  lib/ssr/session-page-model-hydration.test.ts \
  src/task-detail-route{,-recovery,-session-selection}.test.tsx \
  components/task/task-page-session-ownership.test.tsx \
  components/task/task-page-content \
  lib/state/workflow-session-focus.test.ts \
  lib/state/slices/kanban/kanban-slice.test.ts \
  hooks/domains/kanban/use-workflow-step-move.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx
```

Final rendered verification on the fresh R20 frontend, retained R02
backend/plugin, one worker and no retries:

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- --workers=1 --retries=0
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```

- `r20-desktop-final-summary.log` /
  `r20-quiet/kandev-run.e2e.oCDFUbuH.log`: all 23 cases passed.
- `r20-phone-final-summary.log` /
  `r20-quiet/kandev-run.e2e.ch032EG2.log`: all eight cases passed.
- The independently gated task/membership scenario now displays C, persists
  exactly C's seeded tail, leaves B's cursor null and records no B mark-read,
  without reloading. Inspected its capture: C's stored conversation/composer
  is visible and the task dot remains for the unread sibling.

All temporary/dependency/build/artifact destinations stay on the cache drive.
No live-user browser/data mutation. This is not a broad-suite/live-NATS
certificate or proof of the deleted historical incident's exact trigger.
Authoritative list convergence is not isolated cursor-event delivery evidence;
seeded RUNNING rows are not live executions. Round 20 corrections are ready
for selective delivery and independent Sol round 21; the loop remains open.

### Production-fix review round 21 remediation

Sol child `a011e8ad-0f88-467b-84b2-b92c4b87748b` reviewed
`e7a64f2f3d`. Two P2s concern initial route admission, not unread predicates:

1. A settled removal receipt for C or an empty slot vetoed a later valid
   same-task Command Panel conversation link to B.
2. Independently fetched membership could still name A primary after a later
   task DTO named C, selecting and reading the former primary on a task-only route.

Corrections:

- Capture route-entry navigation, task-scoped workflow and removal ownership.
  A validated explicit session link supersedes only entry-time ownership, not
  a later pin, workflow recipient or removal. Missing/foreign/deleted recipients
  do not gain admission. A background task's workflow does not own this route.
- On contradictory surviving primary identities, obtain a later scoped
  membership observation before readable shell selection. Keep generation
  validation through admission, including membership fulfilled before the
  task leg; do not add generic retries or reinterpret timestamps as ordering.
- Exercise the real route hydrator/page synchronizer and real removal handlers.
  Existing route tests delegate to the real hydrator rather than duplicating it.
  Browser cases use the shipped Command Panel and second-client Preview actions,
  with authentic HTTP dispatch/response gates, not fabricated response data.

Causal evidence in `~/.cache/tmp/unread-verification.zWhGjx`:

- `r21-red.log`: four behavioral failures and 16 passing controls. Explicit
  links remained blocked after both proven-C and empty-slot removal; the
  conflicting snapshot cases retained old membership/selection.
- `r21-owner-red.log`: a background workflow incorrectly vetoed the explicit
  route (one failure, six passing controls). Ownership was then scoped to
  the matching task instead of an unrelated global request counter.
- `r21-ui-red-summary.log`, raw
  `r21-quiet/kandev-run.e2e.WmLaPgnb.log`: on the retained R20 bundle,
  the real conversation-result URL changed to B but active selection stayed C.
  The second case did not execute because worker reinitialization hit the
  existing profile-cleanup DELETE 404; that is infrastructure, not product RED.
- Isolated previously unexecuted primary-skew case:
  `r21-primary-red-summary.log`, raw
  `r21-quiet/kandev-run.e2e.tXBSZJ2l.log`. Authentic old membership and
  later primary-C task read produced an unwanted persisted A cursor, even
  though C eventually appeared. The null-A-cursor assertion caught this.

Unit evidence and cache-snapshot replay:

- `r21-units-final.log`: 29 affected files, 360 passing cases, before the
  lint-budget extraction. `r21-owner-final.log`: the two extracted owner files,
  23 passing cases afterward. The route fixture now uses branded ID constructors
  instead of an unchecked task-object assertion.
- Positive explicit links cover proven-C and null receipts. Negative controls
  cover a newer pin, committed workflow, later removal and unavailable session;
  an unrelated task workflow does not veto a valid link.
- Primary-pair controls cover conflicting, unchanged, retired DTO-primary and
  explicit non-primary requests; fulfilled old membership is not ordering proof.

```bash
cd "$VERIFY/apps/web"
pnpm exec vitest run \
  lib/state/task-navigation-reads.test.ts \
  lib/state/task-navigation-reads-retry.test.ts \
  lib/state/task-navigation-reads-budget.test.ts \
  lib/state/task-session-reads.test.ts \
  lib/state/hydration \
  lib/ssr/session-page-state.test.ts \
  lib/ssr/session-page-enrichment-membership.test.ts \
  lib/ssr/session-page-navigation-bounded.test.ts \
  lib/ssr/session-page-model-hydration.test.ts \
  src/task-detail-route-recovery.test.tsx \
  src/task-detail-route.test.tsx \
  src/task-detail-route-session-selection.test.tsx \
  components/task/task-page-session-ownership.test.tsx \
  components/task/task-page-content \
  lib/state/workflow-session-focus.test.ts \
  lib/state/slices/kanban/kanban-slice.test.ts \
  hooks/domains/kanban/use-workflow-step-move.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx \
  src/task-detail-route-explicit-selection.test.tsx
```

Rendered verification uses the fresh R21 frontend (`r21-build.log`, exit 0)
and retained R02 backend/plugin, one worker and no retries. The frontend did
not change after this build; the later adjustment is E2E setup only.

- Initial full Chromium run: `r21-desktop-final-summary.log`, raw
  `r21-quiet/kandev-run.e2e.jLCgZQ5c.log`: 24 passed, one failed before
  explicit-route admission. The Command Panel screenshot reported conversation
  indexing in progress, so its result was not available. The independently
  gated primary-swap case passed and left A's cursor null.
- Follow the existing `mobile-conversation-search.spec.ts` setup pattern:
  wait for the real conversation API to report ready and return the exact
  seeded session/message, then exercise the palette once. This is fixture
  readiness, not a production retry, a fabricated search result or weaker
  route/cursor assertions. Only the affected desktop case needs rerunning;
  the previously unexecuted phone file runs in full.

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts -- --workers=1 --retries=0 \
  --grep "opens a Command Panel conversation after a settled same-task removal"
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```

Observed results:

- `r21-command-final-summary.log`, raw
  `r21-quiet/kandev-run.e2e.NiDf9WxF.log`: corrected desktop owner case
  passed (24.3s). Together with the earlier 24 this covers the selected 25
  desktop cases, not one fresh 25-green run.
- `r21-phone-final-summary.log`, raw
  `r21-quiet/kandev-run.e2e.MoRfDGj8.log`: all nine phone cases passed
  (2.3m).
- Inspected the saved desktop conversation, primary-snapshot and phone
  conversation captures. The explicit B transcript is shown on both surfaces;
  the independent primary-swap surface shows C. Exact persisted cursor and
  no-unintended-sibling-read assertions passed without reloading.
- `r21-lint-final.log`, `r21-format-final-check.log` and
  `r21-typecheck-final.log`: exit 0 after the test-only indexing precondition.
  `r21-build.log`: fresh production frontend build, exit 0.

All setup/build/dependency/output paths remain on the cache drive. No live-user
browser or data writes, broad-suite/live-NATS certification, historical-trigger
proof, isolated cursor-event claim from list convergence, or live-execution
claim from seeded RUNNING rows. This execution checkpoint precedes selective
delivery and independent Sol round 22; parent owns that handoff.

### Production-fix review round 22 remediation

Sol child `bc9daed1-188a-48fe-bbad-10b7281715f1` reviewed
`75906e0814`. Three source-derived P2s reproduced at their consumer boundaries:

1. A task DTO can name a newly created primary absent from already-fulfilled
   membership. Absence is disagreement too: obtain a later scoped membership
   observation, retaining explicit-session precedence and unavailable-read behavior.
2. A validated explicit route must carry presentation authority, not just store
   selection. A store-scoped, navigation/scope-owned focus request clears only
   the target's hide before reconciliation, activates its panel, then is consumed
   after group-local visibility. Passive restore and later independent panels
   do not inherit that authority.
3. A newer pending workflow intent defers explicit route admission rather than
   discarding it. Cancellation/failure retries against current admitted membership,
   route ownership and scope; a committed recipient, newer pin or removal still
   wins. Identity admission survives optional enrichment omitting `tasks`.

The shared state/selection logic remains viewport-independent. Desktop uses
the shipped Command Panel, tab hide/reopen and workflow stepper. Phone retains
its existing Command Palette/direct conversation route; no new composition,
scroll owner, control size or desktop hide policy is introduced there.

```text
Before: DTO names new C, old list has A -> A may be read
        link hidden B -> C remains visible
        link B + pending move -> failed move leaves C/null
After:  later membership proves C -> display/read only C
        valid explicit link B -> unhide/show/read B only
        pending move -> retain C/null -> failure admits B
        later committed selection -> retain that newer selection
Phone:  same route/ownership decisions; no desktop hidden-tab registry
```

Initial causal evidence in `~/.cache/tmp/unread-verification.zWhGjx`:

- `r22-red.log`: four behavioral failures, 28 passing controls. Newly created
  primary selected A instead of C, hidden B stayed hidden, and both C/null
  variants failed to admit B after workflow cancellation.
- Subsequent owner checks exposed an obsolete exact HTTP-call-count assertion
  and two new fixture calls missing required hydration epochs. The incidental
  count assertion was removed while retaining fresh accepted membership assertions;
  fixture publications now use the real hydration-epoch capture helper.
- `r22-units-final.log`: 33 affected files, 455 cases passed before the
  lint-budget extraction. The imperative panel-focus helper and test setup
  were extracted without changing their policies.
- Readiness control: an environment-less restore control initially passed
  without enabling restored-tab adoption. Once the real environment mapping
  was admitted before the settle edge, `r22-restore-ready-red.log` failed
  (one failure, six controls). Passive restored A displaced the pending explicit
  B activation. Restored-tab adoption now honors that same scoped one-shot
  route request; ordinary restore remains separate.

Final code checks after the extraction and restore guard:

- `r22-units-delivery.log`: 33 affected files, 456 passing cases.
- `r22-lint-final.log`, `r22-typecheck-final.log`, `r22-build-final.log`:
  strict changed-file lint, full web typecheck and fresh Vite build exit 0.
- Controls preserve unavailable links, explicit non-primary requests, newer pins,
  committed workflow recipients, current membership exclusions, ordinary hidden
  restoration and one-shot focus (a later Plan selection is not stolen).

```bash
cd "$VERIFY/apps/web"
pnpm exec vitest run \
  lib/state/task-navigation-reads.test.ts \
  lib/state/task-navigation-reads-retry.test.ts \
  lib/state/task-navigation-reads-budget.test.ts \
  lib/state/task-session-reads.test.ts \
  lib/state/hydration \
  lib/ssr/session-page-state.test.ts \
  lib/ssr/session-page-enrichment-membership.test.ts \
  lib/ssr/session-page-navigation-bounded.test.ts \
  lib/ssr/session-page-model-hydration.test.ts \
  src/task-detail-route-recovery.test.tsx \
  src/task-detail-route.test.tsx \
  src/task-detail-route-session-selection.test.tsx \
  components/task/task-page-session-ownership.test.tsx \
  components/task/task-page-content \
  lib/state/workflow-session-focus.test.ts \
  lib/state/slices/kanban/kanban-slice.test.ts \
  hooks/domains/kanban/use-workflow-step-move.test.ts \
  components/task/dockview-session-tabs.hook.test.tsx \
  src/task-detail-route-explicit-selection.test.tsx \
  components/task/dockview-session-
```

Browser and final fixture evidence:

- `r22-desktop-final-summary.log`: 27 passed, one fixture failure. Both new
  created-primary and hidden-link cases passed. The failed-workflow case hit a
  duplicate `aria-busy` selector before cancellation; cleanup also unregistered
  its held route before fulfillment. Neither is a production behavioral RED.
- Restricted readiness to the actual route root and awaited the owned response
  work before unregistering. `r22-workflow-final-summary.log`: corrected owning
  case passed (1/1). This is split evidence, not a fresh 28-case green run.
- `r22-phone-final-summary.log`: full mobile unread-dot spec, 9/9 passed.
- `r22-fixture-lint.log` and `r22-fixture-typecheck.log`: exit 0 after the final
  test-only correction; formatter changed no lines. Production bundle unchanged.
- Inspected `explicit-route-hidden.png`, `explicit-route-failed-workflow.png`,
  `created-primary-reconciled.png` and `phone-explicit-route-after-removal.png`.
  Requested transcripts are visible; the created primary has its primary star,
  while the former primary remains unread. Failure capture shows the real move
  error and requested conversation, not a simulated successful workflow.

Owning browser commands (managed headless, one worker, no retries):

```bash
cd "$VERIFY/apps/web"
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts \
  e2e/tests/task/task-unread-dot.spec.ts -- --workers=1 --retries=0
pnpm e2e:run --host --shards 1 --no-build --project chromium \
  e2e/tests/session/session-primary-removal.spec.ts -- --workers=1 --retries=0 \
  --grep 'opens an explicit conversation route across failed-workflow'
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome \
  e2e/tests/task/mobile-task-unread-dot.spec.ts -- --workers=1 --retries=0
```

Fresh final R22 frontend; retained R02 backend/plugin artifacts. No live-user
browser or database mutation, production Go/predicate/cursor/wire changes, broad
suite or live-NATS certification. Original incident attribution remains qualified;
list readback proves convergence, not isolated cursor-event delivery, and seeded
RUNNING rows do not prove live execution. Review loop continues at R23.
