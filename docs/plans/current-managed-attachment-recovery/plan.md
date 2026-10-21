---
created: 2026-10-21
status: implemented
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
  - ../../specs/tasks/system-design/current-managed-attachment-recovery.md
legacy_specs: []
---

# Implementation plan: Current managed source attachment recovery

## Overview and checkpoint

Restore attachment for missing current managed clones through the existing
scoped host cloner. Implement source attachment first, then wire direct branch
attachment to the same admission. This is option A selected after independent
#4426 investigation, a partial disposition while missing legacy recreation
remains excluded. Reference #4426 without closing keywords in future delivery.

Parent reviewed the eight-Markdown design package (SHA256
`00ba47afb96b8c7123bb7c20b63f442565b4946fd39dcbd02e4cd7f932ce708f`)
and explicitly authorized sequential implementation in this child/session. Both
work orders are implemented and uncommitted for actual-diff validation.
Commit/push/PR/merge/public reply still require
parent actual-diff validation and `delivery_authorized`. No extra agents,
platform tasks or sessions. No #4529, #4477 or other-loop repository-save work.

## Scope

Recover empty provider paths and missing exact CURRENT managed destinations in
eligible server-side Local/Worktree source attachment and Worktree branch
attachment. Validate preexisting/output path, origin and Git control ownership;
preserve existing validated legacy and explicit local sources. Missing legacy
paths fail with re-import guidance. Keep exact identity, verified persistence,
canonical inventory and existing task-owned compensation.

Exclude missing legacy recreation, selected-checkout relocation, remote executor
changes, schemas, flags, registry, new recovery records, global auth/persistence
models, UI composition and live data/deployment/overlay removal. Existing
completed attach/current-workspace/inherited-branch plans remain historical;
this package does not reopen their completed work orders.

## Settled intent and evidence

Tasks own this capability because success and compensation change canonical task
attachment/inventory. Workspace/provider identity and clone placement retain
their owners. Parent selected current-only recovery after reviewing the
[ADR](../../decisions/2026-10-21-current-managed-attachment-recovery.md); the
broader missing-legacy alternative remains deferred, not silently accepted.

Base/head of investigation: `ce6ed5c971137a3c1d1360d75c111b51fde980d5`.
Independent temporary consumer probe used actual registered HTTP source,
guarded task-surface MCP source/branch, service, SQLite, executor/cloner and Git;
rescan stubbed, local transport. Of 35 cases, six current-path recovery
assertions and five foreign-path rejection assertions were RED, 24 controls
passed. Empty source already succeeds. Existing scoped boundary suite passed
14 top-level tests. Final diagnostic backendapp time 9.642s; boundary3.440s.
These are diagnosis/control results, not implementation GREEN.

Evidence lives outside the repo in this task's `../4426-investigation/`:
`report.txt`, complete `issue4426_probe_test.go`, `consumers-final.log`,
`consumer-results.json`, `existing-boundaries.log`, provenance and manifest.
Preserve before archival. Diagnostic report SHA256:
`f591ef88df34a79110adb94cd3d8c26f2e9c0a4b8109f73c77e74a1822c09643`.

## Technical approach

Follow the [focused design](../../specs/tasks/system-design/current-managed-attachment-recovery.md).
`WorkspaceProviderRepositoryPath` derives exact current identity; do not copy its
hash/host/owner layout into materializers. Add only the missing operation-specific
managed attachment admission in repoclone and the small clone-lock guard seam
needed to apply it before mutation. Keep `GitCredentialRequest`, provider
credentials and classified subprocess handling authoritative. Inspect existing
checkout and returned clone before verified path persistence; host attachment
persistence failure is fatal without changing normal launch behavior.

Source: `workspaceSourceMaterializer.ensureHostRepositoryPaths` must classify
nonempty managed paths instead of skipping them. Preserve local ownership and
batch/session/environment guards. Branch: resolve its canonical selected session
and ready Worktree environment before the empty-path decision; use the same
cloner and recheck target before worktree/inventory writes. Wire branch clones
both at startup and when source materialization constructs nested branches.

