---
created: 2026-10-07
status: implemented
requirements:
  - REQ-UI-TRANSCRIPT-HISTORY-FRESHNESS-001
system_design:
  - ../../specs/ui/system-design/transcript-history-freshness.md
legacy_specs: []
---

# Implementation plan: Preserve live tool results during history refresh

## Overview

One sequential work order corrects matching-row revision selection in the
bounded latest-window reconciler. Write meaningful pure and rendered production
regressions first, observe their causal failure, then apply the local selector.
ROOT reviewed the four-file design seal at the original baseline and released
this one work order for implementation in the same primary session. Local
heavy work is exclusive to child66 and serial; hosted collection and merge
remain gated on later separate ROOT releases.

The owning [requirement](../../specs/ui/requirements/transcript-history-freshness.md)
and [design](../../specs/ui/system-design/transcript-history-freshness.md) reuse
the UI transcript owner. Existing history-visibility and subscription-recovery
packages retain their acceptance criteria, status, recorded historical results,
and verification scope; this work adds no task to those completed packages.

## Confirmed cause and evidence

At baseline `5bbc6ec231b03eda49b57e8e4f8c9c2bbab484ce`, `joinMessages` takes
fetched rows for matching IDs. Authoritative and disjoint filters also exclude
those IDs from retained extras. `cachedAtRequest` and `cachedAtResponse` already
exist, but matching-row revision selection ignores both the live change and its
newer update time.

ROOT's read-only protected proof is
`/tmp/kandev-root-live-message-snapshot-candidate.test.tsx`, SHA256
`ac35e2877081413e7388124f08c911a7c85d2cb8eadbe3d28c5b3724e4c9b59d`.
The receipt and classification JSON at
`/tmp/kandev-root-live-message-snapshot-proof-{receipt,classification}.json`
record one causal FAIL and two controls PASS. Native session `17473`, chunks
`5dab17` to `00d5f8`, actually joined exit 1. Wrapper PID 994818 and command
PGID 994877 were independently gone; the owned overlay was removed and ROOT
was clean. The proof exercised real hook/store/renderer before settlement,
with transport only mocked. An old same-ID row replaced completed
`all checks passed: 42`/`complete`/05:00:01 with
`tool is running`/`running`/05:00:00.

The after-settlement DOM assertion followed the failing store assertion and
did not execute. No server, dispatcher, browser, persistence, or unrelated
pagination execution is claimed. This design turn verified the protected
candidate hash and read the receipt/classification; it did not replay, copy,
import, mutate, or delete any protected artifact. Author permanent tests
independently at the designated production boundary after release.

## Scope and technical approach

- Own `message-window-reconciliation.ts`, its pure tests, and a new real
  hook/store/renderer test suite. Resolve matching fetched IDs before all three
  merge branches, using strict update-time comparison and original immutable
  request references for ties/uncomparable times.
- Preserve server-newer acceptance, full-row coherence, membership, pagination,
  authoritative empty/deletion, pending optimistic rows, store/session identity,
  request sharing, readiness, loading, and retry behavior.
- Audit existing older and around reads without changing their duplicate
  policies. No Go, API/schema, event framework, reporter, renderer, settings,
  lifetime cleanup, or sibling child64/65 file changes.

| Consumer/transport | Identity and revision | Behavior/evidence | Unsupported shape |
| --- | --- | --- | --- |
| All agent rows through latest WS `message.list` | Session + message ID, optional `updated_at` | Pure matrix and real deferred hook/store/renderer tests | Equal/missing/invalid follows baseline rule; no provider claim beyond shared public row |
| Authoritative core recovery | Same IDs and request baseline | Real registered callback plus pure deletion/empty controls | Existing recovery failure behavior |
| Older HTTP `before` | Session/cursor + message ID | Read-only audit: prepend rejects duplicates | No freshness upgrade or new test replay |
| Around HTTP `around` | Session/target ID + update time | Read-only audit: existing freshness helper | Existing equal/missing contract |
| Desktop/phone chat | Shared message producer/store | State-only mobile-parity exception; rendered production boundary | Reassess if presentation changes |

## UI outcome (UI-01: Existing tool row)

Entry: an open session with a tool result received while history is pending.
Shared desktop/phone row composition; existing controls and scroll owner remain.
The following text is illustrative message data, not new localized product copy.

```text
Observed before settlement: [tool] all checks passed: 42
Proposed after old history: [tool] all checks passed: 42
```

The original proof supports the first line only. The proposed second line is
the required outcome, not a measured baseline DOM state. Map to AC .1/.6 and
the rendered regression below. No structural UI change is planned.

## Tests

| AC suffix | Required meaningful evidence |
| --- | --- |
| .1 | `use-session-messages.live-refresh.test.tsx`: completed result stays rendered after older same-ID history settles; assert DOM before store. Pure tests: cached revision wins even when present before request. |
| .2 | Rendered controls: server-newer with no live change, and server-newer after an intermediate live change. |
| .3 | Pure matrix: equal instant changed/unchanged, missing/invalid on either side, invalid calendar date, timezone equivalence, nanosecond distinction; use immutable changed row references. |
| .4 | Pure overlap/disjoint cases, including same-ID arrival absent at request, distinct live additions, older out-of-order arrival exclusion, ordering/dedup/cursor. |
| .5 | Pure authoritative matching-ID freshness with an absent deleted row, older retained page, pending local row, and new arrival; normal-empty and authoritative-empty controls. Real authoritative recovery callback. |
| .6 | Real StateProvider store identity and response loading settlement; existing affected hook readiness/dedup/generation tests; no responsive producer fork. |

## End-to-end boundary and mobile parity

