---
id: "01-admit-draft-clearing-by-visit"
title: "Preserve drafts across accepted send settlement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.3
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.4
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.5
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.6
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 01: Admit Draft Clearing by Committed Visit

## Summary

Prevent an accepted send from an ended composer visit from erasing a successor
draft, including identical text and return visits. Add permanent real-editor
regressions, then enforce the local committed-visit boundary in the shared
hook while preserving existing successful/failed/snapshot behavior.

## Authorization checkpoint

ROOT reviewed the six-file sealed package and explicitly released implementation
in this same primary on 2026-10-07, with exclusive local-heavy capacity after
child64's independently accepted physical return. No worker delegation,
additional sessions/tabs or model changes.
The platform task plan retains identity, resource and later delivery barriers.

## In scope

- Independently author production-hook/real-editor/provider/store/persistence
  tests in `chat-input-draft-ownership.test.tsx`; execute behavioral RED first.
- Local rendered-owner plus committed-visit admission in `useChatInputState`,
  before submission ref reads and before any accepted clear side effect.
- Retained `handleSubmit`/`clearAcceptedPayload` callbacks; current accepted
  opening payload remains compatible, stale clearing returns `false`.
- Actual rendered `ChatInputContainer` submit entry point with its real body,
  hook and TipTap; destination draft persistence and remount restoration.
- Current success/false/rejection/newer text, attachment snapshot, readiness,
  generated payload and independent different-session composer/store controls.
- Requirement/design/manifest/result updates; preserve prior performance
  investigation status and verification record.

## Out of scope

Backend/API/WS/transport policy, cancellation/replay, public handle shape,
new payload ownership fields, global coordinator, cache/context rewrite,
upload lifecycle repair, speculative send/toast defect, new copy/layout/flags,
dependency changes and whole application/browser claims. ROOT's proof archive
is read-only evidence and must never be replayed/copied/imported/mutated/deleted.

## Acceptance

1. Both A→B and B→A identical pre-existing draft cases fail for live/editor/
   saved/remount data loss before production changes, then pass with actual
   hook/editor/store/provider. A-B-A, same-ID committed replacement, retained
   callbacks, unmount and StrictMode cleanup cannot revive old clearing work.
2. Unchanged current success and accepted-payload matches clear correctly;
   false/rejection and newer text preserve; attachment-change text-only clear,
   exact outgoing snapshots, readiness and context controls still pass.
3. Actual container submit-control coverage uses the real editor and persistence;
   independent different-session composers/stores do not cross-clear. All
   required checks finish with original process receipts and truthful results.

## Implementation sequence

1. Read the reviewed pair, manifest and scoped chat/web guidance. Mark this
   work order `in_progress` only after both authorization gates are met.
2. Install pinned workspace dependencies once if missing, using Node 24 and
   `apps/package.json`'s `pnpm@9.15.9` pin. Do not change lockfiles or caches.
3. Write the new fixture independently from the requirements and existing
   repository conventions. Real StateProvider creates the app store; use real
   Toast/Tooltip providers as required. Never mock TipTap, storage, core hook,
   `clearSubmittedInput` or visit admission. Only the submit callback is held.
   Seed non-null rich JSON and ready attachment descriptors for storage cases.
4. Run the new regression file RED; record actual causal assertion failures
   and current-owner/rejection controls. Setup/resource/timeout failure is not
   RED. Checkpoint ROOT for unknown/resource/timeout; do not auto-retry.
5. Add the smallest local owner admission. Associate callbacks with their
   rendered task/session identity, publish/retire committed tokens in layout
   lifecycle, and capture active admission before calling the submitter.
   Never adopt a new token from an old callback or revive a retired token.
   Check admission before all clear setters, actual ref, reset and storage
   calls. Current content comparisons remain verbatim in behavior.
6. Add/run lifecycle and payload controls from the manifest. Include synchronous
   acceptance and a submitter that commits replacement before returning true.
   Assert no send by a retained ended-visit submit callback. Test StrictMode
   cleanup/setup with an outstanding operation, not merely a StrictMode wrapper.
7. Execute the listed scoped GREEN commands serially. Correct narrow fixture/
   lint findings without scope expansion, then record actual commands/counts/
   exit statuses and mark task `done` only when all required evidence is complete.
8. Join all owned processes and prove them gone before physical HEAVYRETURN.
   Later publication/hosted wait/merge follow the platform plan's separate gates.

## Verification

