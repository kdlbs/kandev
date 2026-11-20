---
created: 2026-11-16
status: in_progress
requirements:
  - REQ-AGENTS-MCP-INSTANCE-001
  - REQ-AGENTS-MCP-INSTANCE-002
  - REQ-AGENTS-MCP-INSTANCE-003
  - REQ-AGENTS-MCP-INSTANCE-004
system_design:
  - ../../specs/agents/system-design/builtin-mcp-instance-scoped-routes.md
legacy_specs: []
---

# Implementation plan: Instance-scoped built-in MCP routes

## Overview

Make built-in MCP URLs identify the owning execution's helper as well as its port.
Add helper routes and negotiation first. Then carry the effective mode through
lifecycle clients, migrate delivery, and activate new instances. Finish with
existing browser flows, provider smoke checks, and documentation.

Implementation is authorized from the reviewed design package. Requirements
and design remain draft until implementation and required checks are complete.
Execution is sequential and tracked by the work-order statuses below.

## Inputs and decisions

- [Requirements](../../specs/agents/requirements/builtin-mcp-instance-scoped-routes.md):
  `REQ-AGENTS-MCP-INSTANCE-001` through `004`.
- [System design](../../specs/agents/system-design/builtin-mcp-instance-scoped-routes.md).
- [Accepted routing decision](../../decisions/2026-11-16-instance-scoped-builtin-mcp-routes.md).
- [Existing per-CLI strategy decision](../../decisions/0014-passthrough-mcp-injection-strategies.md).
- [Package management](../../specs/agents/system-design/builtin-mcp-management.md)
  and [project delivery](../../specs/agents/system-design/builtin-mcp-project-delivery.md).
- Source handoff: `/root/.kandev/tasks/https-github-com-kdl_82q3ynij/instance-scoped-mcp-url-brief.md`.
  Its decisions and incident context are preserved in the linked durable records.