The regression covers the changed frontend path from a deferred transport
response through actual hook, actual store actions, subscription to stored rows,
and actual tool rendering. Mock only transport interfaces. No helper/store
stand-in, copied production reducer, renderer mock, or candidate-proof import.
Resolve every held promise and unmount before completing a case.

No new Playwright file or project is required under the mobile-parity state-only
exception: layout, touch, scrolling, navigation, and viewport-specific behavior
are untouched. Existing `tests/chat/mobile-message-pagination.spec.ts` covers
the established mobile pagination surface but is not claimed as new freshness
proof or scheduled for unrelated passing replay. Browser/server/database
coverage remains explicitly outside the test boundary.

## Public docs and decisions audit

Searched `docs/public`, root README, screenshot catalog, related specs, and
decisions for transcript, tool results, history, and refresh guidance. The
existing `docs/public/tasks-and-workflows.md` long-transcript how-to describes
navigation and scrolling; neither its controls nor operator guidance change.
No public text/screenshot/API/config update is needed. Only internal specs and
delivery records change. No ADR is needed for this local reuse of existing
revision/cache boundaries.

## Work orders

- [x] [Task 01: Preserve message revisions during latest-history reconciliation](task-01-preserve-message-revisions.md)

Dependency order: Task 01 only, sequential, no delegates.

## Execution and delivery barriers

Task `cc5337a2-aebf-4036-9b88-ba86a088b3e4`, primary session
`4279ac06-1fd4-458e-9fd9-37de3c342548`. The initial design turn performed no production/permanent test/install/heavy
check/browser/build/DB/commit/PR. The later ROOT reviewed-package implementation
interrupt granted GLOBAL LOCAL-HEAVY EXCLUSIVE66. Run heavy checks sequentially
only, with no sibling heavy overlap. Normal ready publication is authorized
after the scoped checks and active hooks. Hosted collection and merge remain
unauthorized until separate later ROOT releases.
Node 4 GiB, Vitest one worker, one conditional pnpm 9.15.9 frozen install if
dependencies are absent. Every process needs UTC/argv/cwd/log/native handle,
PID/PGID/cutoffs and actual terminal join. Resource/timeout/transport/unknown
returns to ROOT without automatic retry. Cleanup only exact owned logdir;
preserve ROOT proof, worktree, dependencies, caches, foreign processes.

The durable Kandev task plan retains routing, completion, collector, review,
serial merge, and archive constraints. Do not override those with generic
skill delivery steps. All ROOT material messages interrupt this same primary;
child callback must not interrupt or gate on ROOT's full queue. No new
task/session/tab, self-queue, goal, timer-wait, or delegation.

## Verification results

Design preflight passed:

- `python3 scripts/list-docs.py validate`: 357 decisions and 1,418 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `python3 scripts/lint-spec-files.test.py`: 36 documentation-linter tests passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage`: `covered`, `ok: true`,
  `errors: []`, with the four actual docs and one explicitly planned runtime
  path as inputs. This is reference preflight, not current-head runtime proof.
- Catalog discovers the new UI pair. `git diff --check` and explicit whitespace
  checks of all four untracked documents passed. Git status contains only the
  four unstaged/uncommitted design documents; production/permanent tests unchanged.

The initial Node command was unavailable on PATH. Existing Node 24.18.0 under
`/home/jcfs/.nvm/versions/node/v24.18.0/bin` executed the reference preflight
successfully; no tooling installation or heavy check occurred.

Implementation verification on 2026-10-07:

- Used existing mise Node 24.21.0 and verified pinned pnpm 9.15.9. The single
  conditional `pnpm install --frozen-lockfile` from `apps` succeeded.
- RED: two affected suites, 34 causal assertion failures and 28 passing controls
  (62 total). Normal refresh and registered authoritative recovery both failed
  at the post-settlement DOM assertion before any store assertion.
- GREEN: all three affected suites passed, 91 tests. A later typecheck found
  missing connection fields in the new fixture; those fields were corrected.
  Targeted lint found repeated fixture strings; constants corrected them.
  The changed pure/rendered suites then passed all 62 tests. The unchanged
  hook suite's 29 passing cases were not replayed for those fixture-only edits.
- `pnpm run typecheck` passed after the concrete fixture correction. Its
  pre-script generated ignored release-note/changelog JSON only, with no
  tracked incidental changes.
- Targeted three-file `pnpm exec eslint --max-warnings 0 ...` passed.
- `pnpm run i18n:ratchet` passed: one modified production file clean; guard
  allowlist intact. No product text or locale changes.
- Every local original invocation has exact argv/cwd/UTC/log/PID/PGID and
  immutable cutoffs under `/tmp/kandev-child66-local-20261007`, with native
  mappings in `native-ledger.json`. Tests and checks are actually joined, with
  owned process groups and wrappers physically absent before subsequent heavy
  execution. Final documentation checks and normal hook receipts are recorded
  in the work order and durable task plan.

This closes the frontend production boundary. No running server, WS dispatcher,
browser engine, or database execution is claimed. Commit/ready publication
follow normal active hooks; hosted collection, review closure, merge, and
archive remain gated on later ROOT release and are not implied by GREEN.

## Risks

- Clock regressions cannot be repaired using timestamps alone. Equal/invalid
  revisions need the request baseline, not guessed terminal-state precedence.
- Cache references must remain immutable and tied to the actual shared request.
- Preserving all cached rows would break authoritative deletion and disjoint
  pagination. Select matching row versions independently of membership.
- Store signature reconciliation can intentionally reuse equal revisions and
  preserve prompt/retention fields; pure selector acceptance is not a guarantee
  that inconsistent same-timestamp server content replaces a row.
- A test that checks the store first can fail before measuring the visible
  regression. Post-response DOM assertions must execute independently first.
