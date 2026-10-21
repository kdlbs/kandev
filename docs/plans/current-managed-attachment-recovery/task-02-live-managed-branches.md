---
id: "02-live-managed-branches"
title: "Materialize live managed branches and document partial recovery"
status: done
wave: 2
depends_on:
  - 01-current-managed-sources
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.6
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
  - ../../specs/tasks/system-design/current-managed-attachment-recovery.md
---

# Task 02: Materialize live managed branches and document partial recovery

## Summary

Apply Task 01's validated source recovery to direct live Worktree branches and
nested batch branches after canonical session/environment selection. Preserve
existing no-restart branch behavior and document the partial recovery scope.

## In scope

- Wire authenticated host cloning into direct branch materializer at startup
  and into branches built by the source materializer; avoid two implementations.
- Move pathless decision after actual selected session/environment eligibility,
  inherited owner validation and expected-target checks. Use the selected
  session for cloning, retain pre-create revalidation and canonical inventory.
- Write registered guarded MCP branch-to-Git/SQLite regressions for empty and
  saved-missing current paths, readback/result paths and requested branch/base.
  Cover existing legacy/local/foreign controls, failed persistence and retry,
  prelaunch versus live, inherited/rebound environment and no CWD/restart change.
- Prove a target change during cloning does not write task worktree/inventory
  or publish success under stale identity; verified shared clone retention is
  allowed, with no copied runtime/session recovery logic.
- Use docs-maintainer to add concise reference guidance to existing attachment
  docs only after implementation. Explain current-only recovery and missing
  legacy refusal, safe explicit re-import, and server-Docker/task-Docker distinction.

## Out of scope

Remote or Local support for legacy add-branch, batch idle bypass, new UI layout,
provider sandbox expansion, ordinary checkout relocation, legacy recreation,
schema/reset flows, issue closure, deployment/release claims and other issues.
No merge/public reply or new task/session/agent. Parent delivery gate persists.

## Acceptance

1. Eligible live branch returns a readable sibling worktree, exact existing
   response paths and inventory under its selected (possibly inherited)
   environment; clone receives that task/session. Genuine prelaunch defers;
   unresolved/non-Worktree live targets fail before host clone effects.
2. Foreign, absent legacy, unsafe Git, authentication, target-change and
   persistence failures compensate new task-owned effects and preserve prior
   state. Direct branch never invokes batch rebind or changes agent CWD.
3. Permanent source/branch consumer suites and existing boundary guards pass;
   public docs state partial scope and actionable existing re-import guidance.
   No result is represented as full #4426 resolution or release acceptance.

## Verification

From repo root, after Task 01 and implementation authorization:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^TestManagedBranchAttachmentConsumers$' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/backendapp -run '^(TestManagedSourceAttachmentConsumers|TestManagedBranchAttachmentConsumers|TestRepositoryHostClonerRequiresExactSessionScope|TestWorkspaceSourceMaterializer_(Remote.*|PrelaunchReturnsExplicitDeferredResult|UnprovisionedEnvironmentDefers)|TestBranchMaterializer_(Inherited.*|LiveSessionBindingOverridesStaleOwnedEnvironment|RejectsSessionReboundBeforeCreate))$' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/current-managed-attachment-recovery
```

Run new branch consumer cases RED before branch wiring, then GREEN. Keep exact
baseline failure (empty branch lacks physical worktree, saved-missing path is
not a Git repository), and after wiring assert the physical result and DB rows.
For inherited and target-change cases use actual service/materializer fixtures,
not only a branch result stub. Keep the running state branch compatibility
case; a waiting-only fixture does not prove active-turn behavior.

Run `bash scripts/lint-go-changed` from root with all Go files changed by this
work order named explicitly and record the exact command in Results. Rerun
Task 01 tests only if shared admission/auth code changes; otherwise the combined
consumer command and original Task 01 evidence remain authoritative. Run the
PR-documentation coverage preflight on the full package using repository
`validateCoverage` and record accepted references. No generic QA/verify task.

## Files likely touched

- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/backendapp/host_repository_materialization.go` (new adapter)
- `apps/backend/internal/backendapp/workspace_source_materializer.go` (shared wiring)
- `apps/backend/internal/backendapp/main.go` (host clone wiring)
- `apps/backend/internal/backendapp/orchestrator.go` (existing conditional path-update adapter)
- `apps/backend/internal/backendapp/managed_branch_attachment_test.go` (new)
- `docs/public/automation-and-mcp.md` (existing reference section)
- This work order and plan results; owning design only for agreed implementation facts.