Do not grow an unrelated Git security framework. If existing path/subprocess/
cloner primitives cannot carry inspected evidence to the mutation boundary,
return that precise gap before expanding the implementation. Keep the failed
foreign-path probe as ownership-admission regression evidence only.

### Provider and executor compatibility

| Shape | Intended behavior and evidence | Unsupported fallback |
| --- | --- | --- |
| GitHub managed, HTTPS/SSH locator | Existing provider canonicalization and exact-scoped credential request; current path recovery and origin proof | Existing missing-host/credential error, no unauthenticated fallback |
| GitLab managed, default/self-managed host, nested owner | Existing credential seam; exact host/path identity; local-transport consumer plus denied-auth request tests | Propagate denied/unavailable credentials |
| Azure DevOps managed HTTPS/PAT | Existing basic-auth lane; shared admission must also precede its mutation | If inspection cannot be applied to that lane, report missing seam before claiming support |
| Scoped plugin identity | Paired scope/immutable ID hashed by existing helper, declared clone URL origin; scoped fake credential resolver test | No unscoped legacy adoption, no owner/name-only identity |
| Existing supported legacy source | Exact saved path, standalone Git/control/origin proof, no relocation or fetch on reuse | Absent/unsafe legacy: refuse with re-import guidance |
| Explicit local with provider metadata | Preserve exact source, no clone even if missing | Missing local fails; no provider fallback |
| Source Local/local_pc/Worktree | Existing root/link or real worktree flow; real consumer evidence | No session/invalid environment fails or existing true prelaunch deferral |
| Branch Worktree, including inherited | Exact selected session/environment/owner and inventory; real MCP consumer | Other executors fail under existing branch contract |
| Task-Docker/SSH/Sprites/Kubernetes | Existing lifecycle/environment path untouched; focused existing stubs | No host inspection/fallback |
| Docker-hosted backend | Host paths refer to server container filesystem | Not task-Docker; no deployment/release acceptance inferred |

## Tests and causal RED/GREEN

Task 01 creates `TestManagedAttachmentPathAdmission` in repoclone,
`TestHostAttachmentRepositoryRecovery` in executor, and
`TestManagedSourceAttachmentConsumers` in backendapp. Task 02 adds
`TestManagedBranchAttachmentConsumers` in backendapp. These are now permanent regression tests. Test files remain below the
800-effective-line limit.

| Criteria | Required evidence |
| --- | --- |
| AC-001.2/.6, AC-008.1 | Five actual handler/environment routes: HTTP/MCP sources on Local and Worktree, MCP branch on Worktree; missing/empty current path, readable content, chosen branch/base and SQLite inventory |
| AC-008.2 | Exact current and existing legacy path/origin/control proof; foreign workspace and full-provider identity mismatch; occupied path, ancestor/.git/nested-ref redirection, alternates and wrong/empty returned clone; foreign file-content snapshots unchanged on refusal |
| AC-008.3 | Local present/missing with provider metadata; existing legacy stays at its saved location; absent legacy refusal without creating legacy/current destination; pathless nonlocal compatibility |
| AC-008.4 | Exact credential request scope, denied/unavailable auth/cloner/inspector, cancellation, missing/changed session/environment and fatal verified-path persistence fault with actual DB readback |
| AC-001.3/.4, AC-008.5 | Mixed batch with first clone success and later failure; old attachments/entries/inventory restored, shared verified clone/path retained; retry succeeds without duplicate rows/worktrees; existing foreign snapshots survive |
| AC-008.6 | Existing inherited/rebound session, prelaunch/unprovisioned and remote-boundary suite unchanged; host cloner spies prove zero calls for task remote executors |

