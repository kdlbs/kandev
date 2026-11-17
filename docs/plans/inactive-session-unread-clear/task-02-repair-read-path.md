---
id: "02-repair-read-path"
title: "Repair the demonstrated inactive-session read path"
status: blocked
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

This order is blocked because no failed read-after-view scenario has yet been
observed. Do not interpret its likely-file list as a confirmed fix location.
Task 01 must record the causal failure, exact owned production files, and narrow
regression command before this order becomes actionable. A successful Task 01
scenario does not authorize speculative production changes.

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

## Files likely touched

Concrete production ownership must be narrowed from Task 01's observed trace:

- `apps/web/components/task/chat/use-session-read-tracking.ts`
- `apps/web/components/task/chat/use-session-read-tracking.test.ts`
- `apps/web/components/task/chat/session-read-ownership.ts`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/components/task/session-reopen-menu.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.tsx`

Shared verification and delivery records:

- `apps/web/e2e/helpers/unread-divider.ts`
- `apps/web/e2e/tests/task/task-unread-dot.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-unread-dot.spec.ts`
- `docs/specs/tasks/system-design/task-unread-dot.md`, only for a confirmed design change.
- This work order and `plan.md` statuses/results.

The current requirement and `task-unread-dot-visibility.ts` are not change targets:
the existing contract and sibling display rule are to be preserved. No automatic
permission to edit every candidate file is implied.

## Dependencies

Task `01-establish-failure-boundary`: deterministic contract violation, actual
causal trace, exact production owner, and regression command. If this evidence
is absent, leave this work order blocked and report what is missing.

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

Blocked. No production correction was selected. Task 01 found conformance on
desktop and phone: explicitly viewing a cursorless `FAILED` session with no
execution persists its newest loaded row, removes only that session from
`unread_session_ids`, survives navigation and reload, starts no agent, and
leaves an unread `RUNNING` sibling unread. The reported task's failed sibling
has also been read since `2026-11-07T16:23:25Z`. A causal violation, owning
production file, and narrow regression command still do not exist, so this
order cannot become actionable. The permanent desktop/phone regressions from
Task 01 now guard the contract.
