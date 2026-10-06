---
created: 2026-10-05
status: complete
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
legacy_specs: []
---

# Implementation Plan: Reset File Previews When Their Targets Change

## Overview

Reset a replaced workspace file card to its existing collapsed, lazy state.
One sequential work order first proves the defect through the real rich-output
consumer, then keys only the file-preview instance by its target descriptor.
Agents owns this repair because its existing rich-output contract owns file
references, lazy expansion, and their transcript outcomes.

## Admission and checkpoint

Current phase: IMPLEMENTATION RELEASED. ROOT reviewed the four design files and
actual renderer/hook, then explicitly granted the exclusive GLOBAL LOCAL-HEAVY
lease through real-consumer RED/GREEN, scoped gates, pinned frozen installation
if absent, normal active hooks, ready publication, and live readback. The single
work order is locally done. Ready publication is next; persistent completion
still requires verified merge and cleanup. No merge lease is granted. Return the local-heavy
lease after all local/publication handles are joined and before the sole hosted
45-minute all-terminal collector.

Historical design checkpoint: this package was unstaged/uncommitted and handed
off WAITING, with no production or permanent test change. ROOT's later release
now admits implementation; artifact creation alone did not. No delegates,
recursive tasks, sessions, tabs, or model switches. Callback queue remains full:
never retry messages/questions; save durable own-plan checkpoints; ROOT watches.

Task: `b4679619-1543-4b5b-935c-79765bdf4b55`.
Session: `b4172ff9-69e1-47be-8580-8ad68a2cc3c8`.
Parent: `14825981-b175-411d-999a-31ddc2aa5fc3`.
Branch: `feature/reset-file-previews-opr`.
Design checkout/main: `48adb0ce73960c5b4fcba968367645a5dc680b26`.

## Confirmed evidence and assumptions

ROOT's accepted direct-component proof is historical evidence, not the
permanent consumer regression. Read only; never replay or remove:

- `/tmp/kandev-rich-file-preview-target-repro.test.tsx`, SHA256
  `e8f356b92ee09bde5d05d67136d9c281175a8f4665bb93d21530cec46c40125f`.
- `/tmp/kandev-root-rich-file-preview-target-proof-receipt.json`:
  actual handle 8271 joined, exit 1, three causal target-change failures and
  one passing ordinary lazy/current-load control.
- `/tmp/kandev-root-rich-file-preview-target-repro.log`.
- Proof base: `eb589f279dc8527101b71fdaf99ce523909a5299`.

Verified once against the checkout and local `main`, all exact source blobs
still match the receipt:

| Source | Git blob |
| --- | --- |
| `use-workspace-file-preview.ts` | `d1d66daa6c70936d599c1ecddb9af7556512e2b1` |
| `file-preview-block.tsx` | `8620d09960e4c5c96940c1d6fbc72784dcf7420b` |
| `rich-output-renderer.tsx` | `053126fae9f32169057ca3900b8c91e55d43888b` |

The outer block key is type/index. Its file branch currently returns an unkeyed
`FilePreviewBlock`, so same-position session/repo/path replacement keeps local
`expanded=true`. The real hook resets to idle. `FilePreviewContent` renders idle
as unavailable despite no read of the new target. The hook already fences
outstanding settlements on unmount; the missing boundary is the whole card's
local lifetime.

Confirmed by ROOT: collapse on target replacement, preserve explicit lazy reads,
and keep metadata-only changes stable. Verified in the active requirement's
Failure modes and ADR-2026-08-14: file content is never fetched automatically.
No material unresolved product choice. This fills missing bounded acceptance
criteria .9 through .12 in the existing active requirement and current design;
it creates no incident requirement, migration, or ADR.

## Scope

### In scope

- AC-AGENTS-AGENT-RICH-OUTPUT-001.9 through .12.
- A target-derived key on the actual file-preview instance in
  `RichOutputBlockView`, using an unambiguous serialized session/repo/path tuple.
- Permanent consumer-level regression tests and the existing owning spec pair.

### Out of scope

- Automatic refetch, request coordinators, abort machinery, global caching,
  retained-callback or passive-effect redesign, hook/transport changes.
- Other block identities, chart parsing/memoization, settings, persistence,
  schema, backend, dependencies, public API/config/copy additions.
- Layout, touch targets, navigation, scrolling, breakpoint changes, app/browser
  or database allocation, broad product suites, speculative fixes.

## Technical approach

In `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx`,
give the `FilePreviewBlock` returned by `RichOutputBlockView` a key such as
`JSON.stringify([sessionId, block.repo, block.path])`. Keep the outer map keys
and non-file branches intact. Title, caption, MIME hint, callback identity, and
argument-object identity are excluded from the target tuple. Optional absent
members remain distinct from explicit empty strings; do not normalize descriptors
or invent a key helper framework.

Unmount/remount resets both disclosure and hook state together. The existing
generation cleanup discards outstanding old results. Fresh mounting performs no
read; the existing explicit expansion handler loads the current target. No
production edit to `FilePreviewBlock`, the hook, or parsing is expected.