Save RED logs before production edits. On base, saved-missing source/branch,
empty branch, foreign rejection and strict output/persistence assertions must
fail for their intended reasons; valid/empty-source/local controls must pass.
After implementation, run the same new tests GREEN plus the listed existing
boundary guards. If causal attribution is uncertain after later shared changes, removing only
the admission/recovery delta in an isolated patch control must restore the
relevant consumer RED, not an unrelated fixture failure. Do not assert that every new negative case must fail on base when an
existing guard already rejects it. Record actual commands and counts.

## End-to-end and platform acceptance

The handler-to-Git/SQLite tests are the end-to-end boundary for this backend-only
change. Do not add an artificial browser test or claim login/JSON-RPC stream,
real provider or agent process coverage. Existing web action, event and error
surfaces remain; desktop/phone share these backend outcomes. No rendered UI or
new locale keys are planned.

Use canonical temporary roots, local transport, real cloner and source contents.
Stub only external credential transport/runtime rescan or deliberate fault
boundaries. Test providers at the existing credential-request seam separately;
local Git alone does not prove provider auth. Reparse/link/permission tests need
capability probes and native execution where available. Native Windows/macOS and
isolated server-in-Docker acceptance are release limitations until actually run;
cross-compilation or remote executor stubs do not satisfy them. Do not launch a
live deployment or remove overlays. A failed unavailable primitive/platform case
must be reported accurately rather than weakening the assertion.

## Public docs

Task 02 updated the existing MCP attachment reference in
`docs/public/automation-and-mcp.md` with current-destination repair, missing
legacy refusal and server-host/task-executor distinction. The registered repository-create handler proves the documented
non-destructive separate registration flow.
Check related public repository docs and root README/screenshots for contradicting
claims; no screenshot or new public page is planned. Use existing error surfaces.
Confirm re-import guidance names a supported explicit registration flow; do not
recommend deleting live referenced repositories or clearing stored paths. If no
safe current registration flow exists, return that specific product gap rather
than add automatic migration/reset behavior.

## Work orders

- [x] [Task 01: Recover admitted current managed sources](task-01-current-managed-sources.md)
- [x] [Task 02: Materialize live managed branches and document partial recovery](task-02-live-managed-branches.md)

Task 02 depends on Task 01. Execute sequentially in this session after explicit
implementation authorization. Waves do not authorize agents or extra sessions.

## Verification results

Design-only validation on the uncommitted package:

- `python3 scripts/list-docs.py validate`: PASS, 396 decisions and 1687 specifications.
- `python3 scripts/lint-spec-files.test.py`: PASS, 36 linter tests.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- Repository `validateCoverage` preflight with the actual package contents and
  an explicitly prospective runtime-path trigger: `covered`, `errors: []`, both
  work orders and owning requirements/designs accepted. No runtime diff was
  created. Executed the saved `../4426-investigation/design-coverage.cjs` with
  `/home/jcfs/.local/share/mise/installs/node/24/bin/node` v24.21.0 because bare
  `node` was not on this shell's PATH.
- `git diff --check`: PASS; untracked Markdown whitespace/links and package
  inventory checked separately before the handoff.

Implementation evidence follows in the completed work orders. Design-only
checks above remain historical; final implementation and documentation checks
are recorded in Task 02. Native/platform and real-provider acceptance remain
unverified.

## Risks and remaining limits

- A legacy path that is absent must not accidentally enter the current clone
  branch because a generic helper returns a new path.
- Best-effort launch persistence must not leak into verified attachment success;
  conversely this repair must not change ordinary launch persistence policy.
- Wrong target selection can clone with another session's credentials or write
  into the wrong inherited inventory. Carry/revalidate the selected identities.
- Control-path validation and clone locks must share existing ownership without
  recursive locking or an unsupported claim about hostile concurrent host writers.
- Native path/reparse semantics and real-provider authentication need explicitly
  qualified evidence. A complete implementation does not close all of #4426.