Baseline: `b4c6a197129b5b4a5e99c3e1dab132ada9fb433f`.
Read-only PR metadata on 2026-11-16 showed
[#5294](https://github.com/kdlbs/kandev/pull/5294) open at
`e9e13e3aca498b5f8521ba100522e7809d686f78`, on `JnManso`'s fork branch.
The credential implementation is absent from this checkout. This package starts
fresh and credits João Manso's investigation. It does not authorize a fork push,
PR closure, or PR rework. Recheck the PR relationship before authorized delivery.

The `agents` system owns this package because helper identity and native MCP
delivery define the durable contract. Existing package-management, Pi-delivery,
and runtime-recovery plans are reference inputs. Their completed evidence must
not be relabelled as evidence for the new routing behavior.

## Scope

### In scope

- Scoped core/package HTTP and SSE routes, including SSE message advertisements.
- Exact own-route auth exemptions and 404 rejection before task effects.
- One shared native URL builder, identity validation, and bounded rejection logs.
- Per-instance create negotiation and authenticated recovery of effective mode.
- Every ACP, passthrough, project-file, remote, and exact-approval consumer.
- Accepted helper identity and effective mode, including staged delivery.
- Targeted Go tests/lint, clarification and TUI browser checks, and documentation.

### Out of scope

- Bearer credentials, Authorization changes, header probes, and caller authentication.
- Runtime feature flags, profiles, operator controls, and an environment kill switch.
- Tool/package grants, workflow policy, question availability, or new rendered UI.
- External MCP and third-party endpoint rewrites.
- Deliberate copying of another workspace's full MCP URL.
- New helper-ID allocation, identity metadata, recovery ownership rules, or session-association fields.
- Persistent Kandev subtasks, delegation, staging, commits, pushes, or PR mutation.

No UI preview is needed: this package changes transport routing and preserves
the existing desktop and phone interactions.

## Technical approach

### Compatible helper expansion

Add `ScopedMCPRoutes` to create/config/response/live-info carriers.
An omitted create field keeps legacy routes. A current backend eventually sends
true, but only the effective echoed value controls its URL generation.

Create `internal/common/mcpurl` for prefix, identity validation, and native URL
construction. Move the existing host/IPv6 handling into this neutral owner.
All consumers must use it. Keep the prefix `/i/` in one constant.

`api.Server` groups native routes by the effective prefix. A narrow middleware
returns 404 for invalid native route targets before control authentication.
Exempt only exact own-instance native paths. Configure both transport handlers
and all rebuilt package views with the prefix. Test the actual SSE endpoint
event and POST, rather than only Gin registration.

Migrate helper-owned ACP injection, approval comparisons, passthrough snapshots,
and project target/read-back URLs in the same order. An explicit scoped create
must work end to end before any backend default changes.

### Lifecycle mode propagation

Carry effective mode from accepted create responses or authenticated
`InstanceInfo` into the selected client. Preserve the field in SSH/Sprites direct
HTTP paths and synthesized get-or-create responses. Do not add it to
`AgentSessionAssociation` or reconcile duplicate mode sources.

Most retained paths already read `InstanceInfo`. Kubernetes reattachment checks
health only, so read its mode through the existing temporary control forward.
A mode-read failure must not become a recreation trigger.

Use the accepted helper ID, which is unique per execution. Same-execution helper
recreation can reuse it without weakening cross-task isolation. Preserve existing
ID allocation, request carriers, metadata, owner matching, and durable delivery.

### Delivery migration and activation

Pass the selected client into lifecycle URL generation. For staged project
delivery, this is `placement.client`. For SSH, retain the snapshot's client
lease through preparation. The execution's published client can still be old.

Update the strict `builtinPassthroughEndpoint` parser together with the builders.
Validate incoming core paths against the selected client's canonical path.
Do not parse an ID from a URL and treat it as trusted helper identity.

Migrate both legacy and bound Cursor composition, package expansion, native
fingerprints, Codex argv, Pi files, and Claude/OpenCode artifacts. Once these
consumers work in both modes, set every backend create builder to request true.
Never retry a rejected scoped URL at a bare path.

### Delivery compatibility and evidence

Every row retains its existing provider/transport support limit. A URL change
does not create a new supported provider or injection channel.

| Provider or channel | Transport and identity | Intended result and evidence | Unsupported fallback |
| --- | --- | --- | --- |
| ACP new/load/resume/reset | Negotiated HTTP/SSE, legacy or bound packages | Helper ID/mode in all injected URLs; helper/API tests | Existing transport/package admission error |
| Claude passthrough | HTTP URL in temporary `mcpServers` file | Scoped file and existing third-party entries; delivery table tests | No strategy remains unsupported |
| OpenCode passthrough | HTTP remote entry and selected config file | Same canonical URL and preserved env; delivery table tests | Existing strategy error |
| Codex passthrough | HTTP URL in repeated `-c` arguments | Exact scoped URL and quoting; delivery tests and installed smoke | No new provider-header dependency |
| Codex app-server | Existing structured delivery, when enabled | Scoped injected URLs; existing native transport plus smoke evidence | Existing experimental/protocol admission |
| Pi | Prepared `.pi/mcp.json` project channel | Selected client's endpoint, matching helper read-back; local/remote tests | Existing project-delivery capability error |
| Cursor legacy and bound | `.cursor/mcp.json`, existing approval fingerprints | Canonical scoped URL, ownership preserved; Go tests and manual approval check | Existing collision/auth/admission behavior |
| SSH TUI | Helper snapshot and prepared file over the same lease | Full response mode/ID and correct current-session context; Go and containers E2E | Old helper mode stays bare; other capability gates remain |
| Docker, Remote Docker, Kubernetes, Sprites, plugin remote | Existing executor-local host and accepted helper identity | Creation/recovery table tests for each carrier | Absent mode means bare only after a successful old-peer response |
| Retained helper | Authenticated `InstanceInfo` | Stable URL without ACP initialization; recovery tests | Read/decode errors block dependent delivery |

Both skew directions are mandatory. Old backend/new helper means absent opt-in
and bare service. New backend/old helper means absent response mode and bare
delivery. Package capability errors are not route-mode fallback.

## Tests

All test names prefixed `TestBuiltinMCPInstance` below are planned additions.
Use new sibling test files if an existing Go file is near its lint limit.
Each work order starts with behavioral RED evidence and records its final GREEN.

Test paths use these prefixes below `apps/backend`: `api`, `config`, `instance`,
and `process` mean `internal/agentctl/server/<name>`. `agentctl` and `lifecycle`
mean `internal/agent/runtime/<name>`. `common` and `mcp` mean `internal/<name>`.

| Acceptance criteria | Exact planned evidence |
| --- | --- |
| 001.1, 001.2 | `api/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstanceRoutes`, core/package and method table, with control auth on/off and a dispatch counter |
| 001.3 | `instance/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstancePortReuse`, real listener stop/create on the same address |
| 001.4 | `mcp/server/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstanceSSE`, core/package endpoint event and valid message POST |
| 001.5 | `common/mcpurl/builtin_test.go`: `TestBuiltinMCPInstanceURL`; `instance/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstanceAdmission`, no listener/lease leak |
| 002.1 | `config/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstanceInjection`; `api/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstanceSessionRequests`, all four request families and both catalog modes |
| 002.2 | `lifecycle/builtin_mcp_instance_delivery_test.go`: `TestBuiltinMCPInstanceDelivery`, each provider artifact in both modes; helper snapshot/target tests in `process/builtin_mcp_instance_test.go` |
| 002.3 | `process/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstanceApproval`, actual allow-once/allow-always selection and managed-policy acceptance, with forged/bare/wrong-ID negatives |
| 002.4, 004.5 | Extend `mcp/server/builtin_packages_test.go` and `process/builtin_mcp_permission_test.go` through scoped fixtures; retain grants, unknown-package 404, and foreign-session rejection |
| 002.5 | Browser flows in the next section; mock URL-consumption tests in `cmd/mock-agent/mcp_client_test.go` and `tui_test.go` |
| 003.1 | `lifecycle/builtin_mcp_instance_delivery_test.go`: `TestBuiltinMCPInstanceDefaultCreate`, table over every fresh and recreation request builder |
| 003.2, 003.3 | `agentctl/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstanceNegotiation`; helper API tests with omitted/false/true create fields |
| 003.4 | `agentctl/builtin_mcp_instance_test.go`: `TestBuiltinMCPInstanceInvalidEvidence`; `lifecycle/builtin_mcp_instance_delivery_test.go`: `TestBuiltinMCPInstanceEndpointValidation`, no bare fallback or writes |
| 003.5, 004.1 | `lifecycle/builtin_mcp_instance_recovery_test.go`: `TestBuiltinMCPInstanceRecovery`, retained bare/scoped modes, no ACP creation/config rewrite or recreation on failed mode reads |
| 004.2 | `lifecycle/builtin_mcp_instance_delivery_test.go`: `TestBuiltinMCPInstanceDelivery`, accepted helper ID despite a different host ID and valid same-execution reuse; `TestBuiltinMCPInstancePortReuse` covers a different task's ID |
| 004.3 | `lifecycle/builtin_mcp_instance_delivery_test.go`: `TestBuiltinMCPInstanceStagedDelivery`, separate staged/published clients with equal and different IDs, both mode directions, rollback on prepare failure |
| 004.4 | `api/builtin_mcp_instance_test.go`: health/control regression cases; retain external routing assertions in `mcp/server/external_integration_test.go` |

Criterion suffixes in this table refer to `AC-AGENTS-MCP-INSTANCE-<suffix>`.
The common URL table includes IPv4, named hosts, IPv6 zones, wildcard listeners,
empty legacy host, invalid identities, and all transport/package variants.
Use existing backend fixture ownership and cleanup rules. Rate-limit tests use
controlled time or tokens, not a new sleep-driven test.

## E2E tests

The mock clients already consume their delivered URLs. ACP uses SSE, while the
TUI context command uses streamable HTTP. Keep these mechanisms intact.
TUI agents disable interactive questions. Use the supported TUI context call
without changing question eligibility. The managed runner selects the e2e
profile for these checks.

| Flow | Acceptance | Playwright file and project |
| --- | --- | --- |
| New ACP question, select answer, originating agent resumes | 002.5, 004.5 | Existing `tests/chat/clarification.spec.ts`, `chromium` |
| Same question interaction on a phone | 002.5, 004.5 | Existing `tests/chat/mobile-clarification.spec.ts`, `mobile-chrome` |
| Local TUI reads actual generated URL and reports the current session | 002.2, 002.5 | Add a case to existing `tests/terminal/terminal-agent.spec.ts`, `chromium` |
| SSH TUI current-session MCP call (recommended) | 002.2, 004.1 | Existing `tests/ssh/passthrough.spec.ts`, `containers`, focused existing MCP case |

`TestBuiltinMCPInstanceRoutes` and `TestBuiltinMCPInstancePortReuse` own wrong-port
404 and no-dispatch evidence. No additional browser isolation spec is needed.
Existing browser flows verify valid delivery, question/answer behavior, and TUI
context through real helpers. Expose no control tokens or full URLs in attachments.

Run these commands from the repository root, sequentially. Install dependencies
once if the worktree lacks them. The managed runner rebuilds required artifacts.

```bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/clarification.spec.ts tests/terminal/terminal-agent.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-clarification.spec.ts
```

Recommended when a Docker daemon and the documented E2E images are available:

```bash
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --shards 1 --project containers tests/ssh/passthrough.spec.ts -- --grep 'remote MCP uses the active task and session'
```

Missing infrastructure is a recorded skip and does
not block implementation acceptance. It does not establish SSH E2E coverage.
Do not substitute a host-only result or disable freshness checks.

## Work orders

- [x] [Task 01: Add compatible scoped helper routes](task-01-scoped-helper-routes.md)
- [x] [Task 02: Migrate delivery and activate scoped instances](task-02-delivery-and-default-activation.md)
- [~] [Task 03: Verify delivery and reconcile documentation](task-03-end-to-end-and-docs.md)

Dependency order: **01 -> 02 -> 03**. All are sequential.
Task 01 adds compatible helper support and client negotiation fields. Task 02
keeps lifecycle mode propagation, writer migration, and default activation together
so no intermediate implementation emits unusable endpoints.

## Verification results

Task 01 helper implementation and Task 02 lifecycle delivery/default
activation are complete. Backend build, focused lifecycle race tests, helper
package tests, changed-Go lint, mock-agent tests, and targeted browser E2E have
passed. Task 03 docs/spec/catalog/coverage checks have also passed. Installed
Claude, Codex, and Cursor runtime/approval checks remain pending, so Task 03
and the overall plan stay in progress.

Implementation and verification results on 2026-11-16:

- `make -C apps/backend build`: passed.
- Task 02 focused lifecycle and API race tests, including retained plugin
  reopen, Kubernetes container restart/absence, and stale-route warning burst:
  passed. Helper/process package tests passed; changed-Go lint reported 0 issues.
- Review follow-up lifecycle/process race tests passed for plugin recovery
  identity, SSH resume control-port discovery, Kubernetes retained metadata,
  and helper-scoped profile preparation. The updated Kubernetes fixture confirms
  the retained route-mode read does not start a nonce handshake.
- Complete lifecycle and helper-process race suites passed after updating
  retained-instance fixtures, and `make -C apps/backend build` passed across
  backend, agentctl, mock-agent, and debug binaries.
- Task 03 mock-agent test passed; Chromium clarification/TUI suite: 36 passed;
  mobile clarification suite: 16 passed; SSH TUI MCP case: 1 passed with a
  real SSH fixture through the `containers` project in host-runner mode.
- Specification catalog/lint, public-doc tests/validation, repository coverage,
  gofmt inventory, and `git diff --check`: passed.
- Installed provider checks are not complete. Claude Code 2.1.322 and Codex
  CLI 0.167.0 are available but their question/approval flows were not run;
  Cursor is not installed. Keep Task 03 and requirements/design in draft until
  those release gates are completed.

Planning validation on 2026-11-16:

- `python3 scripts/list-docs.py validate`: passed, 453 decisions and 2016 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository `validateCoverage` preflight from Task 03: passed, all three work
  orders and their requirement/design references accepted. A source sentinel
  activated validation without changing a production file.
- Local Markdown link and new-file whitespace audit: passed for all seven files.
- `git diff --check -- docs/plans/builtin-mcp-instance-scoped-routes docs/specs docs/decisions`:
  passed. Untracked work orders were also inventoried explicitly.

Each order records its own exact commands and outcomes. Run changed-Go lint
before staging through the command in each relevant order. Keep result counts
and any environmental limitations with the actual run.

Manual release gates remain pending until observed:

1. Claude ACP: ask a question, answer in the owning task, and observe the same
   originating agent receive the answer with native auto-approval working.
2. Codex app-server, when available: repeat the question flow. Codex passthrough
   can instead prove current-task context because TUI questions remain disabled.
   Record which mode and installed version supplied the evidence.
3. Cursor: verify both scoped Core and an enabled package through project
   configuration, then the question/answer flow where its adapter supports it.
   Inspect its native approval flow after URL/fingerprint refresh.

Record provider versions, mode, helper mode/identity evidence, task association,
and outcomes without secrets. Do not mark a missing provider check passed.
If the implementation is complete before these release checks, report that
distinction explicitly and leave the unmet work-order acceptance pending.

## Documentation impact

Implementation must update `docs/public/automation-and-mcp.md`'s agent-attached
route paragraph. It currently promises bare paths. Its primary content type is
reference. Keep the external MCP routes unchanged and explain legacy negotiation.

Update the transport subsection in the active package system design to reference
the scoped prefix contract. Update `apps/backend/internal/agentctl/AGENTS.md`
and `MCP_DELIVERY.md` with effective mode, helper-local URLs, and replacement
delivery. Root engineering guidance does not need another routing description.
Promote these draft specs only after their implementation and required checks
match. Do not edit catalog rows or rewrite historical completed plan results.

## Risks

- One missed native URL builder or the bare-only parser can disable a provider's tools.
- Old-helper fallback can be mistaken for confirmed protection. Keep mode explicit.
- Copying only port/ID from a synthesized response can lose the effective mode.
- Reading the published old client during staged placement can emit the wrong scope.
- A failed mode lookup must not change existing recovery or resource-ownership decisions.
- Gin auth order and SSE advertisements can produce different failures from route tests alone.
- Cursor fingerprints require an installed-provider check despite existing port churn.
- The PR/fork delivery choice remains a later maintainer decision. It does not block this package.
