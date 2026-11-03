---
id: "01-normalize-move-roots"
title: "Normalize effective move roots"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-MOVE-SELECTION-001
acceptance_criteria:
  - AC-UI-FILE-MOVE-SELECTION-001.1
  - AC-UI-FILE-MOVE-SELECTION-001.2
  - AC-UI-FILE-MOVE-SELECTION-001.3
  - AC-UI-FILE-MOVE-SELECTION-001.4
  - AC-UI-FILE-MOVE-SELECTION-001.5
  - AC-UI-FILE-MOVE-SELECTION-001.6
system_design:
  - ../../specs/ui/system-design/file-move-selection.md
---

# Task 01: Normalize Effective Move Roots

## Summary

Write independent regression tests, qualify causal RED, and normalize directory
coverage before target construction in the real move coordinator. Finish with
bounded mounted coverage and real-backend desktop hierarchy GREEN.

## In scope

- Private stable effective-root filtering in `executeMoveFiles` before naming.
- Mounted selection/drag with actual operations/WS dispatch and literal oracles.
- Only the fixture seam needed to specify expected effective-root count, plus
  one desktop E2E scenario; preserve all existing settlement/admission tests.
- Actual result/status/traceability records and fixture cleanup.

## Out of scope

All [manifest exclusions](plan.md#scope)
apply. No production or permanent tests/install/build/heavy/commit/push during
design. No backend, unused-helper symmetry, reply/tree races, same-folder repair,
companion actions, lifecycle hardening, layout/copy or new phone interaction.
You are not alone in the codebase: preserve others' edits and foreign resources.

## Acceptance

1. Literal WS payloads show only effective roots in original relative order,
   with strict segment ancestry at every depth and existing collision targets.
2. A successful covering directory moves selected and unselected nested bytes
   together on the real backend; no child rename or flattened child appears.
3. Existing independent result feedback, self-drop rejection and all settlement
   coverage pass unchanged; no new viewport-dependent behavior or phone feature.

## ASCII UI preview

UI-01: Desktop Files, expanded selected folder and child, drop on `dest`.

```text
BEFORE                            AFTER
[x] src/                          dest/
  [x] a.txt                         src/
dest/                                a.txt
two rename commands               one directory-root rename
flattened child OR ENOENT          subtree hierarchy/bytes intact
```

UI-02: Phone Files bottom-nav entry and focused viewer.

```text
BEFORE                            AFTER
[Files header]                    [Files header]
v src/                            v src/ (current hierarchy)
  a.txt [...]                       a.txt [...]
[Files bottom nav]                [Files bottom nav]
tap -> [path] [Close] viewer       tap -> [path] [Close] viewer
Close -> Files                    Close -> Files
```

See [full previews](plan.md#ascii-ui-preview). Hierarchy is required by AC
`.1`-.4; phone value by `.6`. Existing internal tree scroller, safe-area/bottom
clearance, visible touch actions and single focused list/viewer stay as shipped.
No touch DnD claim or geometry change; spacing is illustrative.

## Regression matrix

Independently author `file-browser-move-selection.test.tsx` using the existing
real mounted settlement fixture. Settle initial root/destination reads before
events; expand source ancestors before selecting. Do not mock selection,
`executeMoveFiles`, target computation, `useFileOperations` or WS rename helper.
Observe actual full `workspace.file.rename` args including session ID; expected
old/new paths below are literals, not implementation-derived expectations.

| Case | Selection order | Expected commands to `dest` | AC |
| --- | --- | --- | --- |
| Folder-first RED | `src`, `src/a.txt` | `src -> dest/src` only | .1, .2 |
| Child-first RED | `src/a.txt`, `src` | `src -> dest/src` only | .1, .2 |
| Multi-level, outer last | `src/sub/deep/a.txt`, `src/sub`, `src` | `src -> dest/src` only | .1, .2 |
| Nested directory root | `src/sub/deep/a.txt`, `src/sub` | `src/sub -> dest/sub` only | .2 |
| Mixed roots, ancestor late | `beta.txt`, `src/a.txt`, `other/b.txt`, `src` | `beta.txt -> dest/beta.txt`, `other/b.txt -> dest/b.txt`, `src -> dest/src` | .3, .5 |
| Prefix control | `src-other/a.txt`, `src`, `src/a.txt` | `src-other/a.txt -> dest/a.txt`, `src -> dest/src` | .3 |
| Folder-only control | `src` | `src -> dest/src` | .3 |
| Child-only control | `src/a.txt` | `src/a.txt -> dest/a.txt` | .3 |
| Disjoint control | `alpha.txt`, `other/b.txt` | `alpha.txt -> dest/alpha.txt`, `other/b.txt -> dest/b.txt` | .3 |
| Directory collision, existing `dest/src` and `dest/src (1)` | `src/a.txt`, `src` | `src -> dest/src (2)`; children remain nested | .4 |
| Covered child cannot reserve name | `src/a.txt`, `other/a.txt`, `src` | `other/a.txt -> dest/a.txt`, `src -> dest/src` | .4 |
| Shift range across expanded source | anchor `src`, Shift-click visible `src/a.txt` | `src -> dest/src` only | .2 |

Assert destination subtree and absence of standalone child after successful
settlement, with independently specified paths. For mixed roots accept the
directory and refuse/reject an independent root through the real operation
hook; require existing error feedback and truthful independent locations.
Do not add a new settlement-race matrix. Run the entire existing settlement
suite unchanged, including both outcome orders, collisions, refresh and cache.

Fixture `drag()` hardcodes request count as selected count. Add an optional
independently specified expected-root count with the current default, or bypass
that assertion using real events; Shift may use direct events. No generalized
fixture overhaul or copied ROOT proof. Teardown all new reads/timers/providers.

## Desktop causal RED and GREEN

Add one test named `selected directory carries selected and unselected descendants`
to `e2e/tests/task/file-tree-drag-drop.spec.ts`. Seed an isolated ordinary source
folder with selected `src/a.txt`, unselected `src/sub/keep.txt`, and occupied-free
destination sentinel `dest/keep.txt`, all with distinct literal bytes. Materialize
the exact fixture branch and obtain the actual task checkout path from environment
records and fixture existence. Do not assume `backend.tmpDir/repos/e2e-repo` is
the mutated checkout. A minimal setup return of the resolved checkout is allowed.

Arm WS capture before navigation; scope it to this session/old paths. Expand
`src`, Ctrl-select child first then folder, assert both rows actually selected,
and dispatch existing native HTML5 event sequence with shared `DataTransfer`.
Wait causally for the real captured rename replies to settle, not a fixed delay
or reload. Assert actual disk source absence, exact selected and unselected
destination bytes, no `dest/a.txt`, destination sentinel intact, and visible
nested tree hierarchy. Require only `src -> dest/src` in actual requests and a
successful reply, with no child request/error. Pre-fix RED must reach command or
hierarchy assertions after real dispatch/backend evidence; setup/import/selector
failure is not qualified RED. Record which assertion causally failed. Final
GREEN runs the full owning DnD spec, retaining file-only and self-drop controls.

## Verification

These commands are for the implementation phase; current execution authorization
is held in the external task plan. Use bash without login, repo root as cwd,
existing toolchains, GNU `timeout`, and strictly sequential execution. Verify
toolchain paths at resume; no broad dependency audit. Conditional bootstrap once:

```bash
export PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH
export GOTOOLCHAIN=local
export GOMAXPROCS=2
export GOMEMLIMIT=512MiB
if [ ! -d apps/node_modules ]; then (cd apps && corepack pnpm install --frozen-lockfile); fi
```

First author permanent tests and run mounted both-order RED, then desktop RED
against a fresh pre-fix production build. No production edit before both causal
RED results are joined and recorded. Commands (all rooted independently):

```bash
(cd apps/web && timeout --kill-after=10s 180s corepack pnpm exec vitest run components/task/file-browser-move-selection.test.tsx --maxWorkers=1)
(cd apps/web && timeout --kill-after=10s 900s corepack pnpm e2e:run --host --shards 1 --project chromium tests/task/file-tree-drag-drop.spec.ts -- --grep 'selected directory carries selected and unselected descendants' --workers=1 --retries=0)
```

After minimum production correction and all final edits, run the full affected
mounted set and fresh managed desktop GREEN serially. Managed E2E rebuilds the
production Vite/backend/fixture assets; never test stale pre-fix assets:

```bash
(cd apps/web && timeout --kill-after=10s 180s corepack pnpm exec vitest run components/task/file-browser-move-selection.test.tsx components/task/file-browser-move-settlement.test.tsx components/task/file-tree-utils.test.ts hooks/use-file-operations.test.ts components/task/file-browser-responsive.test.tsx --maxWorkers=1)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 timeout --kill-after=10s 360s corepack pnpm run typecheck)
(cd apps/web && timeout --kill-after=10s 180s corepack pnpm exec eslint --max-warnings 0 components/task/file-browser-move.ts components/task/file-browser-move-selection.test.tsx components/task/file-browser-move-settlement.test-helpers.tsx e2e/tests/task/file-tree-drag-drop.spec.ts)
(cd apps/web && timeout --kill-after=10s 180s corepack pnpm exec prettier --check components/task/file-browser-move.ts components/task/file-browser-move-selection.test.tsx components/task/file-browser-move-settlement.test-helpers.tsx e2e/tests/task/file-tree-drag-drop.spec.ts)
(cd apps/web && timeout --kill-after=10s 180s corepack pnpm run i18n:check)
(cd apps/web && timeout --kill-after=10s 900s corepack pnpm e2e:run --host --shards 1 --project chromium tests/task/file-tree-drag-drop.spec.ts -- --workers=1 --retries=0)
```

The single required typecheck gets a process-local 6144 MiB Node heap; do not
alter tracked heap settings or dependencies. Managed builds inherit the bounded
Go environment above. Do not expand resource limits automatically or retry an
OOM/timeout; preserve its evidence and report the blocker before continuing.

After all final code/test edits, stage the actual owned code/test paths before
running the existing diff ratchets. This staging occurs only in implementation,
not at the design checkpoint. Keep all new owned paths in the staged set so
untracked additions cannot escape the scan:

```bash
git add -- apps/web/components/task/file-browser-move.ts apps/web/components/task/file-browser-move-selection.test.tsx apps/web/components/task/file-browser-move-settlement.test-helpers.tsx apps/web/e2e/tests/task/file-tree-drag-drop.spec.ts
git diff --cached --name-status -- apps/web
(cd apps/web && timeout --kill-after=10s 180s corepack pnpm run i18n:ratchet)
(cd apps/web && timeout --kill-after=10s 180s corepack pnpm run e2e:sleep-ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/directory-covering-file-moves
```

Record actual scanned/staged paths and the ratchets' reported exclusions. Tests
and E2E exclusions from the i18n scan are not i18n coverage claims. After closing
the manifest/order results and statuses, stage those documentation updates and
commit through the normal repository flow. Any later code/test edit must be
staged and its affected final checks rerun before commit.

Run actual documentation coverage preflight using the repository's exported
`validateCoverage({changedFiles,fileContents})` from `.github/scripts/pr-docs.cjs`:
include every actual changed path (tracked and new), not a docs-only exempt
result. During design include the four anticipated code/test paths listed above
and the changed design package to test future-production coverage. Resolve the
manifest, owning requirement and design contents from real files; require
`status: covered`, `ok: true`, `errors: []`. Do not run unchanged validator units.
Retain the exact preflight invocation and result in the external checkpoint.

Phone: existing responsive mounted suite plus the documented shared-state
exception is meaningful bounded validation. No mobile E2E/visual claim is made;
opening/closing the shipped viewer remains context, not directory-move proof.

Record actual original command handles/PID groups/start/cutoff/exit/join identity
and narrowly owned cleanup externally. Poll every handle to actual completion.
No concurrent heavy/build command, broad suite, auto retry, foreign process
cleanup, or managed app-server-parent termination. Use the repository's ordinary
PR flow after closing results; mutable hosted instructions stay external.

## Files likely touched

- `apps/web/components/task/file-browser-move.ts` (only production correction).
- `apps/web/components/task/file-browser-move-selection.test.tsx` (new).
- `apps/web/components/task/file-browser-move-settlement.test-helpers.tsx` (count seam only).
- `apps/web/e2e/tests/task/file-tree-drag-drop.spec.ts` (one real-backend scenario).
- This manifest/order and owning requirement/design statuses/results.

## Dependencies

None. Implementation follows the separate design handoff; execution authorization
and resource availability are recorded externally.

## Risks

Order-dependent ancestor discovery, incorrect prefix boundaries, collision-name
reservation before filtering, fixture-only hierarchy proof, stale built assets,
and E2E disk assertions against the seed repo instead of actual task checkout.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/ui/requirements/file-move-selection.md), AC `.1`-.6.
- [Owning design](../../specs/ui/system-design/file-move-selection.md).
- [Existing settlement design](../../specs/ui/system-design/file-browser-reply-freshness.md#file-move-settlement).
- Actual coordinator/helper/fixture/DnD source; ROOT evidence reference only.
- Fix/spec/plan/mobile-parity/TDD/E2E skills and scoped frontend AGENTS.

## Results

Local implementation is complete. The coordinator changes only the effective
source list before existing target construction; production naming, admission,
dispatch, settlement, utilities, operations and backend policy remain intact.
All existing settlement tests remain unchanged; the shared fixture adds only
the optional expected-root count. New mounted expectations are literal data.

| Actual command/check from Verification | Result |
| --- | --- |
| One conditional `corepack pnpm install --frozen-lockfile` from apps | PASS; cached packages, no lockfile change |
| New mounted suite before correction | Qualified RED: 10 causal failures/3 controls PASS; both orders expected 1 request, received 2 |
| Single grep-selected fresh managed desktop before correction | Qualified RED on extra child rename after actual backend replies and disk assertions; one worker/retries0 |
| Final five-suite `vitest run ... --maxWorkers=1` | PASS 54/54, including all 13 new cases |
| Scoped `eslint --max-warnings 0 ...` | PASS after test-only lint corrections |
| Final `prettier --check ...` | PASS all four files |
| One `NODE_OPTIONS=--max-old-space-size=6144 ... pnpm run typecheck` | PASS within GNU360s bound |
| `pnpm run i18n:check` | PASS catalog/copy gates |
| Staged `pnpm run i18n:ratchet` | PASS 0 added/2 modified eligible files; guard allowlist intact |
| Staged `pnpm run e2e:sleep-ratchet` | PASS 0 added/1 modified E2E file |
| Full owning fresh managed desktop spec | PASS 3/3, one worker/retries0 |
| `python3 scripts/list-docs.py validate` | PASS 381 decisions/1591 specifications |
| `python3 scripts/lint-spec-files.py --all` | PASS |

The new test and E2E spec were staged before ratchets but are excluded by the
i18n copy scanner's `.test`/`.spec` rule. The two eligible modified files are
the production coordinator and fixture helper; the latter has no new copy.
The actual E2E spec is included in the sleep ratchet. No unstaged/untracked
coverage or phone touch drag evidence is claimed.

All command starts/deadlines/PID groups/exits and actual native joins are retained
under `/tmp/kandev-move-selected-folder-design-20261013-6stsdtfv/`. Its
`red-qualification.json`, `desktop-green-commands.json` and
`desktop-green-cleanup.json` distinguish causal failure, real successful
commands, and observed owned-process/root cleanup. The final actual-path
documentation preflight and diff/status receipts are saved there before commit.
No passing check is replayed for reassurance and no general local suite is run.

## CI fixture observation correction (2026-10-27)

SDK integration head `d62c643512a` exposed a post-move observation race in the
real-backend desktop scenario. The failed attempt's captured command succeeded,
all filesystem/byte checks passed, and exactly one directory rename was issued.
The destination tree node then detached; the failure snapshot shows Changes
active with the four move-related paths. Only the successful retry has a trace,
so the original activation ordering is not retained.

A controlled probe holds the actual status notifications and Git-refresh
responses after the initial status/commit baseline is loaded. The real rename
and existing filesystem assertions complete while the new Changes rows remain
absent. Selecting Files before releasing those frames lets the subsequent Git
update activate Changes, and the original destination-tree assertion fails.
Waiting for both destination Changes rows before selecting Files passes that
control and preserves every original command, byte, hierarchy, and absence
assertion. This follows the adjacent single-file move scenario. No production
move, selection, or tab-activation behavior changes. First-failure tracing is
retained. The full repeated spec and fresh CI remain delivery checks.

Local validation: all nine cases in three full-spec repetitions passed with
retries disabled. Scoped lint/format, full docs/spec checks, and linked-delivery
validation passed. Independent read-only review found no actionable findings.
