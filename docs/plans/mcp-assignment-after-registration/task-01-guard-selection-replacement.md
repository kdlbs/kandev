---
id: "01-guard-selection-replacement"
title: "Guard MCP selection replacement against stale owner snapshots"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-ASSIGNMENT-002
  - REQ-AGENTS-MCP-ASSIGNMENT-003
acceptance_criteria:
  - AC-AGENTS-MCP-ASSIGNMENT-002.3
  - AC-AGENTS-MCP-ASSIGNMENT-002.5
  - AC-AGENTS-MCP-ASSIGNMENT-003.1
  - AC-AGENTS-MCP-ASSIGNMENT-003.2
  - AC-AGENTS-MCP-ASSIGNMENT-003.3
  - AC-AGENTS-MCP-ASSIGNMENT-003.4
system_design:
  - ../../specs/agents/system-design/workspace-mcp-assignment.md
---

# Task 01: Guard selection replacement

## Summary

Add atomic direct-membership preconditions for non-session selection writes
and the shared revision precondition for sessions. Wire the common web editor
to its accepted baseline/revision. Preserve exact committed
responses, existing save ownership, and session runtime application authority.

## In scope

- Non-session `expected_definition_ids`; explicit empty vs omitted vs invalid
  null. Session `expected_revision` against the returned `selection_revision`.
- Transactional comparison and lock coordination across existing writers,
  catalog deletion, legacy profile import, and session desired-state changes.
- Mutation-time eligibility for new additions; preserve cleanup/removal.
- Conflict feedback, explicit reload/review, lost-ACK reconciliation and stale
  context/draft protection in existing owner editors.

## Out of scope

Registration UI, owner search/navigation, schema migration, runtime reconnect
changes, inherited exclusions, and management sibling destinations.

## Acceptance

1. Independent database writers cannot lose a concurrent selection through a
   conditional write. Conflicts perform no mutation or runtime notification;
   existing same-set session application retries remain supported. Successful responses name
   their own commit, and reads pair membership with its desired state.
2. Updated web callers submit only the accepted direct baseline. Conflict,
   handled interlock, ordinary denial, uncertain ACK, and stale-context outcomes
   remain distinct. Newer drafts survive older completion; no null result
   advances a settings baseline. Disabled existing entries can remain/remove.
3. Existing authorization and workspace/profile/session ownership are enforced
   at read and mutation. New additions to disabled/removed/ineligible owners
   fail safely; accepted application evidence remains authoritative.

## UI preview

