---
created: 2026-10-06
status: implemented
requirements:
  - REQ-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001
system_design:
  - ../../specs/ui/system-design/composer-file-search-ownership.md
legacy_specs: []
---

# Implementation Plan: Keep file suggestions with their session

## Overview

Deliver one coherent sequential correction: faithful real-component regression,
local session/query cache identity plus committed-owner settlement guards, scoped
verification, and normal hosted delivery. Design ended before production/test
edits; ROOT subsequently admitted implementation in this same primary session
with exclusive local-heavy ownership. Retain that grant through normal active
hooks, commit, push, READY PR and publication joins; return before the hosted
collector. No delegation, tasks, tabs, sessions or model changes are authorized.

## Scope

### In scope

- [Session-owned file candidates](../../specs/ui/requirements/composer-file-search-ownership.md),
  all six acceptance criteria and their paired local design.
- One production file's private `fetchFileResults` / `useMentionItems` glue and
  one new actual-component/provider/editor/API/WebSocketClient regression suite.
- Narrow source/consumer audit, local receipts, lifecycle status reconciliation,
  and normal commit/push/READY PR after later release.

### Out of scope

- Legacy inline-mention production edits; inspected consumers omit session.
- General composer/draft/focus/writer/serialization changes, `#`, slash, plugins,
  backend, WebSocket API, SDK/store migrations or global request policy.
- Ranking/recency, menu geometry/layout, mobile/touch/breakpoint/navigation/copy.
- Broad local audits, Go, E2E, browser/build/runtime/mock server, shared-cache or
  lockfile/runtime edits, proof replay, unrelated cleanup or fourth task.

## Source and evidence baseline

Design HEAD: `8f533fc85a1b5f0658bae75905fc64ff80c13a87`, clean at entry, branch
`feature/keep-file-suggestion-oxt`. ROOT's accepted handle18897 was actually
joined: exit1 / 6.578s, one causal failure and one current positive PASS. Its
actual-reader fixture renders exported `TipTapInput` under real provider/editor,
API and protocol client; only deferred wire transport is replaced. Completed
alpha `@unique` showed `unique-alpha.ts`; same mounted editor cleared, committed
beta and typed identical query, but no beta request occurred (expected2actual1).
The accepted failure is absent new-session read, not an asserted backend leak or
draft/submission error. No replay is authorized.

At proof base `b84e4add2b7a76ef709ba8797d62665d06916e8a` and design HEAD,
TipTap source blob is `321fb0d10892eb67eb87953d607b474ebc220a7e` and actual wire
adapter blob is `70448f7410fd22090bf462624b15c1a8f57323c1`. Reverse-search hook
changed to `989917952ec6fe0408fcb288a2660b9477f00d95` after independent merge;
the whole dependency tree is not claimed unchanged. Derive fixtures from current
dependencies and consumers at implementation entry.

Read-only proof pointers (retain until ROOT archive release):

- `/tmp/kandev-tiptap-file-session-cache-repro.test.tsx`, regular mode0400,
  SHA256 `5039f26b00521e1b8972f3fb531d2c23833ab98638cdf2dc03ddbb382fe34224`.
- `/tmp/kandev-root-tiptap-file-cache-proof-receipt.json`.
- `/tmp/kandev-root-tiptap-file-cache-proof-classification.json`.
- `/tmp/kandev-root-tiptap-file-cache-proof.log`.
- `/tmp/kandev-root-tiptap-current-main-audit-20261006T0615Z.json`.

## Technical approach

Use a single instance-local completed entry with structured full session and exact
query identity; no empty sentinel alias or shared cache. Commit-bound owner
retirement invalidates previous pending lifetimes, including A-to-B-to-A and
unmount. A latest lookup token prevents superseded same-session replies from
overwriting current reuse. Guard both cache writes and file-candidate assembly.
Retain same-lifetime completed reuse, `query || ""` wire behavior, limit20 and
current fallback/ranking. Detailed boundaries and settlement qualification are
in the [design](../../specs/ui/system-design/composer-file-search-ownership.md).

| Path / identity | Behavior | Evidence / fallback |
| --- | --- | --- |
| Current non-null session + exact query | Real workspace.files.search, limit20 | Wire payload and actual matching menu row |
| Completed same owner/session/query | Reuse last completed files | No extra RPC and identical current row |
| New committed lifetime | Fresh read, even for same text or return to A | New wire owner and its own row |
| Retired owner / superseded lookup | No completed-cache write or retired file contribution | Deferred orders plus subsequent current reuse |
| Null / no client / current error | Existing non-file fallback, no null RPC | Actual sources/menu with real client rejection |

