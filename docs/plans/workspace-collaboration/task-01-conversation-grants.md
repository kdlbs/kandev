---
id: "01-conversation-grants"
title: "Enforce explicit workspace grants for conversation callers"
status: in-progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKSPACE-COLLABORATION-001
acceptance_criteria:
  - AC-TASKS-WORKSPACE-COLLABORATION-001.1
  - AC-TASKS-WORKSPACE-COLLABORATION-001.2
  - AC-TASKS-WORKSPACE-COLLABORATION-001.3
  - AC-TASKS-WORKSPACE-COLLABORATION-001.4
  - AC-TASKS-WORKSPACE-COLLABORATION-001.5
  - AC-TASKS-WORKSPACE-COLLABORATION-001.6
  - AC-TASKS-WORKSPACE-COLLABORATION-001.7
system_design:
  - ../../specs/tasks/system-design/workspace-collaboration.md
legacy_specs: []
---

# Task 01: Conversation grants

Follow the [plan](plan.md) and paired design. Implement typed grant persistence,
human/manage APIs, transport-independent task-service callback enforcement,
verified-principal composition and append-only reports. Reuse the native runtime
flag registry and all-off profiles. Do not change installed permissions, models,
history, operating services, unrelated worktrees or Git references. Use temporary
proofs outside test collection; retain exact commands/results here and remove
temporary sources after sufficient verification.

## Task 01 checkpoint and follow-on scope

This is an API/task-domain foundation, not completion of T3 or a guarantee for
the main MCP plugin-tool flow. The typed grant store, human/manage GET/PUT API,
verified-conversation task-domain boundary and independent status-report HTTP/WS
primitive are implemented. Grant keys retain the logical task/current source
workspace across runtime-session replacement. Invalid/null permission fields and
request-supplied caller identity are rejected. Report-only access cannot become a
generic prompt, control operation or self-grant edit.
The typed table is created after core task migrations and has cascading foreign
keys to its caller task and both workspaces. Actual task/workspace deletion and
existing reset paths therefore remove grants atomically; archive/off retain them.

The shipped toggle is `features.workspaceCollaboration` /
`KANDEV_FEATURES_WORKSPACE_COLLABORATION`, off in prod/dev/e2e and restart-required.
Its metadata explicitly states that plugin Host coverage is incomplete. Off
preserves native APIs and saved grants; enabled identity/store errors deny the
covered call. No operating toggle, grant, profile, model order, history or service
was changed during implementation.