Run from repo root through Bash with `login=false`. Each operation is serial and
needs an owned log/receipt with UTC, argv, cwd, original native handle/chunks,
PID/PGID and cutoff. The following GNU cutoffs bound the inner operation; retain
and actually join its original native session with an outer allowance beyond
the kill grace. Keep `NODE_OPTIONS` at 4 GiB and Vitest at one worker. Do not
infer completion from another run or fabricate unavailable metadata. If any
operation times out or is resource/unknown, stop at ROOT checkpoint without a
successor/retry/cache wipe/foreign kill or weakened gate.

First, only if dependencies are missing (one install; package pin is 9.15.9):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
(cd apps && timeout --kill-after=10s 10m pnpm install --frozen-lockfile)
```

RED, before changing production code (the file is created by this work order):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
(cd apps/web && timeout --kill-after=10s 5m pnpm exec vitest run --maxWorkers=1 components/task/chat/chat-input-draft-ownership.test.tsx)
```

GREEN and existing causal controls after implementation:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
(cd apps/web && timeout --kill-after=10s 5m pnpm exec vitest run --maxWorkers=1 components/task/chat/chat-input-draft-ownership.test.tsx components/task/chat/use-chat-input-state.test.ts components/task/chat/use-chat-input-state-accepted-payload.test.ts components/task/chat/use-chat-input-container.test.ts components/task/chat/use-tiptap-editor.test.ts components/task/chat/chat-input-area-context-submission.test.tsx)
(cd apps/web && timeout --kill-after=10s 5m pnpm exec eslint --max-warnings=0 components/task/chat/use-chat-input-state.ts components/task/chat/use-chat-draft-visit.ts components/task/chat/chat-input-draft-ownership.test.tsx components/task/chat/chat-input-draft-ownership.test-helpers.tsx)
(cd apps/web && timeout --kill-after=10s 10m pnpm run typecheck)
(cd apps/web && timeout --kill-after=10s 5m pnpm run i18n:check)
(cd apps/web && timeout --kill-after=10s 5m pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/composer-draft-settlement-ownership
```

If lint requires a local helper/test-helper file, add only that scoped path to
the lint command and list it in Results. Do not allow new warnings through a
gate exception. Typecheck's prehook generates tracked ignored build-support
files; inspect status and preserve unrelated state. No build or new Playwright
is planned: this is the documented pure-state mobile exception. If distinct
body branches exist, the new actual-container test covers desktop/phone/tablet
conditions; do not substitute mocked layout tests for the shared editor path.

Documentation coverage preflight (standalone reference check, no network):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/composer-draft-settlement-ownership/plan.md',
  'docs/plans/composer-draft-settlement-ownership/task-01-admit-draft-clearing-by-visit.md',
  'docs/specs/ui/requirements/session-refresh-efficiency.md',
  'docs/specs/ui/system-design/session-refresh-efficiency.md',
];
const result = validateCoverage({
  changedFiles: [...paths, 'apps/web/components/task/chat/use-chat-input-state.ts']
    .map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

At design time this is a representative future production-path documentation
check only, not implementation/test evidence. At implementation, include all
actual changed work orders and required linked artifacts in this preflight.

## Files likely touched

- `apps/web/components/task/chat/use-chat-input-state.ts` (production owner).
- `apps/web/components/task/chat/use-chat-draft-visit.ts` (local admission and
  unchanged attachment snapshot/text-storage clear, extracted to meet the
  existing file limit).
- `apps/web/components/task/chat/chat-input-draft-ownership.test.tsx` (new real
  hook/editor and rendered container tests; optional narrowly scoped test helper).
- `apps/web/components/task/chat/chat-input-draft-ownership.test-helpers.tsx`
  (real production fixture composition, extracted to meet the test file limit).
- The owning requirement/design and this package's manifest/work order Results.
- Existing accepted-payload/state tests only if required for focused controls;
  they cannot replace actual-editor coverage.

## Dependencies

None. Later ROOT implementation authorization and exclusive local-heavy lease
are operational gates, not another work order.

## Risks

Render-time or passive ownership, revived tokens, old callbacks adopting new
tokens, false claims from mocked editors, and widening same-visit snapshot
semantics. See the manifest's concrete lifecycle risks and consumer audit.

## Parallelism

`sequential`

## Inputs

- [Owning requirements](../../specs/ui/requirements/session-refresh-efficiency.md),
  `004.3`–`.6`.
- [Paired design](../../specs/ui/system-design/session-refresh-efficiency.md),
  Composer draft settlement ownership.
- [Manifest](plan.md), evidence/scope/test matrix/mobile/public-doc assessment.
- Production hook, TipTap imperative persistence, local-storage helpers,
  existing hook/accepted-payload/context tests and scoped chat/web AGENTS.
- Parent-owned protected proof and supplied receipts, read-only inspection only.

## Results

Historical design checkpoint: no permanent test or production edit, install,
RED/GREEN, lint/typecheck/i18n, browser/build or publication had been run by
CHILD65.

Design reference checks on 2026-10-07 passed: catalog (357 decisions/1416 specs),
36 standalone spec-linter tests, all-spec lint, whitespace including untracked
Markdown, links/AC references and one-work-order inventory. Standalone
PR-documentation preflight reports `covered`, `ok: true`, `errors: []` for the
representative future hook path; this is not implementation evidence. Archive
hash remained unchanged; all changed files were documentation and unstaged.
ROOT's later implementation authorization and exclusive heavy grant arrived
on 2026-10-07. The one implementation work order is complete.

### Implementation results

Added local layout-lifecycle admission with a rendered task/session owner and a
fresh committed token. Both invocation and accepted settlement must retain that
admission. Cleanup, replacement and return visits retire outstanding work;
abandoned renders do not retire the still-committed visit. Existing text and
ordered attachment comparisons, payload generation and persistence shape remain.
The small local helper includes the original unchanged attachment snapshot and
text-storage clear functions; real fixture composition is in the scoped test
helper to satisfy existing file/function limits without exemptions.

Independently authored production-hook/TipTap/provider/store/sessionStorage
regression: initial RED had eight causal failures and six controls passing
before production edits. Final GREEN passes all 27 new cases, including both
session directions, A-B-A, keyed replacement, unmount, retained callbacks,
StrictMode cleanup/setup, null-session/task ownership, a suspended replacement,
current acceptance/rejection/new typing, attachment/readiness/exact rich context
payload, different-session independent composers/stores, and actual rendered
container submission at 1280 px, 390 px and 840 px with coarse pointer.
Stored non-null rich content and remount assertions verify preservation.
Only acceptance is deferred; no core hook/editor/store/storage mock is used.

Final six-suite GREEN: 116/116 tests, 11.40 seconds. Native session 17668,
start chunk 9fef83, terminal 6a9f3e, actual exit 0; wrapper 835804 and command
PID/PGID 835805 joined and physically gone. Initial RED: native 11588,
500c6d/535b6d, actual exit 1; wrapper 799127 and command 799128 likewise gone.
An expanded intermediate run exposed two fixture errors (legacy incomplete
attachment data and coarse-pointer simulation); their narrow corrections are
retained in failed receipt/log `green-final`, not misclassified as product RED.
Lint file-size and fixture issues were corrected locally, with no rule bypass.
The first normal commit hook classified the helper's synthetic draft as copy;
the fixture constant now carries the documented non-copy annotation. No product
string, linter configuration or localization gate was changed.

Required checks returned exit 0:

- Scoped ESLint `--max-warnings=0` on the four changed chat TS/TSX paths:
  native 19220, ac247b/8cde0a; no warnings or errors.
- `pnpm run typecheck`: native 16313, c3b63e/87882e; generated prehook artifacts
  do not add tracked changes.
- `pnpm run i18n:check`: native 81282, da9e90/1f7430; catalogs complete and
  pseudo in sync. Existing 434 orphan-key notices are informational.
- `pnpm run i18n:ratchet`: native 58602, 77fcef/232376; one modified tracked
  source clean, allowlist intact. Normal staged commit hooks also cover new files.
- Documentation catalog: 357 decisions/1416 specs; 36 standalone spec-linter
  tests; all-spec lint. Actual ten-file PR reference preflight is `covered`,
  `ok: true`, `errors: []`, with both changed work orders accepted.

All these original processes were actually joined and their command/wrapper
PID and process groups proven gone. Full UTC/argv/cwd/cutoff/native-chunk/process
metadata and raw logs are retained as `/tmp/kandev-child65-<operation>.json`
and `.log`; no native handle is invented for operations completed in one call.
One pinned pnpm 9.15.9 frozen install was performed in `apps/`; lockfiles and
shared caches were not changed. ROOT's protected archive was neither executed,
copied/imported nor modified.

No default transport, WebSocket, full-browser, visual, upload-lifecycle or
performance run is claimed. The pure-state mobile/public-doc assessments remain
applicable; no layout/copy/workflow change or Playwright/screenshot update is
needed. Normal-hook publication and hosted review/CI evidence, frozen head and
physical heavy return are tracked in the platform task plan. Merge requires a
later separate ROOT serial lease.
