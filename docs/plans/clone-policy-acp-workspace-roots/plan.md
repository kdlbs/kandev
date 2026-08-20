---
specs:
  - docs/specs/platform/task-git-metadata-permissions.md
  - docs/specs/tasks/attach-workspace-sources.md
created: 2026-08-20
status: in_progress
---

# Implementation Plan: Clone Policy Attestation and ACP Workspace Roots

Clone-based Docker, SSH, and Sprites launches will carry a path-free requirement for mutable Git policy, attest their own canonical checkout before an agent can start, and preserve existing cleanup and recovery semantics. In parallel with that security boundary, lifecycle and agentctl will negotiate ACP `additionalDirectories` from the authoritative executor-side source roots instead of inferring a broader directory.

## Backend

### Clone policy requirement and executor attestation

Extend `ExecutorCreateRequest` and launch construction in `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` with an intent-only mutable-repository requirement for clone executors. Keep host `GitMetadataProjections` empty for those executors. Add executor-owned canonical checkout resolution to `git_metadata_remote.go`, use it in Docker, SSH, and Sprites only after their prepare/clone steps, and install the filesystem profile before the agent child starts. Revalidate before mutable launch; sanitize all invalid/unsupported errors and keep host paths out of executor env and logs.

### ACP additional-directory negotiation

Add the ordered executor-side roots to the lifecycle → agentctl instance and WebSocket session-new request path. Teach `adapter/transport/acp.Adapter` to retain the initialized `SessionCapabilities.AdditionalDirectories` advertisement and include only normalized, absolute, deduplicated roots other than `cwd` in ACP `NewSessionRequest.AdditionalDirectories`. Unsupported providers receive none. Remote materialization must derive roots from canonical task workspace destinations, not durable host records; a request that would require unsupported additional roots must fail explicitly rather than widening to the task parent.

### Lifecycle and cleanup integration

Cover initial launch, reset/restart, attachment rebind/rollback, and terminal cleanup. Do not modify `internal/agent/usage` or its Provider Usage dependents. Preserve Remote Docker's existing fail-closed unsupported status.

## Tests

- **Clone policy:** `apps/backend/internal/agent/runtime/lifecycle/git_metadata_remote_test.go`, Docker/SSH/Sprites lifecycle tests, and launch metadata tests. Exercise `git add`, `git commit`, ref/reflog locks, common metadata denial, forged checkout rejection, restart/rebind rollback, cleanup, and host-path sanitization.
- **ACP protocol:** `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_test.go` and client/API tests. Assert advertised providers receive canonical ordered additional directories; unsupported providers receive none and do not receive a parent or host path.
- **Launch/session path:** lifecycle session and workspace-materialization tests. Assert multi-repository canonical destinations reach agentctl before `session/new`, unsupported-provider behavior is explicit, and source rollback preserves the old session state.
- **Race/lint:** run focused `go test -race` packages and `make -C apps/backend lint` after affected package tests pass.

## E2E Tests

- **Scenario:** a Local Docker task with a mutable cloned repository starts, commits normally, and cannot access host Git metadata. **File:** existing container executor Playwright coverage under `apps/web/e2e/tests/`. **Verify:** successful task launch and commit-visible state with no host-path error.
- **Scenario:** SSH and Sprites clone-based launches gate policy before agent startup. **File:** existing SSH container coverage plus Sprites E2E target. **Verify:** run only when Docker/SSH target or Sprites credentials are available; otherwise record the exact environment gate and retain deterministic Go regressions.

## Verification Results

Focused and package-level Go tests, race detection, backend lint, and public-docs validation pass. Container E2E was attempted with the managed runner on 2026-08-20, but its disposable `node:22-slim` image cannot run `apt-get update`: Debian metadata responses are intercepted as `NOSPLIT` and report that the network requires authentication. Docker daemon access itself is present. The SSH fixture uses the same container build and is therefore gated by the same failure. No Sprites provider credential is present in this environment. These are environment gates, not passing E2E evidence.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-clone-policy-attestation](task-01-clone-policy-attestation.md)

Wave 2:

- [x] [task-02-acp-additional-directories](task-02-acp-additional-directories.md)

Wave 3:

- [ ] [task-03-regression-and-documentation](task-03-regression-and-documentation.md)

## Open Questions

None. The accepted ADR fixes the policy boundary: no host projection crosses into clone runtimes, and no ACP root is sent without provider capability negotiation.
