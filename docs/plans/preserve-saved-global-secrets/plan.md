---
created: 2026-10-10
status: completed
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-002
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
legacy_specs: []
---

# Implementation Plan: Preserve saved Global secrets

## Overview and checkpoint

Preserve accepted Global metadata when an older initial list GET succeeds. One sequential
work order adds independent causal tests, changes only the Global success-publication path
in `useSecrets`, and checks affected behavior. Workspaces owns this vertical contract because
its repository-secrets pair already owns metadata scope, Global profile use, and list lifetime.

Current phase: local implementation complete after ROOT full actual-file review and a
later explicit implementation INTERRUPT under exclusive GLOBAL local-heavy grant 111.
The SAME primary owns implementation and delivery; no delegates, recursive tasks, sessions,
or model switches. Hosted CI/review, READY, and separately granted merge remain pending.
DESIGN_READY was the prior design checkpoint; its results below are historical.
Task: `450a75ba-e760-464e-b185-2b3eb1f407b4`.
Session: `da87b7fb-c6ac-4d31-8a41-df529c2e5237`.

## Evidence and settled scope

Qualified ROOT evidence is in `/tmp/kandev-root-global-secrets-discovery-20261010/qualified-proof.json`
and `qualifier.json`, at base `e307f4382b173b00bcac0c0ccf402b9cdc98caaa`.
The Global hook starts the real initial `GET /api/v1/secrets` with loaded/loading false.
Real Settings Add secret/form/shared Save, real `createSecret` POST adapter with an external
synthetic HTTP 200 response, and the registered `secrets.created` handler accept and render
saved metadata while GET is pending. A later GET 200 `[]` reaches unconditional
`.then(setSecrets(response))`, clearing the actual current store and saved rendered row.
One causal RED and two controls passed qualification: ordinary initial GET renders rows;
ordinary already-loaded real creation saves/renders without a GET. There was no transport,
setup, or unhandled failure. The secondary row-text assertion received undefined after the
row disappeared; author a precise independent DOM-presence assertion rather than copying it.

ROOT's native handle 7716 actually joined exit 1 with receipt `04727c`; wrapper 3726319
and group 3726361 were absent. The qualifier records checksum-proved scratch removal,
clean ROOT, and GLOBALheavy RETURN before this task's admission. These are historical ROOT
receipts, not new local execution evidence. Never read, copy, delete, or otherwise touch
the protected mode-0400 `candidate.test.tsx`, SHA256
`f4c608e23c8120a6e2bf85d1c14c0688d635a1f25b5d8d9de75cbdc0642fcc6e`.

Assumption check: outcome, owning scope, success-only evidence, and exclusions are settled by
the request and current source. No material question blocks this package. Existing
[Workspace isolation delivery](../workspace-secret-list-isolation/plan.md) remains a completed
independent-lifetime correction; its earlier exclusion of same-list ordering does not define
this new bounded Global contract. Do not reopen or remigrate that work.

## Scope

### In scope

- REQ-WORKSPACES-REPOSITORY-SECRETS-002, all four ACs: accepted creation/updates/removal,
  metadata events, exact current membership/order/field values, and rendered saved-row usability.
- Compatibility for AC-WORKSPACES-REPOSITORY-SECRETS-001.1/.2/.4/.14: ordinary initial
  success/empty response, shared loaded/loading gates and multiple consumers, Global/legacy
  filtering, ordinary loaded-list creation, and independent Workspace lifetime.
- Real hook/provider/store/API/registered metadata events and actual Settings form/save/row
  evidence; only external HTTP and event transport boundaries are isolated.

### Out of scope

Secret values/reveal, authorization, API/schema/backend/encryption, bindings/transfer,
WebSocket-handler redesign, cross-workspace caching, lifecycle/navigation hardening,
global version/timestamp/journal/framework, dependencies, new copy/interaction/operator steps,
and broad local tests/builds/browser/E2E/typecheck. Preserve other edits, worktrees,
dependencies, caches, refs, processes, protected resources, oversized tasks, and anonymous volumes.

Current catch behavior stays unchanged. A failure-path extension is allowed only after an
independently authored concrete companion RED proves loss of current accepted metadata;
no such evidence is claimed or scheduled here. Otherwise preserve loaded/loading/error policy.