At this Task 01 checkpoint, the [transport gap](../../specs/tasks/system-design/workspace-collaboration.md#transport-coverage-and-original-remaining-work)
was still open: server-originated agent invocation context reached the plugin,
but its later long-lived Host RPC had no restored conversation/owner principal.
Task 02 closes that gap with a bounded per-invocation origin bridge through the
existing SDK/broker path. Payload task IDs are not used as identity, and no
global plugin principal was added.

Later changes in this PR add the desktop/mobile grant-editor entry points and
`message_task_kandev` report delivery. Profile inheritance, delegated/taskless
callers, and aggregate navigation remain follow-up scope.

## Executed verification (2026-10-15)

Working directory for Go commands: `apps/backend`. `OVERLAY` replaces only the
disposable machine-local overlay filename. These results remain valid without
recreating temporary tests; recheck only changed paths or new failure evidence.

```text
go test -trimpath ./internal/task/service -run '^Test(WorkspaceScoping|SessionScoping|AuthorizeWorkflowAccess|AuthorizeSessionAccess|AuthorizeTaskSessionAccess)' -count=1 -timeout 90s
ok github.com/kandev/kandev/internal/task/service 0.735s

go run -trimpath ./cmd/sqlguard ./internal
exit 0

go test -trimpath -race ./internal/persistence/storeconformance -run '^TestStoreConformance$/^sqlite3$/^task$' -count=1 -timeout 90s
ok github.com/kandev/kandev/internal/persistence/storeconformance 13.026s

go test -trimpath -overlay OVERLAY ./internal/task/repository/sqlite -run '^TestCollaborationProbe' -count=1 -timeout 90s -v
--- PASS: TestCollaborationProbeSQLiteFreshReplay (1.29s)
--- SKIP: TestCollaborationProbePostgresFreshReplay (0.00s)
ok github.com/kandev/kandev/internal/task/repository/sqlite 1.464s

go test -trimpath -overlay OVERLAY ./internal/backendapp -run '^TestCollaborationProbe' -count=1 -timeout 90s -v
--- PASS: TestCollaborationProbeIndependentActionsAndIdentity (0.43s)
ok github.com/kandev/kandev/internal/backendapp 0.732s

go test -trimpath -overlay OVERLAY ./internal/task/repository/sqlite -run '^TestCollaborationLifecycleProbe$' -count=1 -timeout 90s -v
--- PASS: TestCollaborationLifecycleProbe/SQLite (1.30s)
--- SKIP: TestCollaborationLifecycleProbe/Postgres/{target-direct,source-direct,target-cascade,source-cascade,caller-task,rollback}
ok github.com/kandev/kandev/internal/task/repository/sqlite 1.358s
```

The SQLite replay probe initialized a fresh schema, stored a grant and an existing
task, initialized the same database again, read both back, revoked flags while
retaining the grant row, and rejected source-workspace inheritance. PostgreSQL
was skipped because `KANDEV_TEST_POSTGRES_DSN` was not set; no PostgreSQL behavior
pass is claimed.

The final lifecycle correction added three `ON DELETE CASCADE` foreign keys and
moved initialization after core migrations. Its new disposable proof stored two
independent grants, replayed initialization without loss, then exercised actual
`DeleteWorkspace` and `DeleteWorkspaceCascade` for source and target, `DeleteTask`
for the caller, and a rollback from injected workspace cleanup failure. All six
SQLite cases passed, with unrelated grants retained. PostgreSQL cases skipped for
the same missing DSN. SQL guard passed again after this schema-only correction;
the earlier permission/transport probes were not recreated or rerun. The existing
task-owner race command above was rerun for the changed schema and passed in
`6.598s`. The prior full-suite Windows storage failure remains unchanged.

The backend probe used real SQLite, `ScopePrincipal` over stored task/session/
workspace rows, the real WS dispatcher/report API and native task services. It
checked independent grants, a completed status-report turn with WFI retained,
generic-message agent-author spoof rejection, human-only edits, task metadata
not changing typed grants, a real workspace control mutation, session replacement,
foreign caller-session rejection, native RBAC denial, HTTP caller-field/null/type
rejection, same-workspace/browser/internal preservation, explicit off returning
to legacy behavior and enabled grant-store failure denying access.

The initial broader Go invocation used
`-run 'Test(.*Workspace.*Access|.*WorkspaceScope|.*TaskScope|.*Permission|.*Registry|.*Feature|.*Profile)'`
with runtimeflags/config/profiles/task-service/handlers/backendapp packages.
Runtime flags (`0.095s`), config (`0.146s`), profiles (`0.096s`), handlers
(`1.209s`) and backendapp (`5.013s`) passed. Task-service matches also selected
unchanged Windows-sensitive orphan-reap EPERM and POSIX Kubernetes-path fixtures;
that combined invocation failed and is not reported as an overall pass. The
focused native authorization command above supplies the relevant result.

The mandatory full `go test -trimpath -race ./internal/persistence/storeconformance
-count=1 -timeout 180s` ran. A final-state JSON capture identified only
`TestStoreConformance/sqlite3/storage/{crud,timestamp,conflict,transaction}` and
`TestPreviousStableUpgrade/sqlite3/v0.93.0` failing with
`storage_temp_artifacts: temporary artifact path must be an absolute path`.
The unchanged `owner_actions.go` fixture uses `/tmp/` paths on Windows. Integration
independently observed the same failure in the unchanged capacity worktree.
The full suite remains non-passing; it was not weakened or repeatedly rerun.
The task-owner race result and grant-specific replay/CRUD proof are recorded
separately above. The previous-tag SQL fixtures and their existing checksums were
retained; the manifest only records that the new required table is absent from
that historical version.

Frontend verification after `pnpm install --frozen-lockfile` in `apps`:

```text
pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts
Test Files 1 passed; Tests 7 passed; Duration 252ms

pnpm run typecheck
exit 0 with process-only NODE_OPTIONS=--max-old-space-size=8192
```

The default 4 GiB typecheck first exhausted the Node heap. The temporary per-command
8 GiB allowance completed; no repository or persistent memory setting changed.
Documentation catalog validation reported 383 decisions/1617 specifications;
specification and architecture lint passed. Existing `pr-docs.cjs.validateCoverage`
read the actual 23 changed paths and four linked documents and returned
`status: covered`, `errors: []`, including the final lifecycle documentation.
`git diff --check` passed. Before the final schema-only lifecycle correction, the
changed-package Go lint completed naturally with exit 0 and `0 issues.`:

```text
golangci-lint run ./internal/backendapp ./internal/common/config ./internal/runtimeflags ./internal/task/models ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/persistence/requiredstores --new-from-rev=HEAD --timeout=5m --concurrency=2
0 issues.
```

That command used process-only `GOMEMLIMIT=3GiB`, `GOMAXPROCS=2` and `GOFLAGS=-p=2`.
Integration retains the normal staged hooks, including the final schema delta,
and publication. The shared lint slot
was returned to the next worker after the result was collected; no parallel-lock
bypass was used. An earlier lint attempt was interrupted during coordination and
is not counted as a pass.

## Review correction: workspace and workflow action boundaries

Independent review found that individual workspace reads bypassed the new read
grant, workspace management used only native permissions, and workflow update,
delete and reorder reused read authorization. A verified conversation could
therefore read without a grant or mutate with read-only access. The correction
adds the missing read/control intersections while retaining the existing native
checks and explicit-off behavior. Read helpers continue to require only read;
control does not imply read. Both ordinary and version-fenced reorder use control.

Existing HTTP/WS workflow policy checks also performed ordinary reads before
mutations. With independent control-only access, those reads would either block
create/reorder or skip the GitHub/Improve Kandev immutable checks on update/delete.
They now request action-authorized boolean policy decisions from the task service,
without exposing workspace/workflow metadata or granting reads. Existing native
policy outcomes and transport error handling remain in place. This correction
does not extend the documented plugin Host coverage or complete remaining T3 work.

Disposable probes reused the real backend SQLite service fixture and stored
`ScopePrincipal` identities, outside test collection. The before-source overlay
reproduced the missing-grant/read-only/store-error/synthetic authorization failures.
After correction, `TestCollaborationGrantSeamDelta` passed all 13 subcases in
`3.355s`: no grant, all-false, read-only, control-only, native denial, grant-store
error, synthetic conversation, same-workspace, browser, internal, explicit-off
with retained grants, and both workspace deletion denial contexts. These results
were retained rather than repeating unaffected cases.

```text
go test -trimpath -overlay OVERLAY ./internal/backendapp -run '^TestCollaborationGrantSeamDelta$' -count=1 -timeout 90s -v
ok github.com/kandev/kandev/internal/backendapp 3.355s

go test -trimpath ./internal/task/handlers -run '^TestHTTP(CreateWorkflow|WorkflowMutationsRejectReadOnlyWorkflows|ReorderWorkflows)' -count=1 -timeout 90s
ok github.com/kandev/kandev/internal/task/handlers 0.262s
```

The new HTTP/WS policy probe exercised manual mutations and immutable GitHub/
Improve Kandev targets under both on and off. Its first run passed 38 of 40
subcases and exposed read authorization in both reorder methods; only the two
affected on/manual/reorder cases were rerun after that correction, and both
passed. A separate read-only reorder check verified denial in both ordinary and
version-fenced paths. Its first retention assertion incorrectly assumed the
repository preserves a requested initial sort order; the fixture was corrected
to compare the actual stored order before and after denial. No production change
was needed for that fixture correction.

```text
go test -trimpath -overlay OVERLAY ./internal/backendapp -run '^TestCollaborationMutationPolicyDelta$/^(on=true|reorder-read-only)$/^(http|ws)$/^manual$/^reorder$' -count=1 -timeout 90s -v
PASS: on=true/http/manual/reorder; on=true/ws/manual/reorder
exit 1 only for the temporary initial-sort-order assertion described above

go test -trimpath -overlay OVERLAY ./internal/backendapp -run '^TestCollaborationMutationPolicyDelta$/^reorder-read-only$' -count=1 -timeout 90s -v
ok github.com/kandev/kandev/internal/backendapp 5.346s

golangci-lint run --allow-serial-runners ./internal/task/service ./internal/task/handlers --new-from-rev=HEAD --timeout=5m --concurrency=2
exit 0; 0 issues.
```

The shared lint slot was returned after natural completion. Temporary overlay
sources and before-source copies were removed after verification; no permanent
tests, runtime settings, operating data or Git references changed. Integration
owns the normal commit hooks and subsequent transport work.
