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
