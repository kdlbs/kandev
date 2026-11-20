---
id: "01-remove-exact-label-name"
title: "Remove an exact label name compatibly"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-LABELS-002
acceptance_criteria:
  - AC-TASKS-LABELS-002.1
  - AC-TASKS-LABELS-002.2
  - AC-TASKS-LABELS-002.3
  - AC-TASKS-LABELS-002.4
  - AC-TASKS-LABELS-002.5
  - AC-TASKS-LABELS-002.6
system_design:
  - ../../specs/tasks/system-design/labels.md
---

# Task 01: Remove an exact label name compatibly

## Summary

Deliver the query-based DELETE route and matching production API client while
retaining existing path callers. Prove the selected association is durably
removed for arbitrary accepted names and the Office task action works on both
desktop and phone.

## In scope

- Apply TDD and mark this work order in progress when implementation begins.
- Independently author registered-route tests with a real `LabelService` and
  settings/task/Office schema constructors sharing an FK-enabled disposable
  database.
- RED: request the intended collection DELETE for `area/backend`, expecting
  200 and exact persisted absence; before the fix it must fail because the
  route does not exist. Pair with old escaped-path characterization (404
  remains the legacy behavior for slash names) and passing simple-name
  deletion controls. Test the production client RED for the expected query
  request rather than only testing a stand-alone encoder.
- Attach `area/backend`, `a/b/c`, literal `area%2Fbackend`, `a+b`, `a b`,
  `100%`, `needs review`, and stored edge-space names with exact collision
  neighbors. Include `urgent` legacy control, repeat removal, absent catalog
  name and existing catalog entry absent from the selected task. Inspect
  actual catalog/association IDs and shared associations after each removal.
- Add missing/empty/repeated/malformed `%` query checks (400 and no writes).
  Reuse existing error envelopes; no normalization or second decoding.
- Add the route under existing scope middleware. Extend mixed-param test cases
  and verify owner/foreign, mixed workspace (including two workspaces owned
  by one caller), missing resource and agent JWT behavior; do not add exemptions.
- GREEN: use `URLSearchParams` in the existing production `removeLabel` and
  share existing service deletion with the new handler. Keep old DELETE,
  GET/POST and catalog routes. No service or schema change is expected.
- Component tests use the real optimistic hook with a stateful provider,
  rejecting only transport for failure checks. Verify exact-name chips,
  mouse click, touch-pointer and Enter/Space activation, and rollback/toast
  rather than mocking the mutation hook. Use the actual production API client
  with controlled fetch responses so successful interaction also verifies the
  query request, and failed transport exercises the real rollback path.
- Apply the manifest's narrow mobile state/data exception consistently. Pair
  component actions with real-route persisted-state rereads; document that
  neither phone geometry nor browser/reload E2E is established. Keep
  `office-api-client.ts`'s path removal helper as the compatibility caller for
  existing tests; production UI must use the new query route.
- Update `docs/public/office-tasks.md` with the short how-to and optional API
  reference described in the design. Preserve experimental Office status and
  all existing label/color assertions. Run its two public-doc validators.
- Record actual results and commands; synchronize this work order and manifest.
  Promote the paired draft design to current only after all outcomes pass.

## Out of scope

Color persistence, catalog feature completion, schema/JSON migrations, global
Gin switches, wildcard routing, new CLI/MCP contracts, UI layout/touch-size
redesign, legacy phone layout audits, new E2E/browser builds, unrelated error
handling, and unrelated file-link behavior.

## Acceptance

1. Every identity/validation/compatibility case in the manifest passes through
   the registered HTTP route and real persistence, preserving exact labels,
   other associations, authorization, response shape and idempotency.
2. The actual production client sends the exact query name and preserves
   request options/errors; the real picker/optimistic hook covers success,
   failure, mouse click, touch-pointer entry and keyboard. Real-route durable
   rereads plus shared component actions establish the bounded desktop/phone
   evidence; do not claim browser reload or rendered phone coverage.
3. The scoped commands below pass, public docs describe the final contract,
   and this work order plus manifest contain actual evidence and final status.

## Verification

Run commands sequentially from repository root; the subshells below root each
command. In a fresh checkout, install dependencies once before package commands:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

