---
id: "01-guard-global-publication"
title: "Guard Global initial metadata publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-002
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.1
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.2
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.4
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.14
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.1
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.2
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.3
  - AC-WORKSPACES-REPOSITORY-SECRETS-002.4
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
---

# Task 01: Guard Global initial metadata publication

## Summary and checkpoint

Preserve the complete current Global metadata list when an older initial successful read
completes after accepted changes. Reuse existing store setters; the hook is the only
production edit. Prove the real rendered Settings save path and independent compatibility
controls, then perform only the bounded affected checks under later explicit authorization.

Current status: done for local implementation after ROOT actual-file review and later explicit
implementation release. Exclusive GLOBAL local-heavy grant 111 covers remaining normal hooks
and publication; hosted READY and separate ROOT merge grant are still pending. Task `450a75ba-e760-464e-b185-2b3eb1f407b4`,
SAME primary session `da87b7fb-c6ac-4d31-8a41-df529c2e5237`.
ROOT actual-file review and a LATER explicit implementation INTERRUPT plus exclusive GLOBAL
local-heavy grant are required before production/permanent tests/install/product tests/
lint/typecheck/build/hooks/commit/push. No lease is held during design. Do not switch models,
spawn delegates, recursive tasks, or sessions. Keep existing task/session identity and
system marker; user edits and title ownership remain intact in external task plans.

## Inputs

- [Requirement](../../specs/workspaces/requirements/repository-secrets.md): both frontmatter
  requirements above; all eight listed ACs, with 001 criteria serving compatibility.
- [Design](../../specs/workspaces/system-design/repository-secrets.md): Global initial metadata
  publication; preserve Metadata list lifetimes and its Workspace boundary.
- [Manifest](plan.md): accepted evidence, full test matrix, exact scope and mobile exception.
- Qualified evidence JSON only in `/tmp/kandev-root-global-secrets-discovery-20261010/`.
  The protected `candidate.test.tsx` must never be read/copied/deleted. Independently author
  precise saved-row presence assertions; do not copy the imprecise secondary text matcher.
- Real provider/store and rendered Save patterns in the existing `use-secrets.lifetime.test.tsx`
  and `secrets-settings.lifetime.test.tsx`; use their composition, not their API mocks.

## Files and ownership

Exact production ownership: `apps/web/hooks/domains/settings/use-secrets.ts` only.

Future new permanent test files, created only after implementation release:

- `apps/web/hooks/domains/settings/use-secrets.global-publication.test.tsx`.
- `apps/web/components/settings/secrets-settings.global-publication.test.tsx`.

Delivery artifacts: this order, sibling `plan.md`, and the owning requirement/design.
Existing providers/store/API/registered handlers/Settings widgets are read-only inputs.
The existing `apps/web/hooks/domains/settings/use-secrets.test.ts` provider mock gains
the stable `useAppStoreApi` export now used by the real hook. This is a test-only compatibility
adjustment covered by the existing affected suite; other compatibility tests stay unchanged. Do not change `settings-slice.ts`, its declarations, handler
implementation, or API functions. No new TS helper is planned. If a concrete implementation
need changes the production scope, reconcile this order/design and actual-path coverage
before using that expanded scope; docs-only exemption is not production coverage.

You are not alone in the codebase. Preserve others' edits, worktrees, dependencies, caches,
refs, processes, protected resources, oversized task content, and anonymous volumes.
Inventory all actual tracked/cached/untracked paths and changes before acting; expected
paths may compare them, never hide unexpected paths. Do not clean/reset foreign resources.

## In scope

- All 002 ACs and 001.1/.2/.4/.14 controls in the manifest's test matrix, in one focused pass.
- Actual initial GET held while real Add secret/form/shared Save and real POST accept data;
  registered created-event echo; exact store/DOM row assertions after older GET 200 `[]`.
- Mixed event creation/update/removal with metadata values/order/current membership; accepted
  empty and replacement lists; ordinary loaded create, initial populated/empty GET, shared
  consumers/gates, Global/legacy filtering and existing Workspace independence.
- Hook-only synchronous admission snapshot and success publication; unchanged readiness
  setters and existing catch/finally policy. Targeted real provider/component evidence.

## Out of scope

Values/reveal/auth/API/backend/encryption/bindings/transfer/WS-handler redesign; caches,
lifecycle hardening, timestamps/global versions/frameworks; new state API or Immer imports;
new UI copy/layout/touch/navigation/operator steps; dependency/config/lockfile changes;
broad local product tests/typecheck/build/browser/E2E/harness work. Catch guarding needs its
own independent concrete companion causal RED; without it preserve established failure policy.

## Acceptance

1. Independently authored saved-row regression fails against unchanged production for the
   precise qualified cause, while the ordinary initial-read and loaded-create controls pass;
   then the correction passes all planned tests using real hook/provider/store/API/handlers/
   form/save/row components with only external boundaries isolated.
