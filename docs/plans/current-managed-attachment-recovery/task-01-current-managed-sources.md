---
id: "01-current-managed-sources"
title: "Recover admitted current managed sources"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.6
system_design:
  - ../../specs/tasks/system-design/current-managed-attachment-recovery.md
---

# Task 01: Recover admitted current managed sources

## Summary

Implement the source-attachment vertical slice: exact managed-path admission,
existing scoped cloner integration, verified persistence and HTTP/MCP Local/
Worktree consumer regressions. Preserve ordinary launch and remote routing.

## In scope

- Add operation-specific repoclone admission for the current destination and
  existing supported legacy paths. Use current path derivation and origin/Git
  helpers, exact original filesystem evidence, and the existing clone mutex.
- Apply it before reuse/clone side effects and to returned clone output. If the
  necessary small mutation guard cannot fit existing ownership, report the
  exact missing primitive before adding a general framework.
- Preserve explicit local rows even with provider metadata, pathless nonlocal
  legacy identity eligibility, existing legacy reuse and absent-legacy refusal.
- Use executor's exact credential request and existing provider lanes; validate
  output before fatal required path persistence through the existing updater.
- Fix source ensureHostRepositoryPaths nonempty-path bypass. Preserve source
  idle/all-live-session/environment admission and existing compensation.
- Author real registered HTTP and guarded task-surface MCP consumer tests on
  Local and Worktree; seed a real owner and task/session principal, use actual
  Git/cloner/SQLite with runtime rescan stubbed. Observer asserts exact scope.

## Out of scope

Direct branch clone wiring (Task 02), normal launch/resume policy, selected-slot
recovery, remote materialization, automatic legacy recreation, schema/registry/
flag/global security work, public docs and UI. No production/permanent tests
until explicit implementation request; no delivery before parent validation.

## Acceptance

1. Missing exact current source recovers through the real cloner; empty source
   continues to work; readable content, requested base and canonical inventory
   are verified through both consumers on both host executor families.
2. Foreign/control-redirect/output/auth/session/persistence failures cannot use
   an unverified path; local and absent legacy sources never clone, and valid
   existing legacy reuse does not rewrite its saved path. Foreign snapshots
   and all prior task state survive rejection.
3. Mixed batch late failure compensates task-owned effects while retaining a
   verified shared clone and its successful path update; exact retry reuses it.
   Ordinary launch behavior and remote host-cloner call counts are unchanged.

## Verification

From repo root, after a later explicit implementation request:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/repoclone -run '^TestManagedAttachmentPathAdmission$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run '^TestHostAttachmentRepositoryRecovery.*$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^(TestManagedSourceAttachmentConsumers|TestManagedSourceAttachmentReimportRegistration)$' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/repoclone ./internal/orchestrator/executor ./internal/backendapp -run '^(TestManagedAttachmentPathAdmission|TestHostAttachmentRepositoryRecovery.*|TestManagedSourceAttachmentConsumers|TestManagedSourceAttachmentReimportRegistration|TestEnsureRepositoryCloned_ReturnsOnlyLocalPath|TestRepositoryHostClonerRequiresExactSessionScope|TestWorkspaceSourceMaterializer_(Remote.*|PrelaunchReturnsExplicitDeferredResult|UnprovisionedEnvironmentDefers))$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run each new focused command RED before its relevant production edits and GREEN
afterward; preserve expected error and source/DB readback, not merely exit code.
Undefined planned symbols or other compile failures do not establish causal RED;
the consumer regression must compile against base behavior and fail assertions.
`TestManagedSourceAttachmentConsumers` must test the actual registered handlers,
not only call the materializer directly. Use the [plan matrix](plan.md#tests-and-causal-redgreen).
Run `bash scripts/lint-go-changed` from root with every changed Go file named
explicitly from this work order's final diff; record that exact invocation in
Results. No full-backend lint or generic verification pass.

## Files likely touched

- `apps/backend/internal/repoclone/managed_attachment_path.go` (new)
- `apps/backend/internal/repoclone/managed_attachment_path_test.go` (new)
- `apps/backend/internal/repoclone/clone.go` (small existing-lock admission seam)
- `apps/backend/internal/orchestrator/executor/host_repository_materialization.go` (new)
- `apps/backend/internal/orchestrator/executor/host_repository_materialization_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_resume.go` (host seam only)
- `apps/backend/internal/backendapp/workspace_source_materializer.go`
- `apps/backend/internal/backendapp/managed_source_attachment_test.go` (new)
- This work order and plan results.

## Dependencies

None. Requirements/design package review and later implementation authorization
are prerequisites, not an implied permission to start tests now.

## Risks

A returned path can be valid Git yet belong to a different workspace/provider.
A nonfatal updater or legacy absence fallback can make a nominally successful
consumer violate recovery scope. Preserve normal launch behavior separately.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/managed-source-attachment.md)
- [Design](../../specs/tasks/system-design/current-managed-attachment-recovery.md)
- [ADR](../../decisions/2026-10-21-current-managed-attachment-recovery.md)
- Existing backendapp `branch_materializer_test.go` fixture helpers,
  `workspace_source_materializer_test.go` boundary tests, executor auth tests and
  repoclone origin tests. The temporary independent diagnostic in
  `../4426-investigation/` is evidence, not permanent coverage.