RED/GREEN backend regression command (repeat only for actual changed code or
failure remediation, recording expected RED and final GREEN separately):

```bash
(cd apps/backend && timeout --kill-after=10s 180s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags fts5 -p 1 ./internal/office/labels -run '^TestTaskLabelRemoval' -count=1 -timeout=120s)
(cd apps/backend && timeout --kill-after=10s 300s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags fts5 -p 1 ./internal/backendapp -run '^TestOffice(TaskLabelRemovalScope|RouteScopeCompleteness|ScopeCasesMatchRegisteredRoutes|ScopeTablesHaveNoDeadEntries|MultiIDRoutesRejectMixedWorkspaces|Scope(DeniesForeignSiblingIDOnWorkspaceRoute|AllowsOwnerOnWorkspaceRoute|ConfinesAgentJWTToItsOwnWorkspace|AllowsAgentJWTInsideItsOwnWorkspace|DeniesIDsFromDifferentWorkspaces))$' -count=1 -timeout=240s)
```

Frontend checks (use repository default worker limits):

```bash
(cd apps/web && timeout --kill-after=10s 180s pnpm exec vitest run lib/api/domains/office-label-removal-api.test.ts components/task/simple/components/labels-picker.test.tsx)
(cd apps/web && timeout --kill-after=10s 360s env NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && timeout --kill-after=10s 180s pnpm exec eslint --max-warnings 0 lib/api/domains/office-extended-api.ts lib/api/domains/office-label-removal-api.test.ts components/task/simple/components/labels-picker.test.tsx)
```

No managed E2E or browser builds are planned. The existing Office labels E2E is
a baseline only and does not establish slash or browser/reload coverage.

Changed-Go lint uses its separate default 3 GiB resource budget, not the Go test
memory cap. This is the expected file set; include any additional actually
changed Go files instead of dropping them:

```bash
timeout --kill-after=10s 300s bash scripts/lint-go-changed apps/backend/internal/office/labels/handler.go apps/backend/internal/office/labels/handler_test.go apps/backend/internal/backendapp/office_scope_test.go apps/backend/internal/backendapp/office_label_scope_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Run the offline `validateCoverage` export in `.github/scripts/pr-docs.cjs` with
the actual changed paths and linked requirement/design/manifest/work-order
contents. Assert `ok` and `covered` for the implementation diff, not merely a
docs-only exemption; record exact invocation and result. During design, direct
`parseFrontmatter`/reference inspection may check the package without inventing
product changes.

## Files likely touched

- `apps/backend/internal/office/labels/handler.go`
- `apps/backend/internal/office/labels/handler_test.go` (new)
- `apps/backend/internal/backendapp/office_scope_test.go` (case tables only;
  put new test functions in a separate file if needed for file-size limits)
- `apps/backend/internal/backendapp/office_label_scope_test.go` (new)
- `apps/backend/internal/workflowsync/service_test.go` (required-CI test
  synchronization correction only; see results).
- `apps/web/lib/api/domains/office-extended-api.ts`
- `apps/web/lib/api/domains/office-label-removal-api.test.ts` (new)
- `apps/web/components/task/simple/components/labels-picker.test.tsx` (new)
- `docs/public/office-tasks.md`
- The four design-package documents (statuses/results only as appropriate).

## Dependencies

None.

## Risks

Query double decoding, routing the new endpoint outside the scope guard, false
positive optimistic UI evidence, overstating component evidence as browser
coverage, and
accidentally changing catalog deletion rather than task association removal.
Preserve the explicit mobile state/data exception rather than expanding into
an unrelated phone layout feature.

## Parallelism

`sequential`

## Inputs

- [Labels requirements](../../specs/tasks/requirements/labels.md),
  REQ-TASKS-LABELS-002 and all six acceptance criteria.
- [Labels system design](../../specs/tasks/system-design/labels.md).
- [Manifest](plan.md), including the exact identity/control matrix.
- `apps/backend/AGENTS.md`, `apps/backend/internal/office/AGENTS.md`,
  `apps/web/AGENTS.md`; `/tdd`, `/mobile-parity`, `/docs-maintainer`.
- Existing `office_scope_test.go`/`office_scope_completeness_test.go`,
  `office-pause-api.test.ts` and `priority-picker.test.tsx`.

## Results

Completed on 2026-10-26. All verification commands above ran from their stated
working directories. The production change is one additional DELETE route,
single-value query validation in the shared removal handler, and query
serialization in the existing client. Existing router configuration, service,
schema and picker markup remain unchanged.

- Dependency bootstrap: one `pnpm install --frozen-lockfile` passed.
- RED: before production edits, collection DELETE returned 404 instead of 200
  for valid names and 404 instead of 400 for invalid query cases. The existing
  path-route controls passed. An initial fixture issue with percent-containing
  subtest directory names was corrected in test setup; the clean RED then
  failed only on the missing-route assertions.
- Frontend RED: 11 expected query-vs-path assertions failed; error propagation
  and real optimistic rollback controls passed.
- Backend GREEN: `TestTaskLabelRemoval*` passed under the bounded race command
  (5.314 seconds test time). Actual registered routes, real service and real
  FK-enabled settings/task/Office schemas cover nine identity cases, six invalid
  queries, legacy compatibility, idempotency, catalog equality, shared
  associations and another workspace's exact label identity.
- Scope GREEN: the stated backendapp command passed (4.749 seconds test time),
  including eight focused browser/agent cases plus existing scope, route
  completeness and mixed-workspace sweeps.
- Frontend GREEN: both suites passed, 13 tests total. After test-only type
  corrections the 13 tests passed again; after extracting the lint-reported
  repeated accessible-name literal, the affected five component tests passed.
  The final API suite has eight passing cases and the component suite has five.
- Canonical `pnpm run typecheck` passed at the stated 6144 MiB/360-second bound.
  The preceding attempt found only owned fixture child-type and Testing Library
  option errors, which were corrected without a production change.
- Frontend ESLint with `--max-warnings 0` passed; canonical changed-Go lint
  reported `0 issues` using its separate default 3 GiB budget.
- `pnpm run i18n:ratchet` passed after staging added tests: no new-copy violations
  and guard allowlist intact.
- Catalog validation passed (403 decisions and 1743 specifications),
  specification lint passed, all 63 public-doc validator tests passed, and
  54 published pages validated. Public docs updated:
  `docs/public/office-tasks.md` (how-to with a compact optional API reference).
- Offline `.github/scripts/pr-docs.cjs` `validateCoverage` passed with `covered`
  and no errors for the actual implementation paths and linked package.
  Frontmatter/reference/local-link checks and diff whitespace checks passed.

No managed E2E, browser build, browser reload, or rendered phone-geometry
verification was performed. Shared click/touch-pointer/keyboard component
behavior plus production-client serialization and real persisted-state rereads
provide the explicitly bounded state/data evidence for AC-TASKS-LABELS-002.6.

### Required-CI test synchronization correction

The PostgreSQL package gate exposed an existing asynchronous test race in
`TestPermanentFailureSuspendsPollingUntilManualRecovery`: persisted suspension
became observable before the original automatic job emitted its transition
metric. The test now requires that job's `automaticInFlight` entry to clear
under `automaticMu` before checking metrics or advancing the fake clock.
All suspension, polling, recovery and metric assertions remain intact; no
production behavior, sleeps or timeout increases were added.

The unchanged targeted test passed 20 race-enabled repetitions locally, so
local reproduction was unsuccessful; the hosted failure supplied the actual
zero-versus-one metric result. The corrected targeted test passed 20
repetitions, followed sequentially by one passing workflowsync package run.
Both used `GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB`, GNU timeout
300 seconds with a 10-second kill grace, and
`go test -trimpath -race -tags fts5 -p 1 ./internal/workflowsync -timeout=270s`.
The targeted runs added
`-run '^TestPermanentFailureSuspendsPollingUntilManualRecovery$' -count=20`;
the package run used `-count=1`. Test times were 1.166 seconds unchanged,
1.161 seconds corrected, and 1.305 seconds for the package.
Canonical `bash scripts/lint-go-changed
apps/backend/internal/workflowsync/service_test.go` passed with `0 issues`
using its separate default 3 GiB memory budget.