## Technical approach and actual paths

Only production edit: `apps/web/hooks/domains/settings/use-secrets.ts`.
Use existing `useAppStoreApi`, synchronous owning-store admission recheck, and a captured
immutable items reference. On success read current items and immediately call existing
`setSecrets(currentItems === capturedItems ? response ?? [] : currentItems)` with no await
or external call between read and write. This reuses the existing loaded settlement.
`finally` clears loading. Preserve scope/gates/Workspace effect and `filterGlobalSecrets`.

No state action/signature/declaration change is needed. The owning pair's
[Global initial metadata publication](../../specs/workspaces/system-design/repository-secrets.md#global-initial-metadata-publication)
section defines the mechanism. A changed list makes its complete current snapshot authoritative;
do not merge in snapshot-only rows or resurrect removed rows. Metadata array identity under
actual Immer provides the guard; no timestamps or equality-by-content inference is allowed.

New regression test paths:

- `apps/web/hooks/domains/settings/use-secrets.global-publication.test.tsx`.
- `apps/web/components/settings/secrets-settings.global-publication.test.tsx`.

Test-only compatibility adjustment: `apps/web/hooks/domains/settings/use-secrets.test.ts`
adds the stable existing `useAppStoreApi` export to its provider mock. Its owning suite
already appears in the affected checks. No other mock uses the real changed hook.

Read-only integration paths: `components/state-provider.tsx`, `components/toast-provider.tsx`,
`components/settings/settings-save-provider.tsx`, `components/settings/secrets-settings.tsx`,
`lib/state/store.ts`, `lib/state/slices/settings/settings-slice.ts`,
`lib/api/domains/secrets-api.ts`, `lib/api/client.ts`, and `lib/ws/handlers/secrets.ts`
(all under `apps/web/`). These paths exist; no production edits outside the hook are covered.

## Tests and AC coverage

The tests below were independently authored and executed against the real affected boundaries.
Use real `StateProvider`/`createAppStore`/`useSecrets`, real APIs and registered handlers.
External fetch stubs must return valid HTTP Responses and reject unexpected routes/methods.
Do not mock providers, store, hook, API functions, form, Save, row widgets, or handlers.

| AC | Suite and named case | Required causal evidence |
| --- | --- | --- |
| 002.1; 001.1/.2 | `secrets-settings.global-publication.test.tsx`: `keeps an accepted saved global row after an older initial GET succeeds empty` | Hold the actual initial GET; fill the real Add secret form with synthetic data; use actual shared Save; verify real POST method/body and HTTP 200 acknowledgment; dispatch registered created metadata event; assert exact store metadata and `secret-row-<id>` DOM presence/name while GET remains pending; release GET 200 `[]`; assert the same store and exact row presence again. |
| 002.2 | `use-secrets.global-publication.test.tsx`: `preserves the whole current list after mixed accepted metadata changes` | Pending real GET with loaded false and existing metadata; apply registered create/update/delete events and repeated create; capture current expected membership/order/fields; release older nonempty conflicting snapshot; every current row survives unchanged, deleted and snapshot-only rows stay absent, no duplicates. |
| 002.2 | Same: `keeps an accepted empty list after removal during initial GET` | Remove the last row via registered deletion event while GET is held, then release a snapshot containing that row; accepted empty list stays empty. |
| 002.2 | Same: `preserves a newer authoritative metadata replacement` | Existing unconditional `setSecrets` replaces the list during held GET; an older success cannot replace it. This is a current-store control, not lifecycle hardening. |
| 002.3 | Rendered suite: `renders ordinary initial GET rows` and `saves on an already-loaded global list without an initial GET` | Two independent positive controls: uncontested HTTP 200 rows render; real form/POST/shared Save on loaded data renders accepted metadata, including created-event echo, with zero list GETs. |
| 002.3/.4; 001.4 | Hook suite: `settles uncontested populated and empty reads with shared gates and filtering` | Populated/empty success; simultaneous real consumers admit one GET, late consumer while loading shares it, loaded remount admits none. Filter mixed Workspace/Global/legacy rows for all consumers without filtering the stored array. |
| 002.4 | Hook suite: `settles superseded success without losing current metadata or refetching` | Preserve exact current array identity; loaded true/loading false for all consumers after success; no trailing GET. |
| 002.4; 001.14 | Existing `use-secrets.lifetime.test.tsx`, `secrets-settings.lifetime.test.tsx`, and `use-secrets.test.ts` | Run unchanged independent Workspace success/failure/lifetime and existing Global filtering controls. |
| 002.4 | Hook suite: `retains ordinary initial-read failure settlement` | External failed initial GET without an intervening mutation leaves established empty/loaded/not-loading behavior; no failure-path redesign. |

Separate affected RED/GREEN evidence from positive controls. The saved-row regression must
fail before the production edit because accepted metadata becomes `[]` and the actual row
is absent. Assert row existence with `queryByTestId`/`getByTestId` explicitly, then use
`within(row).getByText(name)` on a proven element. Do not pass undefined to a text matcher.
Use controlled deferred responses and `act`/causal waits; no arbitrary sleeps. Settle every
owned deferred response and restore external stubs in cleanup. Setup/transport/unhandled
failures are not RED or PASS and must be corrected without expanding production scope.

## Rendered evidence and mobile parity

The rendered test traverses real Settings form -> shared Save -> HTTP adapter -> accepted
store/event metadata -> actual row. This supplies client end-to-end evidence at the affected
boundary. The state/data-only mobile exception applies: no layout, touch behavior, scrolling,
navigation, viewport-dependent interaction, new copy, or operator step changes. The shared
state fix serves phone and desktop equally; targeted component tests are sufficient.
No ASCII redesign or new Playwright/browser/build command belongs to this package.

## Documentation impact

Internal docs only. `/docs-maintainer` assessment found existing public authentication and
agents/profile documentation already describes saved Global metadata and scope/profile use.
The correction restores this behavior without new user instructions or terminology.
The existing scope-and-merge secrets ADR retains ownership/security authority; a local
publication guard changes no architectural owner, persistence boundary, or public contract
that requires a separate ADR. No root/scoped AGENTS convention changes are needed.

## Work orders

- [x] [Task 01: Guard Global initial metadata publication](task-01-guard-global-publication.md).

Exactly one bounded sequential work order, no dependencies. All ACs and design references
are in that order. ROOT actual-file review and later release/heavy grants are execution
gates, not implicit permission from a plan or phase message.

## Inventory and documentation coverage

Capture unfiltered tracked/cached/untracked NUL paths and full NUL status to an external
receipt. Expected paths are comparison inputs only; never filter out unexpected/foreign
paths. Initial inventory `/tmp/kandev-global-secrets-design-20261010/inventory-start.nul`
contains 25,038 entries, SHA256
`fc4de77f04c82094f5df8b07fa9852f41bb8a224e6316778f7129e93dea3db16`;
initial full status was empty at the qualified baseline. Compare all later path/status changes.

Run `validateCoverage` against all actual changed docs and report its docs-only exemption
honestly. Separately include the existing actual hook path with the four actual documents
for production reference coverage; verify the order names that path, every AC exists in
its owning requirement, the design declares both requirements, and its path is in the
manifest. Planned new test paths remain future artifacts; do not create them during design.
Before delivery validate all actual changed/untracked paths again, reporting any scope drift.

## Verification results

### Historical design checkpoint

Lightweight design checks completed on 2026-10-10:

- `python3 scripts/list-docs.py validate`: exit 0; 369 decisions and 1513 specifications.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all specifications passed.
- Actual-path/reference/link preflight in
  `/tmp/kandev-global-secrets-design-20261010/check-design.cjs`: passed. Actual docs-only
  coverage is `exempt`; distinct hook production-path coverage is `covered`, one order,
  both requirements, all eight ACs, valid manifest/design links, and zero coverage errors.
  Future permanent test paths are absent. All eight local Markdown links resolve.
- Full raw NUL inventory/status receipts and `design-coverage.json` preserve all paths:
  25,041 entries, no removals, exactly four changed Git paths (the four documents).
  Additions are the two new plan documents plus ignored
  `scripts/__pycache__/spec_metadata.cpython-312.pyc`, observed during Python document tooling.
  The ignored cache is explicitly reported and retained; no cleanup or foreign-resource
  mutation is authorized. Initial preflight exit 1 reported this unexpected inventory
  entry; `design-coverage-first.json` retains that evidence. It was reconciled explicitly,
  not filtered from inventory, and there is no unexplained drift or foreign Git edit.
- `git diff --check`: exit 0. Both new documents also have no whitespace diagnostics
  under `git diff --no-index --check /dev/null <path>`; exit 1 there denotes added content,
  not a whitespace diagnostic. Final lightweight receipt checks these return semantics.

At that design checkpoint the four files were unstaged/uncommitted, the order was pending,
and no implementation/product check/install/hook/commit/push had run or heavy lease been held.
The later implementation release supersedes that phase status.

### Local implementation results (2026-10-10)

- One missing-dependency frozen apps install completed with pnpm 9.15.9/Node 24.18.0;
  no lockfile change. Original native handle 11327 actually joined exit 0.
- Independent two-suite RED against unchanged production: seven causal failures and three
  passes across ten cases, exit 1, 12.42s; original handle 80016 actually joined.
  The real saved metadata became `[]` after older GET 200 `[]`; ordinary initial read and
  loaded-list real create controls passed. Additional failures proved mixed metadata rollback,
  deletion resurrection, replacement loss, and simultaneous duplicate admission.
- The only production change captures/rechecks the current owning store at admission and
  selects response/current items synchronously through existing `setSecrets`. It settles
  readiness while retaining the current list and preserves Workspace/catch/filter behavior.
  The existing mocked hook suite gains a stable `useAppStoreApi` export, with no store API change.
- First affected six-suite GREEN: 56 passed and one failed across 57 cases, exit 1, 27.50s;
  original handle 76344 actually joined. The sole failure was the new legacy fixture expecting
  explicit `scope: undefined` after real HTTP JSON omitted it. Corrected the fixture to the
  actual omitted wire field without another production edit. Only the affected hook suite
  reran: seven passed, exit 0, 3.20s; original handle 8303 actually joined. Real Settings suite
  (three cases) and existing compatibility suites (47 cases) passed in the first run and
  were not needlessly replayed. All 57 affected cases are therefore verified across these runs.
- Scoped four-file ESLint: exit 0 (original handle 16056 joined). i18n check: exit 0
  (87606 joined; catalog orphan warnings reported). Final base-pinned i18n ratchet: exit 0
  (46552 joined) after staging both new test files; no new copy. Normal formatting applied.
- Actual complete inventory/reference coverage: exit 0 (3674 actually joined). Eight changed
  owned paths, sole hook production path covered, one order, both requirements, zero errors,
  no unexpected Git edit or removal. Raw NUL receipts retain 110,474 entries, including all
  85,432 ignored dependency/tooling additions from the authorized install/tooling. No ignored
  paths were hidden from inventory or deleted. Coverage is production `covered`, not the
  historical docs-only exemption.

Full logs/process receipts, FULL native result frames and complete path records are under
`/tmp/kandev-global-secrets-design-20261010/`. No broad local suite/typecheck/build/browser
ran. Final catalogue/spec/whitespace checks and normal hooks/publication are recorded by the
work order/external task checkpoint; hosted state and exact-head receipts remain external.

## Risks

- Capturing a render-time array or comparing Immer draft proxies would miss the actual
  admission snapshot; use current store reads and immutable published references.
- Updating readiness alone through a new action would enlarge the contract unnecessarily;
  the unchanged setter can retain the exact current array and settle loaded synchronously.
- Whole-list fencing deliberately omits snapshot-only rows after intervening changes.
  A merge could resurrect deletions or overwrite current fields/order.
- Do not infer failure-path authority from successful-read RED, or let failed test setup
  masquerade as causal evidence. The near-limit existing requirement stays within 20 KiB;
  no specification migration or size exception is authorized here.

## Delivery handoff

Task 01 contains later exact affected checks and ROOT's standing delivery constraints.
The prior DESIGN_READY handoff completed before ROOT review/release. Local implementation
is complete; normal authorized publication follows the order’s recorded gates. Hosted READY
and actual merge remain separate outcomes and are not inferred from local success.