UI-03 from [full previews](plan.md#ui-03-partial-success-and-unavailable-targets):
the existing owner editor shows "Choices changed" and Reload choices, retains
the unsaved draft, and requires explicit reconciliation. Shared Save changes
remains the settings commit action. Session errors remain in Customize.
No new overlay or responsive layout is introduced by this work order.

## Verification

From repository root, after an explicit implementation request. First run the
new focused tests red, implement, then run the full block green. Install once
before the first pnpm command in this worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/backendapp -run 'MCP.*(Selection|Assignment|Owner)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/settings/store -run 'PostgresMCPSelection' -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-mcp-selection-editor.test.ts hooks/domains/workspace/use-mcp-selection-editor-context.test.ts lib/api/domains/mcp-api.test.ts components/settings/repository-mcp-selection.test.tsx components/mcp/mcp-selection-feedback.test.ts hooks/domains/session/use-task-session-mcp-selection.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

Set `KANDEV_TEST_POSTGRES_DSN` to a disposable PostgreSQL database for the race
command. A skipped test is not PostgreSQL evidence. Run
`bash scripts/lint-go-changed <each-changed-go-file>` with the actual file list
from this work order; record the expanded command and result. If storage SQL
changes, also run the required repository checks:

```bash
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
```

The new `mcp-api.test.ts` and settings tests above are planned files. Add
transactional cases in `mcp_selection_concurrency_test.go` and
`postgres_mcp_selection_concurrency_test.go`, plus
`internal/backendapp/mcp_assignment_eligibility_test.go`. Inspect current test
names before relying on `-run`; prove intended tests execute.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/selections.go` and new focused tests.
- `apps/backend/internal/agent/settings/store/mcp_selections.go`, related
  catalog mutation/legacy import helpers, and new concurrency tests.
- `apps/backend/internal/agent/settings/handlers/mcp_selection_handlers.go`
  and focused handler tests.
- `apps/backend/internal/backendapp/mcp_selection.go` and eligibility tests.
- `apps/web/lib/types/http-mcp.ts`, `lib/api/domains/mcp-api.ts`,
  `hooks/domains/workspace/use-mcp-selection-editor.ts`, repository/profile
  save contributors, their tests, and shipped locale catalogs for conflicts.

## Dependencies

No shipping prerequisite beyond the verified parent. This work order owns
generic conditional-write admission, response/error naming, shared transaction
locking, and session revision handling. The exclusions sibling's work order 01
depends on its committed result. Publish the exact operation/lock/type contract
and commit SHA in Results so the sibling extends the same session transaction.

Use one `selection_revision`/`expected_revision` contract for sessions, with
one existing desired-state counter. Non-session scopes use the direct-membership
condition. No session expected-ID-set condition or new revision table is added.
Explicit same-set application Retry advances desired revision as before. A
stale revision rejects first; a successful retry preserves accepted setup,
applied revision, and attempt ID. Lost ACK reconciliation reads before retry.

Exclusion fields, validation, storage, resolver changes, and mixed-client tests
remain sibling-owned. Its atomic extension must advance desired revision on an
exclusion-only change and preserve omitted exclusions on additions-only writes.
No exclusions implementation is required to complete this work order. Do not
import uncommitted code, implement sibling scope, or establish competing writers.

## Risks

PostgreSQL empty-set races, lock-order deadlocks, late ACKs, legacy callers
without a condition, and accidentally replacing session accepted-state fields.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/workspace-mcp-assignment.md)
- [Conditional replacement and eligibility](../../specs/agents/system-design/workspace-mcp-assignment.md#conditional-replacement)
- Existing `selections_test.go`, `postgres_mcp_consistency_test.go`,
  `use-mcp-selection-editor-context.test.ts`, and settings save coordinator.
- Backend/frontend AGENTS guidance and `/tdd`; load backend testing reference.

## Results

Implemented 2026-10-23. Repository/profile component cases share the parameterized
`repository-mcp-selection.test.tsx`; no duplicate profile test file is needed.

The shared extension point is `ConditionalSelectionRepository` in
`mcpconfig/selection_write.go`: `SelectionCondition`, `SelectionSnapshot`,
`ReadMCPSelectionSnapshot`, and `ReplaceMCPSelectionsConditional`. The settings
store owns the transaction. PostgreSQL serializes each workspace/scope/owner
with `pg_advisory_xact_lock(hashtextextended(key, 0))`; SQLite takes writer
admission before the baseline read. Definition rows are locked in sorted order.
Legacy profile import uses the same owner lock and replacement helper. Catalog
deletion locks its definition before cleaning selection references.
Session GET and successful PUT return `selection_revision` from the same snapshot
as direct membership. Session writes advance existing desired revision atomically,
including explicit same-set Retry, without replacing accepted setup evidence.
Notify occurs only after commit. Exclusions extends this same transaction;
assignment does not implement exclusions or a second revision counter.

Verification:

- RED: stale HTTP conditions initially returned 200; web callers initially
  omitted conditions, allowed duplicate calls, and lost newer drafts. Added
  tests reproduced each failure before implementation.
- GREEN: focused mcpconfig/store/handler race tests; backendapp owner/eligibility
  tests; 36 frontend tests across six files, including both settings owners.
- Disposable PostgreSQL 17.11: actual concurrent writers (repository and session),
  legacy import, deletion, consistent reads and confirmed deletion passed under
  `-race`. No skipped PostgreSQL evidence; temporary server stopped afterward.
- SQL guard and store conformance (`-race`) passed. Web typecheck passed with
  an 8 GiB Node heap (the default 4 GiB exhausted memory). i18n checks passed.
- Changed Go lint passed with repository-pinned golangci-lint v2.9.0.
- An initial broader MCP race filter also selected the unrelated native-command
  cancellation test, which returned nil instead of cancellation. The exclusions
  sibling later traced it to the fixture's `/usr/bin:/bin` PATH on Nix, where
  `sleep` lives outside those directories. Its committed one-line test fix from
  `442e9f01f77bd091f9d12d8867fa027bc74cf83e` was imported by path only; sibling
  plan files and production command-runner code were not changed. The broader
  command then passed in this workspace:
  `go test -trimpath -race ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/backendapp -run 'MCP|Selection|Effective' -count=1`
  (from `apps/backend`). Changed-file Go lint passed with zero issues.

Commit identity is recorded in the saved task plan and coordination message
immediately after the commit (a commit cannot contain its own SHA).

### 2026-10-25 saved-comparison correction

Consumed the minimal committed correction from exclusions remote head
`69be161548702b4be530536f925777da628302e0`: session-hook `savedSelectedIds`,
the selector's optional saved-ID prop, and comparison against that committed
snapshot rather than the retained checkbox draft. Imported only the two
neighbor helper assertion/reload adjustments from that same commit. No exclusion
fields, controls, resolver, storage, or whole-branch prerequisite was introduced.
Exclusions was notified to reconcile these identical hunks once on its next rebase.

RED: the existing hook conflict test could not read saved IDs; the desktop
pending-changes journey lost all three saved comparison rows after a rejected
uncheck. GREEN: 43 focused unit/consumer tests, 10 desktop E2E cases and 8 phone
E2E cases passed, with no browser retries. Typecheck (8 GiB heap), focused ESLint,
63 public-doc validator tests and the 54-page docs validation passed. Public
explanation now distinguishes the saved pending list from the retained draft.

Browser commands used the managed host runner, one shard, sequential projects,
and isolated fixtures. Desktop covered `tests/chat/mcp-pending-changes.spec.ts`
plus `tests/task/task-mcp-selection.spec.ts`; phone covered their `mobile-`
counterparts. RED log: `/tmp/kandev-run.e2e.alcZxHxm.log`; GREEN logs:
`/tmp/kandev-run.e2e.H7JKgokB.log` and `/tmp/kandev-run.e2e.DS7TdSx5.log`.
These mock-provider checks do not certify live endpoint/provider behavior.

### Local verification for the remediation

- Frontend: 76 tests across eight dialog/assignment suites, plus 30 tests across
  four shared editor/draft/session suites passed. The suspended-render regression
  failed with a null accepted result before the layout-effect fix and passed
  afterward. Full typecheck and affected ESLint passed.
- Backend: the real settings-store/session-service regression rejected the
  internal launch write before the fix. Afterward the MCP/Selection/Effective/
  Usage/ImportStatus race command passed in mcpconfig, settings handlers and
  backendapp. The same regression preserves HTTP rejection, workspace isolation,
  disabled-definition rejection and desired-state advancement. Changed-package
  Go lint passed with zero issues.
- Browser: 14 desktop journeys and 21 phone journeys passed, retries disabled.
  These include the new task-edit recovery, delayed destination check, exact
  session review and all three reported failing E2E scopes. The final binary's
  phone assignment/recovery subset is rerun before publication. Fresh synthetic
  images replace affected evidence. No real endpoint connection was tested.
- Catalog/spec/public-doc checks passed. Incoming main's harness change also
  passed harness and spec validator tests/lint. Normal commit hooks and exact-head
  remote CI/review recheck remain delivery steps, not completed local evidence.

Commands and logs: focused Vitest results are recorded in
`/tmp/pr4760-fixup-unit-final.log` and `/tmp/pr4760-fixup-guards-final.log`;
backend race in `/tmp/pr4760-review-backend-green.log`; browser runs used managed
host mode, one shard and sequential projects (`/tmp/kandev-run.e2e.YETxqUNT.log`,
`/tmp/kandev-run.e2e.RhOn7ji6.log`). Authoritative base for this remediation is
main `b4e9cf5ede3545ca6a8e58165e580ad7c4988afa`.

### Review correction: database collation (2026-10-27)

The existing set-comparison contract also applies when PostgreSQL returns IDs
in non-bytewise collation order. A partial Claude backend review identified
this concern. The regression reproduces a false conflict with normal UUIDs
under PostgreSQL 17 numeric ICU collation. Comparison now sorts a copy of the
saved IDs in Go, preserving the saved response order. Expected membership,
owner locking, session revisions, conflict/no-notify and legacy omission
semantics are unchanged. Unit coverage checks permutation equivalence, stale
membership rejection and input immutability; an isolated real-PostgreSQL test
checks numeric collation and rejected-write preservation. Focused RED/GREEN
passed; broader race validation and delivery are recorded in the plan ledger.
