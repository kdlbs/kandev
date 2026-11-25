---
created: 2026-11-05
status: implemented
requirements:
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
  - REQ-AGENTS-BUILTIN-MCP-004
  - REQ-AGENTS-MCP-PREP-005
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
  - ../../specs/agents/system-design/builtin-mcp-project-delivery.md
  - ../../specs/agents/system-design/agent-mcp-preparation.md
legacy_specs: []
---

# Fix plan: Pi built-in MCP delivery

## Overview

Restore Pi package-mode startup after
[issue #5090](https://github.com/kdlbs/kandev/issues/5090). The repair accepts
successful project-file delivery without relaxing admission for missing
packages. One work order delivers the complete lifecycle-to-adapter path and
its regression coverage. Implementation is complete.

Agents owns this repair because it owns provider delivery and package
admission. There is no rendered interface change. The same corrected backend
serves desktop and phone task starts.

## Diagnosis and evidence

Source baseline: `50b1583e88649bd3442416e18ac6674fd7fddc2f`.
At investigation time, remote main was
`c238042269041d2503ad488364144a7cb7fceeb4`. Its single intervening commit changes
capacity continuation and does not modify the Pi delivery or admission files.
Recheck main before implementation.

`requireBuiltinMCPDelivery` builds its delivered set only from ACP-selected
servers. `filterMcpServersWithDecisions` removes both injected transports when
Pi advertises neither HTTP nor SSE. The guard then rejects Core before any
provider session RPC. Project configuration is absent from that decision.

Lifecycle already calls `materializeRuntimeProjectMCPWithPreparation` before
process start for fresh launches and workspace promotion. Pi declares
`mcpconfig.PiStrategy`. ADR 0020 establishes the project path. This is a
delivery-accounting regression, independent of Kubernetes or model credentials.

A temporary Go overlay exercised the real ACP adapter with Core and Diagnostics
present in a generated Pi project file. New, load, resume, and reset all failed
with the issue's unsupported-server error. Provider counters stayed at zero.
The final reproduction checked both exact package URLs, `streamable-http`, and
`eager` in the file before each call. Project generation received one HTTP
entry per package, while ACP admission received HTTP and SSE alternatives.
The overlay and generated files were removed. No production or permanent test
files changed during diagnosis.

The issue contains no image attachments. The issue's Pi version, container
deployment, and installed-provider tool discovery were not independently
reproduced. The Go wire fixture proves the host admission fault.

## Requirements and assumptions

- The user requested investigation, conditional assignment, and a fix package.
- Restore the established Pi behavior from ADR 0020 and
  AC-AGENTS-MCP-PREP-005.1/.5.
- AC-AGENTS-BUILTIN-MCP-003.6 makes alternate-channel admission explicit.
- Preserve AC-AGENTS-BUILTIN-MCP-002.4, 003.1/.2/.4/.5, and 004.1.
- Existing owning requirements retain their draft status. This package does not
  promote historical draft documents or imply implementation approval.
- No material product choice remains unresolved. Routine API details can follow
  the design's typed record and existing configuration conventions.

## Scope

### In scope

- Evidence from successful Pi project materialization for the exact bound packages.
- One authenticated configure handoff to agentctl and immutable adapter evidence.
- Exact per-package admission across new, load, resume, and reset.
- Helper negotiation, missing evidence, partial delivery, and stale-context cases.
- Truthful delivery observations and existing user-file/ownership guarantees.

### Out of scope

- Provider capability overrides, agent-name bypasses, or changes to `pi-acp`.
- New package choices, grants, defaults, persistence schema, or runtime flags.
- New Cursor delivery support, UI markup, translations, or settings.
- Changes to Pi credentials, Kubernetes infrastructure, or global Pi configuration.
- Live selection reload for project-file providers or a browser E2E project.

## Technical approach

Follow the [project delivery design](../../specs/agents/system-design/builtin-mcp-project-delivery.md).
The execution-bound `ProjectMCPDelivery` record lives beside agentctl's shared
types. It is created only after the lifecycle writer succeeds, kept on the
execution, and sent through `configureAndStartAgentWithPreparedArgs`.

The configure request in `runtime/agentctl/client.go` and
`agentctl/server/api/server.go` carries it. Acceptance and validation run under
the process manager's existing configuration/start ordering. A cloned record is
forwarded through `adapter.Config.ToSharedConfig`. No per-executor launch field
is needed. The new helper capability is required before a dependent launch.
Pending evidence authorizes one process generation: adapter construction
consumes it, and the adapter keeps the immutable copy for same-process
load/resume/reset.

Local and worktree targets keep the backend file writer. Runtimes with a
separate target filesystem use the authenticated helper project-file contract:
`GET /api/v1/mcp/project` returns the helper's own workspace and loopback MCP
URL, `POST /api/v1/mcp/project/prepare` writes and reads back fingerprints
under `os.Root` containment, and cleanup releases the execution's claims. The
Kubernetes restart path stages the replacement client, prepares the project
file on that target, and configures the delivery record before start.

Change `acp/builtin_mcp.go` admission to consider ACP delivery or validated
project delivery for every injected package. Keep the ACP wire list capability
filtered. Update all three guard sites and the shared evidence decision so
reset inherits the same behavior. Missing or partial delivery still fails.

Preserve `manager_passthrough.go` file claims and the current Pi strategy.
Do not duplicate package resolution or infer connection health from file writes.

### Compatibility matrix

| Provider/path | Shape | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Pi ACP | HTTP=false, SSE=false; complete prepared project record | Admit all four session operations with no injected ACP entries | New adapter and process wire regressions |
| Pi ACP | Missing port, empty artifacts, failed write, or absent record | Reject missing selected delivery | Producer and negative admission tests |
| Pi ACP | Core record present, selected Diagnostics absent | Reject before provider RPC | Mixed-package regression |
| Pi ACP | Wrong task, instance, workspace, binding, or prior process | Reject invalid evidence without state replacement | Configure and generation tests |
| Ordinary ACP | HTTP, SSE, or neither; no project record | Preserve HTTP preference, SSE fallback, or explicit rejection | Existing ACP filtering and built-in tests |
| Cursor ACP | Existing project strategy and native preparation | Preserve existing ACP and deferred-update behavior | Existing Cursor materialization tests. No new project admission |
| Passthrough | Existing Pi or Cursor strategy | Preserve file generation and ownership | Existing strategy and file-claim tests |
| Remote executor | Helper-local workspace and endpoint; prepare succeeds with read-back evidence | Materialize through the authenticated helper operation, then carry the same validated record | Remote producer/configure fixture with a separate backend filesystem. Real cluster smoke remains conditional |
| Replacement process on one helper | Pending evidence already consumed by the previous adapter | Refuse until fresh preparation and configure | Stop/start and exit/start regressions without an intervening configure |
| Old helper | No new delivery capability | Fail before dependent process start with upgrade guidance | Client capability-negotiation regression |
| Retained agentctl | Existing bound process | Preserve adoption without a new session or manifest | Existing retention contract plus focused reuse assertions |

## Tests

All new names below are permanent regressions delivered by task 01.

| Acceptance | File and test |
| --- | --- |
| 003.6, 003.5 | `acp/builtin_mcp_test.go`: `TestBuiltinSessionRequestsAcceptPreparedProjectDelivery`; new/load/resume/reset table |
| 002.4, 003.2/.6 | Same file: `TestBuiltinSessionRequestsRejectPartialProjectDelivery`; Core present plus Diagnostics missing |
| 003.2/.5/.6 | `process/project_mcp_delivery_test.go`: `TestProjectMCPDeliveryRejectsMismatchedConfiguration`; absent, invalid, replaced and stale records |
| 003.5 | `runtime/agentctl/project_mcp_delivery_test.go`: `TestProjectMCPDeliveryRequiresHelperCapability` and configure round trip |
| 003.1/.2/.6, PREP-005.1/.4 | `lifecycle/manager_project_mcp_test.go`: `TestProjectMCPDeliveryPreparedOnlyAfterSuccessfulWrite`; launch, promotion, missing port and write failure |
| 004.1 | `acp/builtin_mcp_test.go`: `TestBuiltinProjectDeliveryEvidenceIsNotConnectionHealth`; per-package evidence and third-party filtering |
| PREP-005.1/.2/.4/.5 | Existing Pi strategy, project materialization, shared-file cleanup and successor tests |
| 003.6 remote delivery | `lifecycle/project_mcp_files_test.go`: executor-local target, exact package URLs on the target, helper support, Kubernetes restart configure, long workspace handoff |
| 003.5/.6 process generation | `process/project_mcp_delivery_test.go`: `TestProjectMCPDeliveryConsumedByProcessGeneration` |
| Workspace path bound | `agentctl/types/project_mcp_delivery_test.go` and `process/project_mcp_delivery_test.go`: nested path longer than 256 bytes |

## End-to-end evidence

Use backend integration rather than browser automation for this adapter defect.
Add `process/project_mcp_delivery_test.go::TestPiProjectMCPDeliveryACPFlow`.
Use the production configure and adapter factory, generated Pi configuration,
and an in-process ACP peer that advertises HTTP=false and SSE=false. Base the
pipe setup on `manager_permission_acp_test.go::newPermissionACPFlow`.

Cover initialization, new, load, resume, and reset with exact package identity
and empty built-in ACP wire arrays. Assert the missing-optional-package case
fails before a peer request. Pair this with the lifecycle producer test and
authenticated configure route test. This proves the complete internal handoff
without a model call. It does not certify the installed Pi extension.

When that runtime is available after implementation, an isolated Pi smoke can
demonstrate actual tool discovery. Record its exact version and executor.
Do not claim a real Pi or Kubernetes smoke from mock wire results.

## Work orders

- [x] [Task 01: Recognize prepared Pi project delivery](task-01-recognize-project-delivery.md)

Completed in the primary conversation. There was no delegation grant.

## Verification results

Investigation completed on 2026-11-05:

- Temporary `TestRepro5090PiProjectConfigDelivery`: failed in all four cases
  with the expected unsupported Core error before provider RPC.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp ./internal/agent/mcpconfig -run 'TestBuiltinSessionRequestsRejectUnsupportedInjection|TestPiStrategy_' -count=1)`:
  passed in both packages.
- Focused lifecycle Pi/Cursor materialization and unavailable-port tests:
  passed. This independently covers the existing production file writer.
- Temporary reproduction cleanup: completed.
- Specification catalog: passed for 433 decisions and 1924 specifications.
- Specification linter self-tests: passed, 36 tests.
- Full specification lint and PR-documentation coverage preflight: passed.
- Documentation whitespace and cross-reference checks: passed.
- Issue assignment: verified as `carlosflorencio` on 2026-11-05.

Implementation completed on 2026-11-05:

- Added the execution-bound `ProjectMCPDelivery` contract in `agentctl/types`,
  produced only after a successful project-file write, validated by the helper
  against its own instance, workspace, task, and bound package digest, and
  forwarded through the authenticated configure request into the ACP adapter.
- ACP admission now accepts a package delivered over the ACP wire list or by the
  validated project record; Core never authorizes a missing optional package.
  Project-delivered entries publish `project_config_materialized` delivery
  evidence without implying connection health.
- `project-mcp-delivery-v1` is advertised by agentctl and required before a
  dependent launch, so an older helper fails before configuration.
- Focused ACP, process, API, client, mcpconfig, profile, lifecycle, and
  `agentctl/types` suites: passed. Full
  `./internal/agentctl/types ./internal/agentctl/server/adapter ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/mcpconfig ./internal/mcp/profile ./internal/agent/runtime/lifecycle`
  run: passed.
- Remote coverage: the review-found standalone-only port lookup is replaced by
  a helper-local target; executor-local workspace and loopback endpoint are
  resolved from the helper, package URLs are written to the target and read
  back before evidence, Kubernetes restart configures the replacement record,
  and a separate backend filesystem is not written.
- Process-generation coverage: pending evidence is consumed by one adapter
  generation, same-process operations retain it, and restart without configure
  fails before provider dispatch.
- A nested workspace longer than 256 bytes is accepted by the record and
  handoff; task/instance identities keep their short bound.
- Targeted `-race` subset for the new delivery regressions: passed.
- `go build -trimpath -tags fts5 ./...` for the full backend: passed.
- `scripts/lint-go-changed` over every changed and new Go file: 0 issues.
- The real-Pi/Kubernetes smoke remains unclaimed; no installed Pi extension or
  cluster was exercised.

## Documentation preflight

Run from the repository root. The source path represents the planned production
change and prevents the docs-only exemption from hiding missing references.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/pi-builtin-mcp-delivery/';
const paths = [
  root + 'plan.md', root + 'task-01-recognize-project-delivery.md',
  'docs/specs/agents/requirements/builtin-mcp-management.md',
  'docs/specs/agents/requirements/agent-mcp-preparation.md',
  'docs/specs/agents/system-design/builtin-mcp-management.md',
  'docs/specs/agents/system-design/builtin-mcp-project-delivery.md',
  'docs/specs/agents/system-design/agent-mcp-preparation.md',
];
const result = validateCoverage({
  changedFiles: [
    {filename: 'apps/backend/internal/agentctl/server/adapter/transport/acp/builtin_mcp.go', status: 'modified'},
    {filename: root + 'task-01-recognize-project-delivery.md', status: 'added'},
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans
git status --short -- docs/plans/pi-builtin-mcp-delivery
```

## Documentation impact

Public references already describe Pi support and workspace MCP files in
`README.md`, `docs/public/agents-and-profiles.md`, and `docs/public/executors.md`.
This package proposes an internal regression repair, with no new operator
procedure or public option. Public docs remain unchanged during planning.

Companion packages are
[built-in management](../builtin-mcp-management/plan.md),
[Pi ownership](../pi-project-mcp-ownership/plan.md), and the preparation packages
linked by the owning requirement. Their existing scopes remain intact.
Record this repair's results here and link them from the two directly affected
companion plans. Do not replace their historical test counts.

## Risks

- Evidence created before a successful write can bypass the intended guard.
- Reusing a record across bindings, workspaces, or processes can misattribute tools.
- Configured file entries do not prove that a provider extension consumed them.
- Shared project-file ownership and deferred updates must keep their current rules.
- Older helpers need an explicit capability error before the new contract is used.