## Dependencies

[Task 01](task-01-current-managed-sources.md). Reuse its admitted clone operation
rather than reproducing origin/path rules in branch code.

## Risks

Pathless early return can bypass live environment checks. Different branch and
batch session selectors can be confused; each must retain its existing selected
identity, not borrow whichever session cloned first. Re-import guidance must
not recommend destructive changes to a live referenced repository; report a
missing safe registration primitive instead of inventing an automatic reset.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/managed-source-attachment.md)
- [Focused design](../../specs/tasks/system-design/current-managed-attachment-recovery.md)
- [Branch environment design](../../specs/tasks/system-design/attach-workspace-sources.md#legacy-add-branch-effective-environment)
- Existing `branch_materializer_test.go`, source materialization fixtures,
  `Service.AddBranchToTask`, and public MCP reference section.

## Results

Implemented sequentially after Task01 under explicit parent implementation
authorization. Status: `validation_ready`, uncommitted and delivery unauthorized.
This remains a partial disposition of #4426; missing legacy recovery is excluded.

Causal registered-consumer RED is retained in `../4426-investigation/`:
`task02-consumer-red.log` records empty/saved-missing branch recovery, foreign/
absent-legacy admission and failed-persistence assertions before branch changes.
`scoped-identity-red.log` exposed rejection of paired scope/immutable-ID sources
without owner/name. `verified-persistence-identity-red.log` exposed success after
a repository identity change during cloning. The fix uses the existing
`UpdateRepositoryRequest.ExpectedUpdatedAt` conditional write through the
verified-path adapter; it adds no persistence model. Both empty and saved-missing
current paths now exercise that concurrent identity case. These are behavioral
failures, not undefined-symbol RED.

Final combined race command from `apps/backend`:

```bash
go test -trimpath -race -tags fts5 ./internal/repoclone ./internal/orchestrator/executor ./internal/backendapp -run '^(TestManagedAttachmentPathAdmission|TestHostAttachmentRepositoryRecovery.*|TestEnsureRepositoryCloned_ReturnsOnlyLocalPath|TestManagedSourceAttachmentConsumers|TestManagedSourceAttachmentReimportRegistration|TestManagedBranchAttachmentConsumers|TestRepositoryHostClonerRequiresExactSessionScope|TestWorkspaceSourceMaterializer_(Remote.*|PrelaunchReturnsExplicitDeferredResult|UnprovisionedEnvironmentDefers)|TestWorkspaceSourceMaterializerKubernetesUsesRemoteMaterialization|TestWorkspaceSourceMaterializerRejectsUnknownExecutor|TestBranchMaterializer_(Inherited.*|LiveSessionBindingOverridesStaleOwnedEnvironment|RejectsSessionReboundBeforeCreate))$' -count=1 -v
```

`final-validation-race.log`: PASS repoclone 1.225s, executor 1.119s,
backendapp 42.780s. Source consumers 68/68, branch consumers 20/20, path admission
11/11, executor recovery 8/8, denied scoped credentials 2/2; none skipped.
Includes actual active-turn branch, inherited canonical environment, prelaunch
child, session/environment rebinding, conditional persistence failures and retry,
physical requested-branch checkout, SQLite inventory and unchanged foreign file
contents. Source tests use registered HTTP and guarded MCP consumers; branches
use guarded MCP. Runtime rescans are stubbed; no live agent is started.

Final exact scoped lint command from repo root (`final-validation-lint.log`,
0 issues):

```bash
bash scripts/lint-go-changed apps/backend/internal/repoclone/clone.go apps/backend/internal/repoclone/managed_attachment_guard.go apps/backend/internal/repoclone/managed_attachment_path.go apps/backend/internal/repoclone/managed_attachment_path_test.go apps/backend/internal/orchestrator/executor/executor_resume.go apps/backend/internal/orchestrator/executor/host_repository_materialization.go apps/backend/internal/orchestrator/executor/host_repository_materialization_test.go apps/backend/internal/backendapp/workspace_source_materializer.go apps/backend/internal/backendapp/managed_source_attachment_test.go apps/backend/internal/backendapp/managed_attachment_reimport_test.go apps/backend/internal/backendapp/branch_materializer.go apps/backend/internal/backendapp/host_repository_materialization.go apps/backend/internal/backendapp/main.go apps/backend/internal/backendapp/orchestrator.go apps/backend/internal/backendapp/managed_branch_attachment_test.go
```

Additional credential controls from `apps/backend` (`provider-auth-controls.log`,
PASS 1.792s):

```bash
go test -trimpath -race -tags fts5 ./internal/repoclone -run '^(TestEnsureWorkspaceClonedWithBasicAuthKeepsCredentialScopedToGitChild|TestEnsureWorkspaceClonedUsesSelectedCredentialWithoutAmbientFallback|TestEnsureWorkspaceClonedRequiresExplicitGitHubCredential|TestWorkspaceProviderRepositoryPathSeparatesOpaqueScopesAndImmutableIDs)$' -count=1 -v
```

Public-doc checks use the installed Node executable
`/home/jcfs/.local/share/mise/installs/node/24/bin/node` because bare `node` is
absent from this shell PATH: `--test scripts/validate-public-docs.test.mjs`
PASS 63 tests; `scripts/validate-public-docs.mjs` PASS 54 pages. Final catalog,
specification lint, actual-diff PR-doc coverage and whitespace checks are saved
in the external validation receipt. Re-import guidance is proven by the actual
registered repository-create HTTP handler: separate empty-path provider row,
original legacy registration untouched. No live referenced row deletion advised.

Primary base/HEAD remain `ce6ed5c971137a3c1d1360d75c111b51fde980d5` on
`feature/investigate-missing-vg2`. A disposable detached checkout at advanced main
`07fa2d5e792731b493ec408c9f103edeea392bce` accepted the full candidate patch
without conflict and passed the same race selection (1.247s/1.151s/45.311s),
catalog/spec lint and public docs. All 15 Go files were byte-identical to the
primary tree; the owned checkout was removed after commands completed. Receipt:
`advanced-main-receipt.json`. Final documentation results/status edits postdate
that integration patch; no merge/rebase or commit was performed.

Linux isolated execution only. No native Windows/macOS, real provider credentials,
JSON-RPC server/agent stream, browser runtime, isolated server-in-Docker, deployment
or release acceptance is claimed. Remote lifecycle boundary checks use stubs.
Credential tests prove existing request/control seams with offline local Git and
denied/fake resolvers, not external provider availability. Existing operation
clone locks and ancestor inspection do not promise protection against arbitrary
external filesystem writers. Contributor provenance remains in the focused design.

## Historical PR remediation checkpoint: persistence seam gap

Reopened under implementation-only parent authorization. The new compatibility
matrix covers registered HTTP/MCP sources on Local/Worktree and guarded MCP
branches, with physical content and lexical path readback. Scoped legacy refusal,
existing parent aliases and exact whitespace/newline cases now pass. Missing
alias clones are created, but the verified updater's general service entry
canonicalizes their saved path. Worktree/branch consumers then reject that path;
Local consumers can report success while storing the wrong lexical identity.
The added empty-alias cases demonstrate why merely omitting an unchanged saved
path from the update would not complete recovery.

The existing source68/branch20 consumer suites remain GREEN. The remediation
matrix remains partially RED on the persistence gap; do not claim merge readiness.
See the [plan checkpoint](plan.md#historical-pr-remediation-checkpoint-persistence-seam-gap)
for exact receipts/commands and the narrow missing primitive. Valid review threads
remain unresolved pending reviewed corrections; Codex retry remains deferred until
an eventual validated push. No automatic invalid-clone deletion or new cleanup
policy: see [shared clone lifetime](../../specs/tasks/system-design/current-managed-attachment-recovery.md#failure-retries-and-resource-lifetime).
No remediation commit/push, issue closure or public issue reply authorized yet.

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
