---
id: "01-bind-local-controls"
title: "Bind local SSH reachability controls"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-002.3
  - AC-EXECUTORS-SSH-REACHABILITY-002.4
  - AC-EXECUTORS-SSH-REACHABILITY-002.5
  - AC-EXECUTORS-SSH-REACHABILITY-002.6
  - AC-EXECUTORS-SSH-REACHABILITY-002.10
  - AC-EXECUTORS-SSH-REACHABILITY-002.11
  - AC-EXECUTORS-SSH-REACHABILITY-002.12
  - AC-EXECUTORS-SSH-REACHABILITY-002.13
  - AC-EXECUTORS-SSH-REACHABILITY-002.14
system_design:
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
---

# Task 01: Bind local SSH reachability controls

## Summary

Make the existing hook's local load and probe controls belong to its committed
executor/store/instance visit. Prove the correction through deferred transport
responses and the real SSH APIs, provider, store, hook and rendered card, while
preserving backend request lifetime and keyed evidence.

## Inputs and execution gate

Read [the manifest](plan.md), its evidence and coverage table, the
[requirement](../../specs/executors/requirements/ssh-reachability.md), and
[local request lifetime design](../../specs/executors/system-design/ssh-reachability-surfaces.md#local-settings-request-lifetime).
Read `apps/web/AGENTS.md` and `/tdd` before permanent code/test changes.

Execution checkpoint: **local implementation complete**. ROOT reviewed and released the package
with the exclusive GLOBAL LOCAL-HEAVY lease on 2026-10-06. Task identity is
`8569c391-dae8-4062-b97c-4ad5b647afc6`; primary session is
`f8c2fd84-ebab-41d2-af78-b6b1036cdc73`. Continue here only after a later ROOT
reviewed-package implementation INTERRUPT. A material ROOT message is steering
for this same primary; update its actual conversation and durable task plan.
Do not open another session/tab/task or delegate. No operator approval or model
switch prompt. The task remains incomplete until actual authorized merge and
joined cleanup; ROOT owns independent verification/archive/refill.

## In scope

- `useSSHReachability` committed admission/publication/error/finally ownership,
  local control reset, current probe pending admissions and stale callbacks.
- Independently authored lifetime tests; preserve ROOT proof files untouched.
- Focused existing card/store/API compatibility tests, acceptance coverage,
  changed-file lint/format, project typecheck, i18n/docs checks and normal hooks.
- Update delivery results/statuses only from actual joined command outcomes.

## Out of scope

- Backend changes; probe abortion or changed completion/persistence/coalescing.
- Store reconciliation/cache/API/WS shape, staleness timing, launch behavior,
  security/defaults, global error/retry policy or generic lifecycle utilities.
- New UI/copy/layout/navigation/breakpoints/touch geometry or unrelated producer
  rewrites. Existing consumer production files are read-only inputs.
- Main-only rebase, synthetic checks, weakened gates, blind retries/cache wipes,
  foreign kills, managed-worktree/dependency deletion or proof replay/import.

## Acceptance

1. All admitted local work belongs to the committed executor/store/instance
   lifetime. Old A initial/cadence/probe success, reject and finalizer cannot
   publish into B or a new A visit; retained callbacks after cleanup issue no
   request. Unmount/remount, StrictMode and separate current owners are covered
   through real runtime consumers, including cleanup before passive effects.
2. B's actual card action works while A is pending, targets B, and disables
   during B's own probes. An obsolete or earlier finalizer cannot release B's
   pending work. Current GET/probe failures and success preserve the existing
   local error/record presentation; latest GET refresh still wins.
3. Existing independently accepted A/B records, WS/reset timestamp ordering,
   backend request lifetime, cadence and current owner behavior remain intact.
   All targeted checks pass with named acceptance coverage recorded; no new
   layout/mobile behavior or public documentation contract is introduced.

## Implementation sequence

1. After the release and exclusive resource grant, mark this order
   `in_progress`, synchronize the manifest/task plan, and inspect current
   relevant blobs without rebasing for moving main alone.
2. Add `use-ssh-reachability.lifetime.test.tsx` independently. Use the real
   `StateProvider`, capture `useAppStoreApi`, mount the real hook and card,
   and partial-mock only `@/lib/api/client`'s `fetchJson` via `importOriginal`.
   Assert encoded GET path and probe POST path/method. Use deferred requests
   so A and B really overlap; settle all promises and clean timers.
3. Red: run the new lifetime suite against the unchanged hook. Record causal
   failures for `retired initial load cannot mark B not-known` and
   `B can probe while A is pending`, with independent positive controls for
   own GET failure and own probe disable/reenable. Unexpected setup/resource
   failure is not a red test. Do not replay or copy ROOT's scratch proof.
4. Green: apply the smallest hook-local committed lifecycle repair from
   `use-routing-preview.ts`. Keep a non-reused generation, layout cleanup,
   guard admission and every settle path, reset local controls on a committed
   identity change, and account for current pending probes. Do not mutate an
   active owner during render or attach new cancellation signals.
5. Cover every row of the manifest's matrix: A-to-B-to-A; initial/refresh/probe
   obsolete success/reject/finalizer; callback calls and pending completion
   from layout cleanup before passive effects; remount/StrictMode; independent
   same/different-executor owners and real separate providers; current
   errors/success with/without records; superseded same-executor GETs;
   overlapping current probes; newer keyed records/configuration reset.
6. Run the exact serial checks below, inspect named coverage, and record
   original command receipts. Mark this order `done` only when all required
   local checks pass; synchronize manifest and paired spec lifecycle only
   after confirming implementation conformance.

## ASCII UI preview

UI-01 excerpt from [the combined preview](plan.md#ascii-ui-preview), criteria
.13/.14. B is the displayed executor; A's request is still pending.

```text
+------------------------------------------+
| B reachability   [Probe now: enabled]     |
| B's accepted record, or B's own not-known |
+------------------------------------------+
Click B: action disables until B probes settle.
A settles: no change to B error or pending control.
```

Desktop action remains beside the heading; phone action remains below it in
the shipped `SettingsCardHeader`. No structure, geometry, scroll, breakpoint
or touch change. The real rendered action/error/record tests satisfy the
mobile-parity state/data exception. Do not add mobile Playwright for this
bounded correction; original SSH containers E2E is historical broader feature
evidence and is not scheduled for rerun.

## Resource preparation and receipts

Require both the later implementation release and ROOT's exclusive
**GLOBAL LOCAL-HEAVY** grant before installation or product checks. Use existing
Node 24.21.0 at `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin` and
repo-pinned pnpm 9.15.9; invoke tool commands with `/bin/bash`, `login:false`.
Verify tool versions before dependent commands. If this managed worktree lacks
working workspace dependencies, run ONE conditional frozen install from
`apps`; otherwise reuse them. Preserve dependencies after work.

Each command is serial and task-owned. Capture UTC start/end, command,
PID/PGID, original tool handle, log path and actual exit. Retain every yielded
`session_id`, poll that original handle to ACTUALLYJOINED completion and confirm
its owned processes are gone before the next command. Use Node 4 GiB for
Vitest, one worker, no file parallelism and GNU outer 120s with kill after 10s.
No overlapping suites, cache wipe, foreign kill or blind retry. Routine own
fixture/lint fixes rerun only affected checks. Setup/resource/timeouts or
out-of-scope unknown failures checkpoint ROOT with original evidence.

## Verification

Commands below run from repo root with the explicit Node path. Run preparation
only after both gates, and use isolated directory roots as shown. The pnpm shim
uses `apps/package.json`'s `pnpm@9.15.9`; stop if the version differs.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
node --version
(cd apps && pnpm --version)
# Conditional ONE install only if dependencies are absent/unusable:
(cd apps && pnpm install --frozen-lockfile)
```

Red run, before hook changes:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx)
```

Final focused run includes every changed suite and the existing compatibility
boundaries. No broad suite or coverage instrumentation dependency is required;
coverage is the actual named tests mapped to every matrix row in Results.

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx components/settings/ssh-reachability-card.test.tsx lib/state/slices/settings/settings-slice.test.ts lib/api/domains/ssh-api.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/settings/use-ssh-reachability.ts hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx)
(cd apps && pnpm exec prettier --check web/hooks/domains/settings/use-ssh-reachability.ts web/hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Typecheck includes its existing pretypecheck generators; inspect resulting
files and preserve unrelated content. If another owned test file is changed
for a meaningful uncovered case, add that exact file to the focused/lint/format
commands before running; do not silently broaden suite scope.

## Files likely touched

- `apps/web/hooks/domains/settings/use-ssh-reachability.ts` (production owner).
- `apps/web/hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx` (new
  independent transport-boundary regressions and real rendered-card cases).
- The existing requirement/design pair and this manifest/order (status/results).

Read-only consumers/patterns: `components/settings/ssh-reachability-card.tsx`,
`components/state-provider.tsx`, `lib/state/store.ts`, settings slice,
`lib/api/domains/ssh-api.ts`, existing card/store/API tests,
`hooks/domains/office/use-routing-preview.ts`, and
`hooks/domains/session/use-commit-detail.lifecycle.test.tsx` for real provider
capture, deferred requests and layout attempts.

## Dependencies

None. One sequential order; later ROOT implementation release and exclusive
resource grant are operational gates, not additional work orders.

## Delivery gates after implementation

Later standing delivery authorization applies only after the reviewed-package
implementation interrupt. Load commit/push/PR guidance and `.github/AGENTS.md`;
use normal hooks, no bypass. Once all required checks pass, commit, push and
open one ready PR. Freeze its SHA except for actual findings. Accept automatic
authenticated configured App347564 FULL current-head ALL-files substantive
review; inspect skipped/gapped evidence before ONE necessary review request.
Do not wait for optional acknowledgement or request duplicate review.

Explicitly return the local-heavy grant before starting ONE original 90-minute
all-terminal hosted observer (`scripts/pr-await <PR>`, GNU outer 91m/kill10).
Inspect reviews/failures promptly while CI runs and retain/join that observer
before any ROOT-authorized successor. All six required checks plus parent
Backend/Frontend/E2E must be SUCCESS, with fresh complete `errors: []`, clear
threads/human/hidden evidence and current-head review. No uncontrolled hosted
retries or moving-main rebase. Report **MERGE READY END** and end the turn.

Merge requires a separate ROOT **SERIAL MERGE** interrupt: normal expected-head
squash, independently verify actual merged SHA/tree/owned blobs and main,
then join only owned cleanup processes. Preserve managed worktree, deps, ROOT
proof/evidence and foreign resources. Do not signal completion at design,
local implementation, PR creation or merge-ready checkpoint.

## Risks

Commit/passive-effect timing, executor-ID reuse, same-owner finalizers and
StrictMode replay can conceal an incomplete guard. Keyed store evidence is
independent of visit-local controls. The transport mock must preserve real API
functions and unrelated exports so tests exercise immediate consumers.

## Parallelism

`sequential`

## Results

Completed 2026-10-06. Production change is limited to the existing hook;
27 independently authored lifetime cases plus 36 existing card/store/API
cases pass (63 total). ROOT's proof was never read, replayed, copied, imported,
mutated or deleted. Approved design hashes were verified before execution.

RED against the unchanged hook: first run 6 failures/4 passes, expanded run
13 failures/10 passes. Both reported causal cases failed and both current-owner
positive controls passed. The initial commit-stage fixture invoked work during
sibling deletion before owner cleanup; it was corrected to invoke retained
callbacks from the replacement layout commit before passive effects. That
corrected case independently failed against the original hook (1 failure,
23 unselected). Initial GREEN was 58/59; corrected fixture plus remaining
lifetime cases passed 63/63. Final run after fixture lint/format edits passed
63/63. No setup, timeout or out-of-scope failure occurred.

Acceptance coverage:

Review correction for PR #4272, Greptile comment 4200044167: two additional
transport-boundary clock cases pass in the anchored `reachability clock
lifetime` group (2 passed, 27 unselected). `current fresh card becomes stale
only after its deadline despite failed refreshes` preserves the accepted store
record through failed periodic GETs, checks the exact three-interval boundary,
and renders the stale badge one millisecond later. `retired clock callback
leaves B fresh until B's own clock callback runs` invokes a captured A callback
after the B visit commits, observes unchanged B presentation/evidence, then
invokes B's current callback as the positive control. Deferred requests and
fake clocks use the existing cleanup. Production source and cadence are
unchanged; the earlier 63-case run remains historical validation, not a replay.

- .13/.10: retired initial load and A-B-A success/failure; obsolete cadence
  refreshes; retained load/probe callbacks invoked during replacement layout
  commit and again after it; pending GET/probe success/failure after unmount.
- .13/.14: A pending/B actual card click, encoded B POST/method, obsolete
  finalizer leaves B disabled; A-B-A probes; same-executor remount; StrictMode
  initial success/failure and probe-finalizer replay; same/different-executor
  siblings plus separate simultaneous real providers.
- .3/.13: newer keyed A and B reset evidence survives obsolete HTTP;
  same-executor refresh supersession; a current older HTTP success clears
  its own error without displacing newer reset evidence.
- .5/.10/.14: own GET failure remains not-known; own probe disable/success;
  overlapping probes stay pending until both settle; current refresh/probe
  errors retain accepted records or report not-known when absent.
- .4/.11/.12: existing rendered card cadence/staleness and localized state/
  record cases pass; shared phone/desktop controls and geometry are unchanged.
  Existing API and store suites validate route and timestamp compatibility.
- .6: admitted requests receive no new abort signal or backend policy change;
  old transport promises settle independently while hook publication is inert.

One conditional frozen install was required because `apps/node_modules` was
absent. Node 24.21.0 and pnpm 9.15.9 were confirmed; all original commands were
run serially under the exclusive lease and ACTUALLYJOINED with their process
groups gone. Original UTC/PID/PGID/tool-handle receipts and raw logs remain in
`/tmp/kandev-child58/<operation>.json` and `.log`; no foreign process was killed.
Routine test fixture lint repairs split long describe blocks and replaced a
repeated test ID with a constant. Hook formatting was already clean.

| Receipt operation | Exact command (within documented working directory) | Result |
| --- | --- | --- |
| `focused-final` | `env NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx components/settings/ssh-reachability-card.test.tsx lib/state/slices/settings/settings-slice.test.ts lib/api/domains/ssh-api.test.ts` | PASS, exit 0 |
| `changed-eslint-final` | `pnpm exec eslint --max-warnings 0 hooks/domains/settings/use-ssh-reachability.ts hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx` | PASS, exit 0 |
| `format-check` | `pnpm exec prettier --check web/hooks/domains/settings/use-ssh-reachability.ts web/hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx` | PASS, exit 0 |
| `typecheck` | `env NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck` | PASS, exit 0 |
| `i18n-check` | `pnpm run i18n:check` | PASS, exit 0 |
| `i18n-ratchet` | `pnpm run i18n:ratchet` | PASS, exit 0 |
| `docs-catalog` | `python3 scripts/list-docs.py validate` | PASS, exit 0 |
| `spec-linter-tests` | `python3 scripts/lint-spec-files.test.py` | PASS, exit 0 |
| `spec-lint` | `python3 scripts/lint-spec-files.py --all` | PASS, exit 0 |
| `public-doc-tests` | `node --test scripts/validate-public-docs.test.mjs` | PASS, exit 0 |
| `public-doc-lint` | `node scripts/validate-public-docs.mjs` | PASS, exit 0 |
| `diff-check` | `git diff --check` | PASS, exit 0 |

Specification-linter tests: 36 passed. Public-doc validator tests: 62 passed;
47 published pages validated. Typecheck generators produced no unrelated
working-tree change. Existing i18n orphan-key notices were warnings only;
i18n key/translation and ratchet checks passed.

The owning requirement/design were checked against the final hook and promoted
to active/current. Task 01 is done and the manifest is implemented. Public docs
and screenshots need no update for this state-only lifetime correction. Normal
hook commit, published-head alignment and hosted evidence are subsequent
checkpoints recorded in the live task plan/private delivery receipts; merge
remains subject to a separate ROOT SERIAL MERGE interrupt.
