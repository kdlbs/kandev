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
  supersession. Do not add polling or new automatic retry behavior.
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
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts)
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
- `apps/backend/internal/gateway/websocket/task_notifications.go` and
  `task_notifications_test.go` (ordered failure/primary notification delivery).

Shared verification and delivery records:

- `apps/web/e2e/helpers/unread-divider.ts`
- `apps/web/e2e/tests/task/task-unread-dot.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-unread-dot.spec.ts`
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

F1: retain the publisher's ordering rather than introduce a new payload schema.
`TaskEventBroadcaster` now routes `task.updated` through the existing lifecycle
wildcard alongside session/task state changes. The orchestrator publishes the
failed state before promoting a sibling; NATS callbacks on separate subscriptions
could invert those notifications. A single callback queue prevents that inversion.
No workflow focus, primary promotion, or unread predicate changes are needed.

`TestTaskEventBroadcaster_OrdersFailureBeforePrimaryPromotion` exercises both
the memory bus and the existing deterministic NATS-style callback-queue fake.
Before the fix, the fake's task-first scheduling produced `task.updated` before
`session.state_changed`; the memory counterpart passed. After the fix both
preserve order and deliver exactly once. The obsolete hard-coded subscription
count/counter test was removed rather than re-pinned to another count.

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
pnpm exec vitest run lib/ws/handlers/agent-session.test.ts lib/ws/handlers/tasks.test.ts
pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts
```
