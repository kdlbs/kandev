---
id: "11-macos-support"
title: "Support sandbox policies and Nono on macOS"
status: done
wave: 11
depends_on:
  - "05-nono-provider-lifecycle"
  - "06-approval-channel"
  - "07-mcp-connectivity"
  - "10-execution-surfaces"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-011
  - REQ-PLUGINS-SANDBOX-002
acceptance_criteria:
  - AC-AGENTS-SANDBOX-011.1
  - AC-AGENTS-SANDBOX-011.2
  - AC-AGENTS-SANDBOX-011.3
  - AC-AGENTS-SANDBOX-011.4
  - AC-AGENTS-SANDBOX-011.5
  - AC-AGENTS-SANDBOX-011.6
  - AC-AGENTS-SANDBOX-011.7
  - AC-AGENTS-SANDBOX-011.8
  - AC-AGENTS-SANDBOX-011.9
  - AC-AGENTS-SANDBOX-011.10
  - AC-AGENTS-SANDBOX-011.11
  - AC-AGENTS-SANDBOX-011.12
  - AC-AGENTS-SANDBOX-011.13
  - AC-AGENTS-SANDBOX-011.14
  - AC-PLUGINS-SANDBOX-002.1
  - AC-PLUGINS-SANDBOX-002.2
  - AC-PLUGINS-SANDBOX-002.3
  - AC-PLUGINS-SANDBOX-002.4
  - AC-PLUGINS-SANDBOX-002.5
system_design:
  - ../../specs/agents/system-design/sandbox-macos-support.md
  - ../../specs/agents/system-design/sandbox-policies.md
  - ../../specs/plugins/system-design/sandbox-provider-capability.md
---

# Task 14: Support sandbox policies and Nono on macOS

## Summary

Admit darwin Local and Worktree executors, add the darwin profile compiler and process ownership, and prove confinement with canaries on a real macOS host.

## In scope

- Startup Seatbelt probe and capability advertisement in `agentctl/server/api/identity.go`; admission in `prepareExecution` (`backendapp/sandbox_runtime.go`) before `protectedPorts`, previous-attempt reconciliation, Prepare and attempt capture (not task-creation admission, not `sandboxLaunchAdmission`), the native-terminal refusal in `startSandboxInteractive` (`lifecycle/sandbox_native.go`), an explicit darwin refusal in `prepareUtility` (`backendapp/sandbox_utility_runtime.go`), the provider `host_unsupported` sentinel in `plugins/sandbox_provider_dispatch.go`, and the Settings availability check in `sandbox_availability.go` (one blocker, no reason field). Both codes are typed errors surfaced through the generic launch failure and logs; no Tasks envelope, cause code, or web mapping change.
- Darwin ownership by agentctl alone, from existing mechanisms: `Setsid` wrapper, owner route and `BindOwner`/`VerifyLive` darwin branches keyed by `ownership_scope`, agentctl's existing stop and exit waiter (no second reaper), backend read-only process-group scan (5 s bound) replacing the containment calls in `Close`/`ReconcileAttempt` (`BindOwner`/`validateOwner` adds the `GroupID == SessionID == PID` check; `RetireDescendants` and the agentctl start receipt are unchanged); TMPDIR root reserved before compile through a reserve/materialize split of `MaterializeRuntime` (Linux calls both after compile); `ownership_scope` in the private snapshot. The backend never signals darwin processes. Restarted-agentctl stop, a backend darwin containment object, a stop route, utility/discovery support on darwin (refused in this version), a persisted launch-cause carrier and web mapping for the codes, an earlier native-terminal refusal, per-reason host-unsupported granularity, an approval refusal reason string, codex-app-server on darwin, and darwin start-receipt handling are out of scope (follow-up card `0995aaaf-0b84-49b2-9eec-74e279ced0cf`).
- Darwin environment in `common/sandbox/`: per-attempt TMPDIR (fresh root per attempt, no reuse), `GIT_CONFIG_NOSYSTEM`, darwin `lockAgentState`, transport refusal.
- Provider contract: `host_unsupported` constructor and matcher in `pkg/pluginsdk` and its mapping in `internal/plugins`; no proto field change.
- `agent/sandbox`: approval denial and report shape keyed by `ownership_scope`, status, and the two `sandbox_*` failure classes.
- Web: Execution details copy and states in `apps/web` with component tests, plus all seven locale catalogs.
- Plugin repository branch of `nova28/kandev-plugin-nono` based on the head of kdlbs/kandev-plugin-nono#2: darwin/arm64 description with packaged runtime artifact, darwin profile compiler, darwin-gated conformance test. Both PRs link each other.
- Acceptance record in `acceptance.md` with real darwin results, the exact option that removes Nono's default temp groups, and `active_status_evidence`.

## Out of scope

Remote and SSH executors, macOS approvals, keychain-resident agent logins and their failure class, a CONNECT-relay egress canary, a profile-apply failure class, darwin/amd64, native terminal under a sandbox on darwin, and any Linux behaviour change. Derived common-git grants for linked worktrees, host git identity copy, the Seatbelt log reader, and the three items above are cut to follow-up card 1ae5b8fa-67dd-4ea9-bae5-f6199e7ae9be.

## Acceptance

- Linux tests pass unmodified; darwin advertises only after a passing probe.
- Every canary in AC-AGENTS-SANDBOX-011.11 reports the expected result with a succeeding control on a real Mac, with no file names or contents printed.
- No approval listener exists on darwin and a darwin attempt has no report and Execution details states that filesystem denials are unavailable on macOS; the ownership canary shows the child in the wrapper's group.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/common/sandbox ./internal/backendapp -run 'Sandbox' -count=1)
(cd apps/backend && GOOS=linux GOARCH=amd64 go vet ./internal/agentctl/... ./internal/common/sandbox/...)
(cd apps/backend && go test -trimpath ./internal/plugins -run 'SandboxDarwinConformance' -count=1)
(cd "${NONO_PLUGIN_DIR:?Set the dedicated plugin checkout}" && go test ./... -count=1 && make validate package)
```

The darwin conformance run must execute on macOS and its output is pasted into the acceptance record.

## Files

- `apps/backend/internal/agentctl/server/api/identity.go`
- `apps/backend/internal/common/processcontainment/` (darwin identity)
- `apps/backend/internal/common/sandbox/` (darwin environment, agent_state lock)
- `apps/backend/internal/agent/sandbox/` (approvals, reports, status, failure classes)
- `apps/backend/internal/backendapp/sandbox_runtime.go`, `sandbox_availability.go`
- `apps/backend/pkg/pluginsdk`, `apps/backend/internal/plugins/`
- `apps/backend/internal/plugins/sandbox_darwin_conformance_test.go (new)`
- `apps/web` Execution details components and `apps/web/src/locales/*`
- `docs/plans/nono-sandbox-integration/acceptance.md`

## Risks

Seatbelt behavior on newer macOS releases, processes that escape the group via setsid, a process left running after an agentctl restart (stays `recovery_required`, unmanaged), a reused group ID making the scan fail closed, and linked-worktree commits failing on darwin until the follow-up lands.