## Results

Implemented sequentially after explicit parent authorization. Causal RED:
`task01-consumer-red.log` records actual registered HTTP/MCP missing-current and
foreign-checkout assertions failing on base behavior; `task01-executor-red.log`
records scope/output/persistence rejection failures; `task01-path-red.log`
records runtime absence of the inspection capability, not a compile failure.
Additional session/root-change and relative-base RED logs led to bounded guards.

GREEN evidence in `../4426-investigation/`:

- Required scoped race selection, including denied credential scope, re-import
  and unscoped wrapper compatibility: `task01-final-checked.log`, PASS repoclone
  1.215s, executor 1.112s, backendapp 29.674s. Subsequent assertion-only source strengthening passed with race in
  `task01-source-final.log`, backendapp23.780s; additional cancellation/unavailable
  cloner/inspection executor cases passed `task01-executor-final.log`,1.094s.
- Source matrix: 56 cases (HTTP/MCP x Local/Worktree x14 scenarios), real Git,
  SQLite, materializer/executor/cloner, owner identity and exact request count
  and scope; foreign file-content snapshots unchanged; active inventory restored
  after mixed-batch failure, tombstones retained by existing lifecycle, retry
  reuses the verified shared clone. Registration handler control also passes.
- Path admission includes relative configured base, existing/missing legacy,
  redirected Git metadata and replaced ancestor during credential resolution.
  `task01-path-final.log` passed before the relative-base addition; the final
  scoped race log covers that addition and final helper extraction.
- `task01-lint-checked.log`: 0 issues for all ten changed Go paths. Final two
  changed test files additionally passed `task01-test-lint.log`,0issues.
- Catalog and specification lint PASS in `task01-catalog.log` and
  `task01-spec-lint.log`; `git diff --check` PASS.

Exact full lint invocation from repo root:

```bash
bash scripts/lint-go-changed apps/backend/internal/repoclone/clone.go apps/backend/internal/repoclone/managed_attachment_guard.go apps/backend/internal/repoclone/managed_attachment_path.go apps/backend/internal/repoclone/managed_attachment_path_test.go apps/backend/internal/orchestrator/executor/executor_resume.go apps/backend/internal/orchestrator/executor/host_repository_materialization.go apps/backend/internal/orchestrator/executor/host_repository_materialization_test.go apps/backend/internal/backendapp/workspace_source_materializer.go apps/backend/internal/backendapp/managed_source_attachment_test.go apps/backend/internal/backendapp/managed_attachment_reimport_test.go
bash scripts/lint-go-changed apps/backend/internal/backendapp/managed_source_attachment_test.go apps/backend/internal/orchestrator/executor/host_repository_materialization_test.go
```

The new `managed_attachment_guard.go` carries only this attachment operation's
repository snapshot into existing clone locks; it is not a generic lock or
filesystem capability model. The unscoped compatibility wrapper retains its
old behavior; normal launch private helpers remain unchanged. Provider request
proof is GitHub/scoped-plugin denied-auth coverage, not real provider acceptance.
No native Windows/macOS, server-in-Docker, live data, deployment or release
acceptance claimed. These are Task01-stage receipts; completed Task02 extends
the source matrix to 68 cases and records final combined checks. Delivery remains
unauthorized.