## Original implementation checkpoint (superseded by PR remediation)

Task01 completed with causal registered-consumer RED and scoped GREEN/race/lint.
Task02 then completed with guarded MCP consumer RED/GREEN, shared identity
persistence fencing and final scoped checks. Its work order owns final exact
receipts. Status: `validation_ready`; no delivery authorization or issue closure
is implied. Base and HEAD remain `ce6ed5c971137a3c1d1360d75c111b51fde980d5`,
branch `feature/investigate-missing-vg2`. Final patch/hash/inventory are external
in `../4426-investigation/validation-ready-manifest.json`.

## Historical PR remediation checkpoint: persistence seam gap

Parent authorized scoped remediation implementation only after PR review of
head `df32a003d6336d78e523ce95c6a00b831bd7eb4b`. Original completed receipts
above are historical; both work orders are reopened for this bounded remediation.
No remediation commit or push is authorized before parent actual-diff validation.

Permanent compatibility regressions were captured RED before production edits:
`remediation-red.log`, registered HTTP/MCP Local/Worktree sources and guarded MCP
branches, plus path/control-root and alias identity tests. Space cases and
in-root-link/unpaired-scope refusal controls passed on the prior head. Scoped
legacy admission, parent aliases and newline cases failed behaviorally.

The uncommitted patch excludes scoped legacy candidates only in attachment
admission; bounds link rejection to configured base and descendants; compares
Git physical paths only after inspection; queries one path per supported
`rev-parse` invocation and strips only the record newline. The existing operation
guard additionally retains the resolved target identity of observed parent links.
A replaced alias target is refused even when the configured base was absent.
Global relocation, clone placement and ordinary local-path registration remain
unchanged. No schema, registry, journal or general lock/capability model added.

The second focused run (`remediation-green02.log`) passes all 11 new path/auth
cases and 30/35 compatibility consumer cases, without skips. Five missing-alias
consumer cases remain RED. They successfully clone and conditionally persist,
but `repoLocalPathUpdater.UpdateVerifiedRepositoryLocalPath` delegates to
`Service.UpdateRepository`, which always calls `canonicalRepositoryLocalPath`
for `LocalPath` and resolves its parent alias to a physical path. Worktree/branch
admission then rejects that physical registration path; Local consumers can
report success with the wrong stored path. Readback assertions independently
show the lexical saved path changed. Existing service tests explicitly require
canonicalization for ordinary explicit-local updates; do not weaken them.

Exact missing primitive: service-owned authenticated/version-fenced verified
managed-path persistence that preserves the already-admitted lexical path while
retaining repository event publication. The current public update seam cannot
carry that distinction. Return this gap before adding an internal method/typed
attestation or changing persistence policy. A narrow candidate could reuse the
existing exact updater and workspace authorization/event publication in a separate
internal verified-managed-path service entry; it needs parent boundary/diff review.
Persisting physical paths or changing global clone layout would contradict the
selected lexical ownership rule. No bypass was added.

Final scoped race/lint/docs receipts and complete uncommitted patch/hash are
external under `../4426-investigation/remediation-*`; the final report records
actual failures rather than claiming remediation GREEN. Status is blocked on
that precise helper gap, not ready for commit/push/merge. Review threads stay
unresolved until reviewed corrections. Codex's failed current-head review remains
explicit; bounded retry belongs after an eventual validated remediation push.

