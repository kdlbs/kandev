---
id: "01-preserve-message-revisions"
title: "Preserve message revisions during latest-history reconciliation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-HISTORY-FRESHNESS-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.1
  - AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.2
  - AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.3
  - AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.4
  - AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.5
  - AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.6
system_design:
  - ../../specs/ui/system-design/transcript-history-freshness.md
---

# Task 01: Preserve message revisions during latest-history reconciliation

## Summary

Implement the existing request-baseline and update-time selection contract for
matching fetched IDs. Prove that actual rendered tool results survive an older
history response and that a newer server result still replaces cached state.
ROOT reviewed the sealed package and released this work order for implementation
in the same primary session on 2026-10-07. GLOBAL LOCAL-HEAVY EXCLUSIVE66
remains in force until the physical local return; hosted collector and merge
require later separate ROOT releases.

## In scope

- Own the latest-window selector and its pure revision/membership matrix.
- Author an independent actual hook/store/renderer regression, with transport
  mocking only. Include normal refresh and authoritative recovery coverage.
- Preserve all existing metadata, cursor, request, loading, and lifecycle guards.
- Keep the requirement/design/manifest/results synchronized after actual checks.

## Out of scope

- Protected ROOT proof replay/copy/import/mutation/deletion.
- Backend/API/schema, WS scheduler/reducer redesign, global version tracking,
  around/older-page policy changes, reporter changes, lifetime cleanup.
- Layout, mobile interactions, product copy, browser builds, DB execution,
  sibling child64/65 ownership, optional polish, broad verification.

## Acceptance

1. Matching rows in overlap, disjoint, and authoritative paths satisfy the
   strict timestamp/request-baseline matrix; newer server rows still win.
2. A real rendered regression measures the retained completion after response
   settlement before any store assertion can skip that measurement; controls
   prove server-newer application and distinct live-addition retention.
3. Pure mixed-row controls prove contiguous cursors, authoritative removal/empty
   behavior, optimistic retention, deduplication and creation ordering. Actual
   hook/store integration preserves identity, loading, and shared-request guards.

## UI-01: Existing tool row