## Tests

One new file: `apps/web/components/task/chat/tiptap-input-file-search-ownership.test.tsx`.
Use actual exported component, provider/store, editor, suggestion and rendered
menu, real API/WebSocketClient; replace only deferred wire transport. Private
predicates and hook returns cannot be the regression seam.

| Acceptance | Named cases in the new suite |
| --- | --- |
| `.1` | Current alpha positive; completed alpha to beta same query returns beta row and excludes alpha |
| `.2` | Completed same-owner reuse; different query; empty versus literal __empty__; exact limit20 |
| `.3` | Old success before/during/after beta settlement; old error; same-session newer different query; cache-hit supersedes pending work |
| `.4` | A-to-null, null-to-B, A-to-B-to-A; no null search or retirement-triggered search/draft operation |
| `.5` | Same-session rerender reuse; independent editors; unmount; StrictMode cleanup/setup |
| `.6` | Current empty/error/no-client fallback; preserved task/Plan/prompt sources and ranking |

Mixed settlement cases use distinguishable returned paths and exact request IDs.
Reopen/retype a current query after retired settlement and assert current cache
reuse plus actual menu rows; missing API count alone is insufficient. The work
order owns exact commands, bounds, cleanup and results.

## E2E and mobile parity

The end-to-end local evidence runs from exported composer through actual provider,
editor/suggestion/menu/API/protocol client to deferred wire and back to rendered
rows. This is a pure state/data correction in the existing shared surface;
`/mobile-parity` explicitly permits targeted component tests and a written note
without mobile Playwright for this narrow scope. There is no composition/layout
preview to review. No local E2E/browser/build is admitted. Hosted E2E remains a
delivery gate. No public documentation change is needed for internal cache
intent; published `@` interaction and API remain as documented.

## Work orders

- [x] [Task 01: Keep file search with its committed owner](task-01-session-owned-file-search.md)

Exactly one work order, sequential in this primary session. Dependencies: later
explicit ROOT implementation release; then local checks; then hosted review/CI;
then separate ROOT serial merge grant. No agent/model roles are prescribed.

## Resource and checkpoint contract

GLOBAL LOCAL-HEAVY NONE and MERGE NONE at design. Later ROOT owns exclusive local
heavy admission and the continuous three-child loop. Conditional frozen install
once only after release, project pnpm9.15.9, Node24 with command-local PATH in
Bash login=false; retain dependencies/lockfile/shared caches. Serial checks,
one Vitest worker/no file parallelism, Node4GiB and original120s test bound.
Record command/cwd/head/start/cutoff/handle/PIDs/group/log/exit/join/gone receipts;
actually join every retained handle and prove owned processes gone before
returning local-heavy. No automatic budget reset, timeout retry, accepted proof
replay or rebase solely because main advanced.

Routine own fixture/lint repairs may rerun only affected checks. Resource,
timeout, transport, unknown, or scope findings checkpoint ROOT with actual
failure receipt before recovery/expansion. The notification/question queue is
full: persist the actual checkpoint in task plan/conversation and END WAITING;
do not retry messages/questions. Preserve user edits with version-safe plan
operations. A crash or absent receipt is NO VERDICT.

## Hosted delivery and merge gates

Standing delivery authorization applies only after implementation admission and
successful task checks. Use normal active hooks, a new Conventional Commit,
push and READY PR; no bypass/amend. Freeze SHA except actual correction. Read the
current commit/push/pr/pr-fixup skills and `.github/AGENTS.md` at that phase.

Own one original45m all-terminal `scripts/pr-await` collector, with upfront handle,
PIDs/start/absolute cutoff/watchdog/log. Actually join and prove it gone before
any ROOT-specific replacement; no manual GitHub timer polling or duplicate
monitor. Require authenticated configured CodeRabbit App347564 substantive FULL
EXACT-HEAD ALL changed-file review (source=covered=head, kind=reviewed); accept
sufficient automatic review, at most one necessary full request after a real
completed skip/gap. ACK/progress is not the gate. No optional duplicate review,
Claude wait or cosmetic head polish. Ground disposition of every actionable
thread/grouped finding.

