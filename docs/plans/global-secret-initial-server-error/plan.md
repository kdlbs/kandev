---
created: 2026-10-12
status: implemented
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-002
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
legacy_specs: []
---

# Implementation Plan: Global initial server-error settlement

## Overview and checkpoint

Preserve accepted current Global metadata when an older initial HTTP list read completes
with 5xx. Exactly one sequential work order extends the existing immutable-reference fence
to the Global catch, with independent causal and compatibility evidence.

Execution released on 2026-10-12 after FULL ROOT actual-file review and a LATER explicit
implementation interrupt to the SAME primary. Exclusive global local-heavy lease granted.
Task `2fef1dc7-ebaf-4d6b-9644-7005b589ba4d`, session
`668dcc3a-3293-41e4-9efd-79643e6209f0`; retain the selected profile. No delegates, recursive
tasks, sessions/tabs, worktrees, or model changes. ROOT reads directly; callbacks are not gates.

## Evidence, ownership, and assumptions

ROOT supplied independent qualification at `ec024742`, with owner/dependency sources unchanged
at actual main `d6887b98`: the actual Add secret form/shared Save accepts POST200 metadata,
and a registered `secrets.created` event renders it. An older pending initial GET500 then
reaches unconditional Global `catch(() => setSecrets([]))`, clearing current store/displayed
metadata and settling loaded=true/loading=false, which suppresses later initial fetching.
This loses metadata, not server-side data or plaintext. Historical native38646 actually
joined exit 1 (receipt c0879d): one causal RED/four passing controls, 9.82s; ROOT reported
checksum-proved scratch removal and current owned-group absence. These are supplied ROOT
receipts, not child executions. Never read/copy/delete the private fixture
`/tmp/kandev-root-global-secret-failure-20261012.kmhu58ka/candidate.test.tsx`, ROOT scratch,
or proofs; no root-probe replay. Independently author tests from public actors/patterns.

Read-only child checks confirm HEAD and remote main at
`d6887b98b60f530b7e52d5d874019ea1a960550a`. Workspaces owns this extension because the same
repository-secrets pair owns Global metadata publication, scope, and list lifetime.
Adjacent UI, Integrations, Platform, and Tasks indexes do not own this metadata contract.
The intended error class, quantifier, exclusions, and controls are explicitly settled;
no material assumption blocks planning. A different error policy must checkpoint ROOT.

The completed [Global success package](../preserve-saved-global-secrets/plan.md),
[Workspace isolation](../workspace-secret-list-isolation/plan.md), and
[Workspace publication](../preserve-saved-workspace-secrets/plan.md) remain historical delivery
records. Their failure exclusions describe those packages; the newly qualified boundary
belongs to this companion. Do not reopen their completed work or copy their proof fixtures.

## Scope and technical approach

Only production ownership: `apps/web/hooks/domains/settings/use-secrets.ts`, Global catch.
Use existing `ApiError` from `apps/web/lib/api/client.ts`. `listSecrets` already calls
`fetchJson` without retry. At catch completion synchronously read the current store's
items and preserve them through unchanged `setSecrets` only for an actual `ApiError`
with `500 <= status < 600` AND changed items reference versus admission. The small file-local
unexported pure `failureItems` selector owns that predicate; ROOT approved this required
function-length lint extraction after the original 110-test GREEN. Otherwise clear
with `setSecrets([])`. Keep read/compare/set synchronous and existing `finally` unchanged.
The setters settle loaded/loading without retry, new error state, or trailing fetch.

Qualification is existential: at least one accepted mutation changed the immutable array
reference while the read was pending. Retention is universal: preserve the complete current
inventory's identity, membership, order, and fields, including untouched rows in a mixed
list, deleted-row absence, duplicate-event deduplication, and accepted empty inventory.
Do not predicate preservation on nonempty length, one saved ID, equality by value, timestamps,
or loaded flags. Preserve the successful reference fence and synchronous admission gate.