The original rich-output and chart-performance packages are complete historical
implementation records. Their scope and results remain accurate; this repair
adds a link in the owning pair and does not reopen their work orders.

## ASCII UI preview

UI-01: same inline file card on desktop and phone; target changes at the same
block position. Illustrative localized labels, existing control order:

```text
Before replacement: [Report] alpha:a.txt  [Hide preview] [Open file]
                   old content visible
After replacement:  [Report] beta:b.txt   [Preview]      [Open file]
                   collapsed; no file read
Explicit expansion:[Report] beta:b.txt   [Hide preview] [Open file]
                   new target loading, then content or unavailable
```

The structural requirement is disclosure reset, then existing lazy expansion
(AC .9-.10). Existing phone header stacking below 420px, controls, transcript
scroll owner, and bounded preview scroll region remain as shipped. This sketch
introduces no geometry or copy. Same-target title/caption updates leave the
expanded preview visible (AC .11); sibling cards keep their own state (AC .12).

## Mobile parity and public-guide audit

Apply the narrow exception at
[mobile-parity SKILL.md line 118](../../../.agents/skills/mobile-parity/SKILL.md#mobile-e2e-expectations):
pure state/data normalization inside an existing shared component, with no
layout/touch/scroll/navigation/viewport-dependent interaction change. Actual
source evidence is `FilePreviewBlock`'s shared `article`, toggle `Button` with
`aria-expanded`, conditional preview region, `min-[420px]` header classes, and
existing file-open callback. The later tests render these real elements and
assert disclosure, visible content/loading/error, and transport calls. No
mobile browser, build, or E2E is scheduled; no new rendered behavior is claimed
as already tested in DESIGN. A causal visual change would require ROOT to
review the scope before any browser allocation.

Audited `docs/public/automation-and-mcp.md` (Native rich output), root `README.md`,
`docs/screenshots.md`, the owning requirement, and the accepted rich-output ADR.
The public guide describes lazy file expansion and existing viewer routing;
those remain accurate. No public docs change is needed, and no copy, API, or
config is added. Internal docs updated: owning requirement/design and this
package only.

## Tests

New file:
`apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx`.
Render `RichOutputRenderer` through its real renderer harness with valid
version-1 arguments, real `parseRichOutput`, real `FilePreviewBlock`, and real
`useWorkspaceFilePreview`. Mock only `getWebSocketClient` and `requestFileContent`.
Do not mock the hook, parse, target-key predicates, React, UI controls, or file
card. Use the existing locales setup and normal cleanup; no production fixtures.

| Named scenario | Bounded cases | Evidence / acceptance |
| --- | --- | --- |
| `collapses a loaded preview when its $field changes` | Parameterize path, repo, session independently; same index and unchanged title | Old content gone, `aria-expanded=false`, no additional read, explicit expansion requests exact new tuple; AC .9 |
| `discards pending old %s while the replacement is collapsed` | Deferred old success and rejection, path replacement | Loading gone on replacement; settle old promise while collapsed, then load new target and keep its result; AC .9-.10 |
| `keeps the new preview after late old %s` | Deferred old success and rejection after new preview loads | New content/disclosure survive and no old content/error appears; AC .10 |
| `preserves cached disclosure through same-target rerenders and metadata changes` | Same props, fresh args, title/caption update while loaded; also preserve an actual error through same-target rerender | No reread/reset, updated metadata, current cached content/error; AC .11 |
| `loads lazily and reuses successful preview content` | Initial collapse, successful expansion, hide/reopen; replace a collapsed descriptor before first expansion | Zero automatic reads, exact current-target request, no successful reread; AC .9/.11 |
| `retains a same-target error and retries only after explicit re-expansion` | Rejected first request, hide/reopen then success | Existing failure and retry path; AC .11 |
| `keeps duplicate-target cards independent when only the first is replaced` | Two cards with the same initial tuple; replace only first card | Second disclosure/cache survives; toggling first does not expand/reset second; AC .12 |

Additional AC .9 controls: `keeps collapsed descriptor replacement lazy until
the new target is expanded`, and `clears a previous target error without reading
the replacement`. The latter starts with a real rejected read; replacement hides
its failure and still makes no automatic request. Total new consumer tests: 13.

Do not multiply the entire matrix across all descriptors or viewports. Explicitly
settle each owned deferred promise inside React `act`; assertions use rendered
outcomes and exact read arguments, not component mount counts or computed keys.
Keep the existing isolated `rich-output-renderer.test.tsx` memoization regression
separate: it intentionally mocks parse and is not full-consumer evidence.

After release, permanent same-position replacement tests must fail causally on
current production code before the correction. ROOT's direct-component RED
does not satisfy this requirement. Then run only the new file plus the existing
renderer memoization test for GREEN. Exact commands are in Task 01.

## Work orders

- [x] [Task 01: Reset target-local preview](task-01-reset-target-local-preview.md)

One sequential order; no delegation or independent implementation wave.

## Verification results

DESIGN results (2026-10-05):

- `python3 scripts/list-docs.py validate`: exit 0; 351 decisions and 1366
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all specifications passed.
- Agents catalog lookup discovers the existing owning requirement/design pair.
- Task 01's local `validateCoverage` preflight: exit 0, `covered`, no errors;
  one work order and one current owning design. The planned renderer path is
  prospective input, not a production edit or hosted-check receipt.
- Frontmatter/reference and unstaged/untracked whitespace checks: pass; exactly
  the owning pair, manifest, and one pending work order. No staged changes.
- All design command handles terminated and were joined; no owned running
  handles remain. Workspace dependencies are absent; no install attempted.

Product RED/GREEN, eslint, typecheck, i18n, hooks, publication, and merge remain
pending and are not admitted during DESIGN.

## Implementation verification

Implementation was explicitly released by ROOT after review. The sole production
change is the target tuple key on the actual file instance; the hook, parser,
transport and card markup are unchanged.

- One frozen apps installation completed with actual pnpm 9.15.9, Node 24.21.0,
  exit 0, session 71025 joined. Root Corepack initially reported 12.4.2, but the
  apps install honored its pinned package manager; no second install occurred.
  Subsequent commands put the explicit pnpm 9.15.9 binary first in PATH.
- Permanent full-consumer RED: new suite, exit 1, session 28337 joined. Exactly
  three path/repo/session disclosure failures and one lazy/cache control pass.
- GREEN: final new suite passes 13/13, exit 0, session 54271 joined. Existing
  renderer memoization control passes 1/1 in the combined invocation. That
  invocation also reported an own-test extra closing brace; it was fixed and
  only the affected new suite rerun. Scoped ESLint's four own-test warnings
  were corrected with constants and top-level tests; final lint exits 0 clean.
- `pnpm run typecheck`: exit 0, session 63700 joined, including normal generated
  release-notes/changelog preparation. No additional tracked files changed.
- `pnpm run i18n:check`: exit 0, session 40963 joined. All catalogs/pseudo, Trans,
  plurals, module-scope translation, punctuation and non-JSX-copy gates pass.
- `pnpm run i18n:ratchet`: exit 0, session 93642 joined; changed copy and guard
  allowlist gates pass.
- `python3 scripts/list-docs.py validate`: exit 0, 351 decisions/1366 specs.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all pass.
- Repository `validateCoverage` against the actual six-path diff: exit 0,
  `covered`, no errors; owning requirement/design/work-order mapping accepted.
- `git diff --check`: exit 0. All local product handles joined; no app/browser/DB
  allocation. Normal active hooks and ready publication are next; their actual
  receipts belong in the own platform plan. Local work-order done is not
  persistent completion.

Logs are owned `/tmp/kandev-file-targets-*.log`; ROOT proof remains untouched.
No browser/build/E2E allocation, screenshot, copy, navigation or layout change.
The rendered state controls satisfy the reviewed mobile-parity line118 exception.

## Delivery after later release

ROOT owns one global local-heavy lease across ROOT and three children. Install,
tests, lint, typecheck, and active hooks run serially with every returned handle
retained and actually joined. Routine causal own-fixture/test/lint corrections
are allowed within the lease. Resource, timeout, transport, unknown-cause,
out-of-scope, or actual hosted failures require a durable WAITING checkpoint
for ROOT recovery; no blind retry, cache wipe, foreign kill, or weakened checks.

Ready PR/full delivery is standing-authorized only after release. Freeze the
published SHA except actual corrective findings; no main-drift rebase, synthetic
merged testing, or optional polish. Retain one hosted all-terminal collector
and actually join it before any specifically ROOT-authorized replacement.
Require the six actual required contexts and actual CI-parent terminal success,
exact-head error-free snapshots, zero visible/hidden/actionable threads, and no
changes requested. Require authenticated CodeRabbit App 347564 FULL substantive
CURRENTHEAD/ALLFILES evidence: inspect a completed automatic full report first;
at most one full request only for a proven gap/skip. ACK/progress is insufficient.
Ground every actual disposition; no optional additional-review wait.

Merge needs a separate ROOT MERGE lease: normal expected-head squash, no admin
or bypass. Independently verify actual merged SHA/tree/owned blobs and
authoritative remote inclusion, join all owned handles, and clean only owned
temporary artifacts. Preserve managed worktree/deps, ROOT proof, foreign
worktrees/caches/refs/processes, paused oversized child, and unproved volume.
Local work-order completion is not persistent completion: only verified merge
and cleanup completes this child; ROOT archives/removes/refills its loop.

## Risks

- Keying a wrapper above the file branch would reset unrelated blocks; key only
  the actual file instance. Delimiter-joined keys can alias legal descriptors.
- Extending the existing parse-mocked test would miss the real consumer bug;
  use the separate transport-only-mocked regression file.
- A remount intentionally loses this card's local cache and disclosure when
  the target changes; same-target and sibling controls bound that effect.