## Historical PR remediation checkpoint: persistence seam gap

Reopened under implementation-only parent authorization; no commit/push before
actual-diff validation. The bounded attachment-only scope rejection, configured
control-root inspection, physical Git comparison and exact whitespace/newline
parsing are implemented uncommitted. New path/authentication tests pass; existing
source/executor controls pass in the final scoped race run. Causal registered
consumer RED preceded production edits.

Source alias recovery still exposes the precise verified-path persistence gap:
`Service.UpdateRepository` canonicalizes the admitted lexical path to a physical
path. No bypass or change to ordinary explicit-local canonicalization was added.
Return the missing service-owned authenticated/version-fenced lexical managed-path
write seam for parent review, as directed. See the [plan checkpoint](plan.md#historical-pr-remediation-checkpoint-persistence-seam-gap)
for alternatives, receipts and exact commands. This work order remains in progress.

## Approved internal persistence seam implementation

The parent inspected the helper gap and explicitly authorized the narrow internal
`Service.UpdateVerifiedManagedRepositoryPath` entrypoint. Only the attachment
adapter may call it after repoclone/executor admission. Reuse persisted-workspace
authorization, full admitted identity/version comparison, existing exact updater
and repository event publication. Preserve current unrelated fields, bindings
and checkout values; reject explicit-local, invalid/stale/missing input. Ordinary
`UpdateRepository` canonicalization remains unchanged. No public bypass field,
HTTP/MCP flag, schema, registry, token model or direct backendapp database write.

Implementation and service RED/GREEN plus the same actual 40-case consumer matrix
are complete and uncommitted for parent validation. The historical blocker above is settled
by this exact boundary, not a change to clone layout or realpath admission.
Return the complete uncommitted remediation for parent actual-diff validation
before any commit/push or valid review-thread resolution. Codex retry belongs
after a later validated push. No merge, issue reply/closure or release claim.

## Verified internal write results

Implemented the parent-authorized internal write without changing ordinary
repository canonicalization. Service tests: 5 preservation/compatibility,
32 invalid/local/denied/stale admission and 6 persistence/CAS/cancellation cases
PASS. The ordinary explicit-local canonicalization control remains PASS.
All 40 actual compatibility consumers now PASS, including exact lexical SQLite
readback for empty/saved-missing parent aliases and trailing whitespace/newlines.
Existing source68/branch20/path11 plus new path9/auth2 and executor8/denied2
cases PASS, no skips. The historical ten alias failures above are corrected.

The final combined race selector ran after the last production edit and is
retained in `verified-service-final-race.log` with its exit0 receipt and parsed
`verified-service-final-test-summary.json`. Final lint covers all 19 Go paths
in the full PR candidate, not only this increment; exact command/exit is in
`verified-service-checks.json`. Source/test hashes are frozen across checks.
Final documentation coverage, catalog/spec/public-doc and whitespace receipts
are in that same external evidence directory.

Caller provenance is narrow: the only production call is the attachment-specific
`repoLocalPathUpdater` after repoclone/executor admission. No public request flag,
direct backendapp storage write or general validation bypass was added. The
service authorizes the current persisted workspace, compares full identity and
observed version, updates path/version through existing exact CAS and publishes
through the ordinary repository event helper. Current settings, bindings and
checkout values survive; unrelated caller snapshot fields are ignored. Existing
publisher delivery semantics remain unchanged; read/validation/cancellation and
write errors are fatal. No schema, token, registry or cleanup model was added.

Status: `remediation_validation_ready`, pending parent actual-diff validation.
No remediation commit/push, valid-thread resolution, Codex retry, merge or public
issue reply has occurred. Valid review corrections and bounded Codex retry await
an eventual approved push. Partial #4426 only, no closing keywords. Linux/offline
Git/SQLite evidence only; native Windows/macOS, real provider/JSON-RPC/agent,
isolated server-in-Docker and release acceptance remain unverified. Shared build
admission serialized the checks; it was not bypassed.