| Completion / current inventory | Behavior | Primary evidence |
| --- | --- | --- |
| HTTP500–599, intervening accepted reference change | Retain exact current array; settle shared readiness | Mounted causal test; mixed/two-consumer hook cases |
| HTTP500–599, unchanged reference | Clear and settle once | Uncontested GET500 control, also with seeded rows |
| HTTP4xx, changed or unchanged reference | Existing clearing; preserve 401 challenge semantics | Mounted GET403 control; hook 401/4xx cases; real client suite |
| Network/abort/malformed200/plain lookalike error | Existing clearing | Real-client downstream hook cases |
| Successful initial GET | Existing publication fence and filtering | Existing Global publication suites |
| Already loaded Global create | Accepted row; zero initial GETs | Real form/save control |
| Workspace callbacks/list lifetime | Existing policy | Existing Workspace publication/lifetime suites |

No backend/API/reveal/encryption/binding/runtime changes, plaintext exposure, generic
retry/refresh/lifecycle hardening, store action/type change, journal/cache/version/framework,
new production helper module, dependency/config/lockfile changes, markup/copy/operator step, browser,
build, or full-suite checks. Providers, API adapters, store, and handlers are read-only.

## Tests and traceability

The independent tests below are implemented and passed; actual results are recorded below.
Real `StateProvider`/Immer store, `SettingsSaveProvider`, `ToastProvider`, `SecretsSettings`,
form/row widgets, `useSecrets`, APIs and `registerSecretsHandlers` stay real. Stub only
external `fetch`, with valid Responses and rejection on unexpected routes/methods. Capture
the actual owning store through public `useAppStoreApi`. Dispatch metadata through the
registered handlers. Settle all owned deferreds in cleanup; restore fetch/auth callbacks.
No artificial sleeps, private fixtures, API-function mocks, or production-only test seams.

New suite `apps/web/components/settings/secrets-settings.initial-server-error.test.tsx`
uses independently authored, test-only
`apps/web/components/settings/secrets-settings.initial-failure.test-helpers.tsx` for narrow
transport/provider/form composition following the public existing Global-publication suite.
The helper owns no application behavior and may not become a generalized harness.

| AC (full prefix AC-WORKSPACES-REPOSITORY-SECRETS-) | Exact case and boundary |
| --- | --- |
| 002.5; 001.1/.2 | `keeps accepted Global metadata and its row after older initial GET500`: hold initial GET, submit real form/shared Save, assert POST200 method/body and accepted store/row, dispatch registered created echo, assert no duplicate, release actual GET500, assert exact current store/row and loaded=true/loading=false, one GET, no reveal request. Primary causal RED. |
| 002.3 | `renders uncontested initial GET200 metadata`: independent populated success control. |
| 002.3 | `creates on an already-loaded Global list without initial GET`: real form/Save/POST/event control, existing and accepted rows rendered, zero GETs. |
| 002.6 | `settles uncontested initial GET500 empty`: no mutation, empty/loaded/not-loading, one GET. |
| 002.6 | `clears accepted Global metadata after older initial GET403`: same real create/event path as causal test; post-denial store empty and exact accepted row absent, readiness settled. |

New suite `apps/web/hooks/domains/settings/use-secrets.global-server-error.test.tsx`:

| AC | Exact case and distinct invariant |
| --- | --- |
| 002.5/.4; 001.4 | `preserves complete mixed inventory for two consumers after initial 5xx` (500, 503, 599): actual immutable store, two consumers/one held GET; use returned mutation callbacks for accepted create/edit/delete and registered events for additional create/update/delete/repeated echo. Include untouched Global, legacy, Workspace-filtered rows, multiple removals and changes to name/has_value/updated_at. Capture exact expected current array before failure; both consumers keep filtered current rows, stored entire array identity/order/all fields stays exact, deleted rows absent, no duplicates. Late consumer while pending and settled remount admit no extra GET. |
| 002.5 | `settles an accepted empty inventory after last-row deletion and GET500`: preserve exact empty array identity and settle; prevents a nonempty-list guard. |
| 002.5 | `preserves an accepted metadata replacement after GET500`: existing `setSecrets` reference replacement; readiness and exact array retained. |
| 002.6 | `clears unchanged seeded inventory after uncontested GET500`: nonempty seed but no mutation must clear. |
| 002.6 | `retains 4xx clearing after accepted changes` (400, 401 without challenge, 403, 404, 429): actual Responses after mutation. |
| 002.6 | `retains challenged 401 notification and clearing`: real `setOnUnauthorized` observer; `WWW-Authenticate: Bearer` notifies once before rejected read settles, unchallenged counterpart notifies zero. Restore global callback. |
| 002.6 | `clears accepted metadata for nonqualified read errors`: reject external fetch with network TypeError, AbortError, or plain status-500 lookalike; malformed HTTP200 JSON also clears through the actual API client. No retry. |