Decline aggregate automatic invalid-clone deletion without a demonstrated
producer and operation-owned deletion proof: see
[the requirement](../../specs/tasks/requirements/managed-source-attachment.md#req-tasks-attach-workspace-sources-008-recover-current-managed-sources)
and [shared clone lifetime](../../specs/tasks/system-design/current-managed-attachment-recovery.md#failure-retries-and-resource-lifetime).
Task-owned compensation grants no deletion authority over unverified shared
content. No `RemoveAll` or cleanup policy change was made. Partial #4426 only;
no issue closure, final public issue reply, native/provider/server-Docker or
release acceptance claim. Parent validation remains the next barrier.

Exact final scoped checks from repo root:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/repoclone ./internal/orchestrator/executor ./internal/backendapp -run '^(TestManagedAttachmentPathAdmission|TestManagedAttachmentPathCompatibility|TestManagedAttachmentAliasIdentityDuringAuthentication|TestManagedAttachmentCompatibilityConsumers|TestHostAttachmentRepositoryRecovery.*|TestEnsureRepositoryCloned_ReturnsOnlyLocalPath|TestManagedSourceAttachmentConsumers|TestManagedSourceAttachmentReimportRegistration|TestManagedBranchAttachmentConsumers|TestRepositoryHostClonerRequiresExactSessionScope|TestWorkspaceSourceMaterializer_(Remote.*|PrelaunchReturnsExplicitDeferredResult|UnprovisionedEnvironmentDefers)|TestWorkspaceSourceMaterializerKubernetesUsesRemoteMaterialization|TestWorkspaceSourceMaterializerRejectsUnknownExecutor|TestBranchMaterializer_(Inherited.*|LiveSessionBindingOverridesStaleOwnedEnvironment|RejectsSessionReboundBeforeCreate))$' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/backendapp -run '^TestManagedAttachmentCompatibilityConsumers$' -count=1 -v)
bash scripts/lint-go-changed apps/backend/internal/repoclone/managed_attachment_path.go apps/backend/internal/repoclone/managed_attachment_guard.go apps/backend/internal/repoclone/managed_attachment_compatibility_test.go apps/backend/internal/backendapp/managed_source_attachment_test.go apps/backend/internal/backendapp/managed_attachment_compatibility_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
/home/jcfs/.local/share/mise/installs/node/24/bin/node scripts/validate-public-docs.mjs
git diff --check
```

The first combined run included 35 remediation cases; its only five failures
were saved-missing alias cases. The targeted follow-up extended the matrix to
40 cases, adding empty saved paths under parent aliases: 30 pass, 10 fail, none
skipped. A lint-driven private helper extraction reduced inspector complexity
without changing the boundary. The final combined run covers that refactor and
the 40-case matrix. Actual counts and limits are recorded in the external report;
no failing run is represented as full GREEN.

Documentation assessment: the published repair/re-import flow is unchanged by
this internal ownership clarification. Owning requirement, design and ADR carry
the selected base/alias/path-byte boundary. Existing published pages still pass
the validator; no new public arguments, UI copy or screenshots are introduced.

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

Final combined race command from `apps/backend`:

```bash
go test -trimpath -race -tags fts5 ./internal/repoclone ./internal/orchestrator/executor ./internal/backendapp ./internal/task/service -run '^(TestVerifiedManagedRepositoryPath.*|TestService_UpdateRepositoryCanonicalizesExplicitLocalPath|TestManagedAttachmentPathAdmission|TestManagedAttachmentPathCompatibility|TestManagedAttachmentAliasIdentityDuringAuthentication|TestManagedAttachmentCompatibilityConsumers|TestHostAttachmentRepositoryRecovery.*|TestEnsureRepositoryCloned_ReturnsOnlyLocalPath|TestManagedSourceAttachmentConsumers|TestManagedSourceAttachmentReimportRegistration|TestManagedBranchAttachmentConsumers|TestRepositoryHostClonerRequiresExactSessionScope|TestWorkspaceSourceMaterializer_(Remote.*|PrelaunchReturnsExplicitDeferredResult|UnprovisionedEnvironmentDefers)|TestWorkspaceSourceMaterializerKubernetesUsesRemoteMaterialization|TestWorkspaceSourceMaterializerRejectsUnknownExecutor|TestBranchMaterializer_(Inherited.*|LiveSessionBindingOverridesStaleOwnedEnvironment|RejectsSessionReboundBeforeCreate))$' -count=1 -v
```