See the [full plan](plan.md#ui-outcome-ui-01-existing-tool-row). Desktop and
phone share the producer and existing row composition; no structural changes.
The required post-settlement state (AC .1/.6) is:

```text
[tool] all checks passed: 42
```

Illustrative message data only. The original proof did not measure this DOM
state after settlement. The new regression must do so.

## TDD sequence and exact test responsibilities

1. Read the paired specs, source, and nearby tests. Reconfirm the baseline and
   original shared-request snapshot; mark this work order `in_progress` only
   after ROOT release. Do not move/rebase to a newer main for convenience.
2. Add `use-session-messages.live-refresh.test.tsx` independently. Mount actual
   `StateProvider` (hence `createAppStore`) and `ToolCallMessage` with the actual
   hook and `ToastProvider`. Mock only WS connection/request/readiness and
   session-turn HTTP transport. Capture the real store through `useAppStoreApi`.
   Hold `message.list`, use actual `updateMessage` to complete the running tool,
   verify the output rendered, then settle the older response. Await
   `historyRefreshPending === false` and assert retained visible output/absence
   of old running title before asserting selected row content/status/time.
3. Add rendered controls for a newer server result without live activity, a
   newer server result after an intermediate real live update, and a distinct
   real `addMessage` during the read. Exercise the registered core recovery
   callback with a deferred authoritative response after initial hydration.
   Confirm provider store identity and request counts while held. Resolve all
   transport promises, dispose/unmount, and restore test state.
4. Extend `message-window-reconciliation.test.ts` with the design matrix across
   all merge branches. Include same-ID arrival absent at request in disjoint
   and authoritative cases; changed and unchanged ties; missing/invalid on
   either side; `"0"` and February 30; timezone-equivalent instants; nanosecond
   revisions within one millisecond. Use genuinely distinct immutable row
   objects for concurrent changes. Include mixed matching/absent/older/pending/
   new rows and both empty policies. Retain existing ordering/cursor assertions.
5. Run the targeted regression to RED and record causal assertion failures, not
   infrastructure errors. Then add the smallest local selection helper/map in
   `message-window-reconciliation.ts`, calling the existing strict timestamp
   helpers without changing their contracts. Keep branch membership filters,
   empty paths, creation order and cursor derivation intact. Run affected checks
   to GREEN sequentially; no unrelated passing replay or optional refactoring.

## Verification

Design turn: cheap documentation/source preflight only. After ROOT grants local
heavy execution, first inspect dependencies; if absent, run once from `apps`:
`PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" NODE_OPTIONS=--max-old-space-size=4096 pnpm install --frozen-lockfile`.
Do not reinstall an existing managed workspace. No automatic
retry for resource, timeout, transport, or unknown failures.

Run each following command in its own recorded process, sequentially. Record
start/end UTC, exact argv/cwd, owned log, handle, PID/PGID, immutable cutoff,
terminal exit and physical closure. A deadline is not a pass. First RED can
select the causal new tests with `-t` to keep it narrow; final GREEN covers the
listed affected boundary suites once.

```bash
(cd apps/web && PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run --maxWorkers=1 hooks/domains/session/message-window-reconciliation.test.ts hooks/domains/session/use-session-messages.live-refresh.test.tsx hooks/domains/session/use-session-messages.test.ts)
(cd apps/web && PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" NODE_OPTIONS=--max-old-space-size=4096 pnpm exec eslint --max-warnings 0 hooks/domains/session/message-window-reconciliation.ts hooks/domains/session/message-window-reconciliation.test.ts hooks/domains/session/use-session-messages.live-refresh.test.tsx)
(cd apps/web && PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" NODE_OPTIONS=--max-old-space-size=4096 pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The shell initially lacked Node on PATH. Cheap preflight found the existing
Node 24.21.0 and pinned pnpm 9.15.9 binaries above; no installation was performed. Set that
process-local PATH for each future command, without changing global shell setup.
The existing hook suite exercises the same changed reconciliation boundary;
around/older pagination, Go, broad Vitest/lint, Playwright, and builds are not
scheduled. If a corrective implementation finding changes that scope, record
it and checkpoint ROOT before adding heavy work. The typecheck pre-script
generates release-note/changelog artifacts; inspect and record its actual
effects, do not silently commit incidental generated changes.

Use `.github/scripts/pr-docs.cjs` `validateCoverage` for local reference
preflight with current document contents. Design-only calls can list the planned
runtime path as a coverage input, clearly distinguished from an actual change;
delivery must use the real changed paths/current head. Validate all six ACs
against the owning requirement, all REQs against design frontmatter, and the
work-order design against the manifest. Check untracked work orders with
`git status --short -- docs/plans/preserve-live-tool-results`.

## Files likely touched

- `apps/web/hooks/domains/session/message-window-reconciliation.ts`
- `apps/web/hooks/domains/session/message-window-reconciliation.test.ts`
- `apps/web/hooks/domains/session/use-session-messages.live-refresh.test.tsx` (new)
- Paired freshness requirement/design and this plan/work order.

Read-only dependencies: `use-session-messages.ts`, StateProvider/store/session
actions/message-signature, strict message-timestamp helpers, WS message
handlers, tool renderer, older/around loaders, existing hook tests. Do not edit
those producers merely to add a test seam.

## Dependencies and parallelism

No prior work order. ROOT implementation/resource release received for this
work order; local execution is exclusive and serial.
`sequential`; no delegates or new persistent tasks/sessions.

## Risks

Reference equality detects immutable changes, not event origin. Do not deep
clone the request baseline. Equal timestamps are not proof of different server
content; existing store identity rules remain. Preserve full accepted rows,
keep membership independent, and test authoritative emptiness explicitly.

## Inputs

- [Requirement](../../specs/ui/requirements/transcript-history-freshness.md), all ACs.
- [Design](../../specs/ui/system-design/transcript-history-freshness.md), revision matrix and scope audit.
- [Plan](plan.md), protected-proof limits and public-doc/mobile audit.
- Durable Kandev task plan for all ROOT routing, heavy/delivery/merge/cleanup barriers.

## Results

Implemented the local selector in `message-window-reconciliation.ts`; no other
production file changed. Actual hook/store/renderer tests exercise normal
refresh and the registered authoritative callback, with DOM-first measurements.
Pure cases cover all three merge branches, the revision matrix, mixed-row
membership, empty/deletion/pending policies, ordering and cursors.

Validation executed serially with existing mise Node24.21.0, Node4GiB and
Vitest `--maxWorkers=1`:

- Pinned pnpm9.15.9 verified; dependencies were absent, so exactly one frozen
  install from `apps` ran and passed.
- Original RED command (pure and rendered files) exited 1: 34 causal failures,
  28 passing controls, 62 total. Actual post-settlement DOM failures occurred
  in normal and authoritative refresh; newer-server and live-addition controls
  passed. No protected ROOT proof was copied, imported, or replayed.
- Full affected GREEN command in the verification block passed 91 tests across
  three suites. The first typecheck subsequently reported missing required
  connection fixture fields. Correcting the fixture yielded 5/5 rendered tests.
- The first targeted lint reported duplicate fixture strings. After extracting
  constants, the exact targeted lint passed with zero warnings, and the final
  changed pure/rendered suites passed 62/62. The existing unchanged hook suite's
  29 cases were covered by full GREEN and not replayed for fixture-only changes.
- Corrected `pnpm run typecheck` passed. Generated release-note/changelog JSON
  are ignored artifacts and introduced no tracked changes.
- `pnpm run i18n:ratchet` passed (one modified production file clean, 643
  allowlist entries intact).
- Final fixture formatting changed line wrapping only; the affected renderer
  suite was then run again and passed 5/5 without replaying unchanged suites.
- `python3 scripts/list-docs.py validate` passed (357 decisions, 1,418 specs).
  `python3 scripts/lint-spec-files.py --all` passed. Actual changed-path
  `.github/scripts/pr-docs.cjs` reference preflight returned `covered`,
  `ok: true`, `errors: []`. `git diff --check` passed.
- Normal active pre-commit and commit-msg hooks remain required before
  publication; their original receipts are retained in the durable task plan.

Original process records and logs: `/tmp/kandev-child66-local-20261007`.
`native-ledger.json` retains the original native session/chunk mapping for each
invocation; per-command JSON retains actual joins, physical group closure and
immutable cutoffs. These are owned evidence, not default cleanup targets.

No Go, browser/build/server/dispatcher/database, unrelated-suite or hosted
collector run occurred. The design turn itself remained design-only. Later
ROOT release authorized this implementation and normal ready publication;
hosted collector and merge still require distinct releases. Overall task
completion remains gated on actual verified normal merge and joined closure.