Existing Global success suites retain primary ownership of 002.1–.4. Existing Workspace
publication/lifetime tests retain 001.14 and their same-list success contracts. Exact affected
test/type/lint/i18n/spec commands are in the [single work order](task-01-preserve-current-metadata.md#verification).
Run initial causal RED plus four controls before editing production. Mixed cases must assert
all current metadata, not merely that a created row exists. Setup/transport/unhandled errors
invalidate causal evidence. Additional hook cases protect collection and error-policy risks.

## Rendered evidence, mobile parity, and documentation

Real form -> shared Save -> HTTP adapter -> store/registered event -> actual row gives client
end-to-end evidence at the changed boundary. This is the existing data-only mobile-parity
exemption: no layout, touch, scrolling, navigation, breakpoint, copy, or control changes.
The same store outcome serves desktop and phone. No ASCII redesign, Playwright/browser,
or build is required. This specific integration proof is required by ROOT and exercises
causal state loss, rather than testing static component markup.

Internal docs only. Public authentication and agents/profile instructions already describe
scope and accepted saves; no user instruction or terminology changes. The existing
[scope/security ADR](../../decisions/2026-08-03-scope-and-merge-repository-secrets.md) remains
authoritative. This local extension reuses ownership/persistence/authority boundaries;
the amended pair preserves sufficient rationale, so no additional ADR is needed.

## Work orders

- [x] [Task 01: Preserve current metadata on qualified Global server failure](task-01-preserve-current-metadata.md).

Exactly one order; no work-order dependencies; sequential in the SAME primary.

## Verification results and closing discipline

Design checks completed on 2026-10-12:

- `PYTHONDONTWRITEBYTECODE=1 python3 scripts/list-docs.py validate`: exit 0, 377 decisions
  and 1545 specifications. Spec-validator tests: exit 0, all 36 passed. Full specification
  lint: exit 0. `git diff --check`: exit 0; new-file whitespace checked directly too.
- External `check-design.cjs`: exit 0; 13 local links resolve, all ten work-order ACs exist,
  owning design declares both REQs, manifest includes that design, exactly one order.
  Actual docs-only coverage is `exempt`; separately labelled anticipated existing-hook
  production coverage is `covered`, zero errors. Future permanent test/helper files absent.
- Full raw path/status receipts in `/tmp/kandev-child114-design-20261012-ljmvzcik`:
  baseline 25,901 entries and empty status; final 25,903 entries, only two new plan files,
  no removals/drift, exactly the four owned docs changed, no staging. Final requirement
  remains below 20 KiB; repeated motivation was tightened while preserving prior ACs.
- First size gate exited 1 at 20,531 bytes before any repo write; revised prose fixed it.
  Read discovery reported missing `docs/public/agents.md` (batch exit 2; correct public path
  is `agents-and-profiles.md`) and nonexistent `scripts/lib/git-diff.mjs` (read diagnostic
  in a batch ending exit 0). Initial coverage launch exited 127 because shell PATH lacked
  Node; explicit existing Node24 fixed it. Next launch exited 1 because complete raw inventory
  exceeded Node's default 1 MiB buffer; a bounded 16 MiB external checker buffer fixed it,
  without filtering paths. These are design-tool diagnostics, not product test results.

At the historical design checkpoint all commands completed synchronously, with no yielded
native handles or live commands; no implementation, install, lease, hooks or publication had
run. The later reviewed implementation release supersedes that phase status.

### Local implementation results (2026-10-12)

- One frozen install: native25693 actually joined exit0; pnpm9.15.9, 935 reused packages,
  2s, no lockfile change. The SAME primary retained the selected profile and exclusive lease.
- Independent original causal RED: native31686 JOIN1/chunk633ae0, one expected missing-row
  failure/four passing controls/five cases, 11.86s. Real POST200/form/shared Save/registered
  created echo accepted and rendered metadata before releasing held GET500; the row then
  disappeared. Controls proved GET200, loaded create/no GET, GET500 empty, and GET403 clearing.
- Original affected GREEN: native19494 JOIN0/chunk90ae85, twelve suites/110 passing tests,
  39.30s. Scoped lint63764 JOIN1 found function lengths108/100 in the hook and103/100 in
  the mixed test. ROOT approved one file-local pure selector with identical policy, and
  test fixture setup was shortened. No additional production file or behavior changed.
- After that real extraction, only two new failure suites reran: native82796 JOIN0/chunke3cccb,
  21 tests,10.30s. The other ten previously passing suites were not replayed; no reinstall.
- Scoped final four-file ESLint: native88143 JOIN0; zero warnings. First scoped typecheck74772
  JOIN2 found widened test event-action types plus omitted ambient globals/Node typeRoots
  in the external config. Typed the test action generically and restored existing ambient
  inputs, without application changes. Final scoped typecheck19720 JOIN0, config retained
  at `/tmp/kandev-child114-types-fr8ofmes/tsconfig.json`; no emitted/incremental files.
- i18n check14293 JOIN0. Actual staged new-helper final base-pinned ratchet14755 JOIN0:
  one added plus one modified file clean; 643 allowlist entries. The helper was an actual
  staged added candidate, rather than an untracked or unstaged coverage claim.
- Specification checks45243 JOIN0: catalogue377 decisions/1545 specs, 36 validator tests
  passed, full spec lint passed. Actual complete local inventory/coverage exit0: eight owned
  changed paths,111337 total entries, no additions/removals against post-install inventory,
  thirteen local links/ten ACs/two REQs/one order, actual production coverage covered/errors[].
  Dependency installation entries remain recorded and retained; no foreign resource cleanup.
- Whitespace and full status checks passed. All original local handles have joined, no live
  local command remains. Original receipt JSONs/raw inventories are in
  `/tmp/kandev-child114-design-20261012-ljmvzcik`. Protected ROOT fixture/proofs untouched.

Local implementation is complete; order done and manifest implemented before FIRST publish.
Normal active hooks/commit/push remain the immediate authorized delivery steps; their actual
receipts, hosted validation, READY, and separate ROOT expected-head merge grant are recorded
in the external operational plan. No optional post-publication artifact polish is scheduled.

Before FIRST publication, mark the order done and plan implemented only after all exact
checks pass, replace pending results with actual exits/counts/joins and local closing notes,
and record hosted/merge gates as pending in the external operational plan. Include the actual
staged new TS helper in the existing final ratchet (no untracked/unstaged coverage claim).
Do not schedule optional documentation/polish pushes after publication. Hosted receipts and
final merge/archive closure live externally unless an actual required fix changes artifacts.

## Risks and delivery gates

An overbroad catch could retain metadata after authorization denial or hide non-HTTP failures.
A row-only/nonempty guard could drop edits/deletes/untouched inventory or mishandle empty
lists. Stale render snapshots or an await between read/set could violate the shared-store
fence. Real typed HTTP Responses, mixed inventories, exact array assertions, and two consumers
address these risks. If evidence needs another error policy, checkpoint ROOT before expansion.

ROOT full review plus later explicit same-primary implementation interrupt and one exclusive
global local-heavy lease gate execution. After frozen install once, causal RED/GREEN, exact
checks, normal active hooks/commit/push and ALL original local handles joined, report concise
lease RETURN then proceed DIRECT SAME-TURN to hosted validation. Use sole original
`pr-await`90m/cadence60/GNU91m/kill10 observer; retain its handle/clock through interrupts,
without END/WFI, duplicate observers, or extra publication-release ceremony. Published heads
freeze except actual code/data/security/required-CI fixes. Read automatic reviews; no fresh
FULL bot request per addition/fixup/push/SHA. Reuse substantive review plus bounded later diff;
focused rereview only for material correctness/security/architecture need. Honor approvals.

READY requires six required contexts plus actual CURRENT Backend/Frontend/E2E parent SUCCESS,
fresh complete error-free snapshot, zero actionable visible/hidden/change-requested/human
gates, and a clean exact head. ROOT owns one static compatibility check and separate SERIAL
normal expected-head squash grant. No admin bypass/synthetic merged tests/drift-only rebase.
Verify actual merge and known-owned cleanup; ROOT independently verifies/archives once.
Preserve managed workspace/deps/media/cache, foreign refs/worktrees/processes and anonymous
volumes. Every ROOT instruction uses delivery_mode=interrupt; keep SAME primary/live handles.