2. The only production edit is the Global hook path: current-store admission recheck,
   immutable items capture, and synchronous success read/compare/set through existing
   `setSecrets(current === captured ? response ?? [] : current)`. Current metadata array,
   membership, order, and field values survive changed-list success; loaded/loading settle;
   uncontested results and scope/Workspace/failure/filtering compatibility remain intact.
3. Run the exact affected checks below only after release/heavy grant; retain full original
   native results, every yielded session ID and actual joins, inventory and coverage receipts.
   Record honest results in this order and manifest. Hosted delivery and READY/merge have
   the separate gates below; passing local tests is not publication or merge completion.

## Implementation sequence

After actual-file review and a later explicit implementation INTERRUPT, read `/tdd` and
scoped frontend guidance, then mark this order `in_progress`. No generic phase envelope
or design artifact creation authorizes implementation. Acquire ROOT's exclusive GLOBAL
local-heavy grant before install, checks, or normal hooks. Use the current worktree;
no additional worktrees/sessions/native delegates.

Author the two permanent regression suites independently before modifying production.
Do not mock `listSecrets`, `createSecret`, `useSecrets`, `StateProvider`, `SettingsSaveProvider`,
`ToastProvider`, `createAppStore`, registered handlers, or form/row widgets. Stub external
fetch with valid synthetic Responses; inspect real methods/routes/payloads. Dispatch metadata
messages through `registerSecretsHandlers` with real store. Capture its store using
`useAppStoreApi`. Unexpected transport calls, provider setup, invalid matchers, or unhandled
errors invalidate RED; correct only the owned test setup and retain all original receipts.

Use the saved-row test named in the manifest for the primary causal RED. Before releasing
GET, verify POST acknowledgment and accepted metadata/event/DOM; after releasing GET,
assert exact accepted metadata and DOM presence/name. Run the two independent positive
controls in that same unchanged-production run. Mixed mutation and gate controls belong to
the hook suite; capture actual expected current metadata immediately before releasing GET.
Assert removal remains absent and every current row remains in order with its field values.

Implement only the reviewed success guard. Existing `setSecrets(items)` sets loaded true;
feeding it the exact current array settles readiness without metadata replacement. Keep
get/compare/set synchronous with no await/external call. No setter parameter/signature/
state module change is needed. Retain catch/finally and the independent Workspace effect.

## Verification commands

Run serially from repo root in Bash, `login=false`, runtime Node 24.18.0 and pinned pnpm
9.15.9. These were not run during design; the later reviewed release authorized execution.
Conditional frozen install happens once only if worktree dependencies are missing;
no dependency/cache reset or lockfile change. Use one worker and no file parallelism.

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
if [ ! -d apps/node_modules ]; then
  (cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
fi
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-secrets.global-publication.test.tsx components/settings/secrets-settings.global-publication.test.tsx)
```

Run that first command against unchanged production for RED/positive controls; record
counts, causal failures, exit, duration, original handle and actual join before editing
production. After the correction run the following bounded affected GREEN and check block:

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-secrets.global-publication.test.tsx components/settings/secrets-settings.global-publication.test.tsx hooks/domains/settings/use-secrets.lifetime.test.tsx components/settings/secrets-settings.lifetime.test.tsx hooks/domains/settings/use-secrets.test.ts components/settings/secrets-settings.test.ts)
(cd apps/web && corepack pnpm@9.15.9 exec eslint --max-warnings=0 hooks/domains/settings/use-secrets.ts hooks/domains/settings/use-secrets.global-publication.test.tsx components/settings/secrets-settings.global-publication.test.tsx hooks/domains/settings/use-secrets.test.ts)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet --base e307f4382b173b00bcac0c0ccf402b9cdc98caaa)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short --untracked-files=all
```

No new helper is planned; stage any later reviewed new TS helper before the existing final
i18n ratchet so it is scanned. Keep normal commit hooks, with no bypass. Do not add generic
full tests/typecheck/build/browser/QA/verify/review passes. New changes/failures may justify
only concrete affected checks. Every long command keeps its FULL original exec result,
every yielded `session_id`, and actual joins through the original handle. Never infer
completion from a duplicate run or abandon a handle. Record current absence of known-owned
wrapper/process groups before cleanup/lease RETURN. Unknown failures require exact evidence;
no blind retry, resource deletion, or foreign process termination.

## Design and actual-path checks

The design-only phase authorized just these checks: catalog validation, specification lint,
Markdown links/AC/design references, whitespace, and unfiltered actual path/inventory coverage.
Use `git ls-files -z --cached --others` and full `git status --porcelain=v1 -z
--untracked-files=all`, saving raw bytes externally. Compare complete records to the baseline;
show every unexpected path or foreign edit. Do not whitelist before inventory or validation.

`validateCoverage` from `.github/scripts/pr-docs.cjs` receives actual document contents and
all actual changed paths. Record docs-only `exempt` separately. In a distinct production
preflight add the real existing `apps/web/hooks/domains/settings/use-secrets.ts` path;
assert `covered`, one changed order, accepted owning design/requirements, all eight defined
ACs, and no errors. Verify this order names every covered production path and that each
exists. Planned tests are absent during design and are not counted as implemented coverage.
At delivery, repeat with the entire actual changed/untracked inventory; every production
path must be owned and listed in coverage, with any drift reconciled explicitly.