Before merge-ready END require all six actual SUCCESS: SHA-pinned refs,
architecture, E2E passed, Frontend passed, harness lint, Run Backend Tests; also
actual Backend/Frontend/E2E parents SUCCESS. Obtain fresh complete exact-head
report, errors[], governance/resolver/hidden/actionable/changes-requested/human
empty and all joins. No hosted retry without a specific ROOT workflow/job-NAME
grant; never rerun passing/whole/unverified checks or reset the budget.

ROOT alone grants serial merge after its static main/head/blob audit. Only then
use normal expected-head squash with no admin/bypass. Verify actual mergeSHA,
tree and every owned blob, remote-main inclusion and all joins independently.
Clean only owned temporary resources; retain platform worktree/deps/evidence/
parent proof for ROOT archive. Completion requires actual verified merge plus
joins. There is no fourth task or foreign resource cleanup.

## Verification results

Design light checks passed on 2026-10-06 at the stated HEAD: catalog validation
(351 decisions / 1373 specifications), all-file specification lint, and new-pair
catalog discovery. Dependency-free Node24 coverage validates the actual four-file
design inventory as exempt, errors[]; a separately labelled prospective source
scope reports covered, errors[], with the work-order references accepted. This
does not claim actual production/PR coverage. Coverage receipt:
`/tmp/kandev-child46-file-search-design-coverage.json`. Final whitespace, local
reference, diff/checksum and preservation receipts are saved in
`/tmp/kandev-child46-file-search-design-handoff.json` at handoff.

These are historical design receipts. Later admitted implementation changed
only the one private production glue file and its new faithful regression suite.
The requirement is active and design current. Local implementation verification
and delivery results are recorded below; hosted/merge completion is separate.

### Implementation checks (2026-10-06)

One missing-dependency frozen pnpm9.15.9 install passed in 2.266s. Permanent
causal RED after owned draft-fixture isolation: 8 failures / 11 passes, including
current alpha positive and missing beta expected2actual1. The first attempt was
fixture contamination, not claimed causal evidence. Final suite: **21 passed**
in 6.667s after final fixture type repair, one worker/no file parallelism under
the original120s cap. Current production SHA256:
`1c15c762e87e14e9f6ea15389fae5e9a7e7e932de071ae2b7a093b2a865aa6e4`.
Current test SHA256:
`d79da136415296dde1943df1ba86f599147caf7ec74fccd50686e5fbd7842a2b`.

Scoped ESLint passed: final production and initial formatted tests together,
then affected-test-only after adding empty retired replies and required fixture
workflowId. Normal typecheck (including pretypecheck) passed in 8.665s after a
single owned fixture missing-workflowId diagnostic; first failed typecheck was
61.677s within180s, not a resource/timeout failure. i18n:check passed in 9.118s;
i18n:ratchet passed in 1.750s. Every retained native handle was actually joined
and every owned PID/group/watchdog was proved gone before the next heavy run.
No production edit followed these final product checks.

Individual actual command/cwd/HEAD/input-hash/start/cutoff/PID/group/log/exit/join
receipts and native-handle sidecars are `/tmp/kandev-child46-<label>.json` and
`-handle.json`, labels: install, red, red-fixture-repair, green, eslint,
format, eslint-repair, green-final, eslint-final-test, typecheck,
green-fixture-type-repair, eslint-fixture-type-repair, typecheck-fixture-repair,
i18n-check, i18n-ratchet. Routine repairs were limited to owned fixture/lint.
No root-proof replay, new framework, source expansion, broad suite/browser/E2E/
build, global runtime/cache change, retry of a timeout, or budget increase occurred.

Light final docs/reference/actual six-path coverage/whitespace passed in 1.856s:
catalog351 decisions/1373 specifications, all-file spec lint, pair discovery,
actual six-path documentation coverage `covered`, errors[], all work-order
references, untracked-aware whitespace, links and proof preservation. Receipt:
`/tmp/kandev-child46-docs.json`; actual inventory/coverage:
`/tmp/kandev-child46-local-contracts.json`. All handles joined, owned group gone.
The local work order is done and plan implemented; publication/hosted/merge
remain separate gates. Normal hook/commit/push/READY PR publication is pending.
No hosted collector/review, CI verdict or merge is claimed yet.

## Risks

- A session-only cache key fix does not prevent pending old writes or retired
  candidate assembly; both guards require faithful deferred tests.
- Generic TipTap async menu lifecycle is a separate boundary. A demonstrated
  causal popup problem beyond qualified file ownership must checkpoint ROOT.
- Commit retirement must preserve ordinary rerender reuse and StrictMode usability.
- Fresh worktree dependencies may be missing; installation is barred this turn.