## Mobile parity and documentation

Pure state/data correction in existing Settings. No layout, touch, scroll, navigation,
viewport-dependent interaction, copy, or operator workflow changes. The real rendered
component save/row check exercises the shared behavior used on desktop and phone; the
mobile-parity exception permits it without a new mobile Playwright case or ASCII redesign.
Public scope/profile guidance already matches the outcome; only these internal artifacts
change. Preserve the prior Workspace lifetime design and scope/security ADR.

## Dependencies and parallelism

None. `sequential`, SAME primary only. No delegates/recursive tasks/new sessions/model switches.

## Risks

Render-time capture, draft-proxy comparison, metadata-content equality, or a stale setter
argument can miss accepted changes. Use actual current owning store at admission and success;
keep read/compare/set synchronous. Whole-list fencing must not insert snapshot-only rows.
A changed-list success must still settle loaded/loading without another GET. Failure-path
scope may not be inferred from the supplied success RED. Protected source is never an input.

## Later authorized delivery gates

These preserve ROOT’s standing delivery gates. Actual-file review and the later implementation
release/grant 111 have occurred; the separate merge release has not.
After later review/release/heavy grant and meaningful affected checks, normal commit, push,
and a ready PR are authorized. Load relevant commit/push/PR skills then, keep conventional
commits and normal hooks without bypass. Preserve all original native handles/joins. After
local-heavy work actually joins, verify current owned cleanup/absence and RETURN the grant.
Then proceed DIRECTLY in the SAME TURN to hosted observation: one original observer,
90-minute ordinary deadline, 60-second cadence, GNU 91-minute TERM with kill-after 10 seconds.
Retain its original clock/receipt/handle across interrupts, main/head changes, and fixups;
do not create duplicate observers or request an extra ROOT publication release or END/WFI.

Read automatic feedback while CI proceeds. Do not initiate a fresh manual FULL BOT review
for additions, fixups, pushes, or new SHAs. Reuse adequate substantive review plus later diff;
focused review is justified only by concrete material correctness/security/architecture need.
ACKs and infra failures are not substantive reviews. Optional style/docstrings/metrics/polish
must not cause a push or become READY gates. Job-name retry budgets persist across pushes;
unknown failures require exact evidence and no blind retry.

READY requires known SIX required checks plus actual Backend/Frontend/E2E parents at CURRENT
success, fresh complete `errors: []`, zero actionable visible/hidden/human review threads,
clean exact head, substantive review plus later diff, and the original observer actually
joined with current known-owned absence. A healthy-pending renewal may happen once only after
the original ordinary deadline/actual join/current owned absence; preserve truthful clock
and receipt. A callback or queue_full never becomes a gate: ROOT stays ACTIVE in direct
supervision and can read actual files/checkpoint. Every ROOT-to-child instruction uses
`delivery_mode="interrupt"`; preserve SAME session and handles, no duplicates.

No merge until ROOT separately performs one static compatibility check and grants serial
expected-head normal squash. No admin/bypass/branch deletion. If a branch is queued and an
authorized fixup is needed, follow existing dequeue/exact push/check/requeue handling;
merge grant remains separate. Do not touch protected oversized tasks, foreign processes,
or anonymous volumes during cleanup. Update this order and manifest from actual results.

## Results

Local implementation complete on 2026-10-10 in this SAME primary under grant 111.
Only `use-secrets.ts` changed in production; the existing mock-provider export adjustment
and two independently authored new suites are test-only. Whole-current-list metadata,
readiness, real rendered save/event/row usability and the scoped compatibility controls pass.

Original RED: native 80016 joined exit 1, seven causal failures/three passes/ten cases,
12.42s. Original affected GREEN: native 76344 joined exit 1, 56 passed/one legacy JSON-fixture
failure/57 cases, 27.50s. Corrected only the legacy omitted-scope fixture; affected new hook
suite native 8303 joined exit 0, seven passed, 3.20s. The three real Settings cases and
47 existing compatibility cases passed in the first GREEN and were not replayed.

One frozen install (11327) completed; scoped four-file ESLint (16056), i18n check (87606),
and staged final base-pinned ratchet (46552) all joined exit 0. Actual complete inventory
coverage (3674) joined exit 0: eight owned changed paths, one covered production hook,
one order, correct owning design/requirements, zero coverage errors. Ignored install/tooling
entries remain in full raw inventory receipts; no unexpected Git edit/removal/cleanup.
No broad tests/typecheck/build/browser or protected-source access occurred.

Catalogue/specification/whitespace and normal hooks/publication receipts are kept in the
external task checkpoint and log directory. Normal hooks run without bypass; this status
records the local implementation result and does not assert hosted READY or merge.
