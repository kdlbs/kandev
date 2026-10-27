---
id: "05-nono-provider-lifecycle"
title: "Implement Nono in its own repository"
status: done
wave: 5
depends_on:
  - "01-runtime-contract"
  - "04-policy-authority"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-001
  - REQ-AGENTS-SANDBOX-003
  - REQ-PLUGINS-SANDBOX-001
acceptance_criteria:
  - AC-AGENTS-SANDBOX-001.1
  - AC-AGENTS-SANDBOX-001.2
  - AC-AGENTS-SANDBOX-001.3
  - AC-AGENTS-SANDBOX-001.4
  - AC-AGENTS-SANDBOX-003.1
  - AC-AGENTS-SANDBOX-003.2
  - AC-AGENTS-SANDBOX-003.3
  - AC-AGENTS-SANDBOX-003.4
  - AC-AGENTS-SANDBOX-003.5
  - AC-AGENTS-SANDBOX-003.6
  - AC-PLUGINS-SANDBOX-001.1
  - AC-PLUGINS-SANDBOX-001.2
  - AC-PLUGINS-SANDBOX-001.3
  - AC-PLUGINS-SANDBOX-001.4
  - AC-PLUGINS-SANDBOX-001.5
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
  - ../../specs/plugins/system-design/sandbox-provider-capability.md
---

# Task 05: Implement Nono in its own repository

## Summary

Build the production Nono plugin in a dedicated repository and connect core-owned launch lifecycles.

## In scope

Use the official template, pinned runtime probe, policy compiler, foreground supervisor, host-owned artifacts, pipe/PTY handling, and retained snapshots.

Implement the generic status-normalization RPC using the protected runtime evidence verified in Task 01.
Keep Nono artifact parsing, CLI syntax, and override constraints in this plugin. Core receives typed evidence only.
If evidence cannot bind the attempt and policy, return unsupported or unknown rather than active.

## Out of scope

No remote execution, detached Nono ownership, global installation, marketplace publication, or macOS support claim without evidence.

## Acceptance

- The packaged plugin runs discovery, ACP, and terminal fixtures under the selected policy.
- Initial, restart, cancellation, timeout, failed startup, plugin loss, and cleanup preserve ownership and policy snapshots.
- Plugin source stays outside this repository and uses only public SDK contracts.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'SandboxLifecycle|SandboxTransport|GenericWrapper' -count=1)
(cd "${NONO_PLUGIN_DIR:?Set the dedicated plugin checkout}" && go test ./... -count=1)
(cd "${NONO_PLUGIN_DIR:?Set the dedicated plugin checkout}" && make validate package)
KANDEV_TEST_SANDBOX_PACKAGE="${NONO_PLUGIN_PACKAGE:?Set the dedicated packaged plugin}" go -C apps/backend test -race ./internal/plugins -run TestSandboxProviderExternalPackageRetainsOwnedRuntime -count=1
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `Dedicated kdlbs/kandev-plugin-nono repository: manifest.yaml, Go provider, conformance tests, Makefile validation/package targets`
- `apps/backend/internal/agent/hostutility/`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/agentctl/server/process/`
- `apps/backend/internal/agentctl/server/utility/`
- `apps/backend/internal/agent/runtime/lifecycle/sandbox_lifecycle_test.go (new)`

## Dependencies

01-runtime-contract, 04-policy-authority

## Risks

Repository creation/attachment occurs only after the design checkpoint. Nested PTYs require actual transport evidence.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).
- [Design](../../specs/plugins/system-design/sandbox-provider-capability.md).

## Results

### Remaining session-state cleanup slice

Core must inventory the frozen attempts for the exact task/session in bounded pages, validate captured state scopes, and reconcile terminated attempts before deleting an allocation. A live retained attempt, uncertain owner, open state lease, substituted marker, or foreign path must preserve the allocation and return a failure. Removal is idempotent and must preserve sibling state and task workspaces.

Wire cleanup after the session deletion path quiesces its execution and before it removes the session row. Wire durable task-deletion jobs after successful runtime stops, using their captured session inventory. Archive and retained-workspace jobs preserve session state. Task cleanup failures remain retryable through the existing job. Keep frozen attempt records as ownership evidence. When sandbox runtime cleanup is unavailable, captured sandbox attempts must block cleanup rather than silently skip it. The disabled/default path must not dispatch a provider or change files.

Implementation is authorized by the later user request. Runtime contract work has started while Task 04 remains open for final environment enforcement.

The dedicated local plugin repository was created from official template commit `805d2018773bf3c6f594f5c4f9e1b2d07e71bc48` and attached to the existing task. Its worktree is `../kandev-plugin-nono-main`, on branch `feature/integrate-nono-sandb-w4a`. No hosted repository or release has been created. The template SDK pin still needs to be replaced with the tested host implementation commit before delivery.

The host description contract now accepts a packaged runtime artifact with a relative path and SHA-256. Unsafe artifact declarations are rejected by focused tests. Callback endpoint/token references are rejected before provider dispatch and from supervisor arguments; the packaged provider contract suite passes under the race detector. Artifact materialization, actual Nono provider methods, lifecycle integration, and product acceptance remain pending.

The dedicated provider now implements all six SDK methods. Compilation removes implicit Nono filesystem grants, defaults to blocked networking, permits only explicit TCP proxy destinations, and filters approved environment keys. Approval normalization preserves the provider session and child identity. Audit parsing checks the pinned SHA-256 event chain and reports partial coverage. Status normalization explicitly remains unsupported because the registry lacks a policy digest.

`make validate verify-package` passed in the dedicated checkout with the pinned local release archive. The package contains the Linux provider, verified Nono runtime, lock metadata, and licenses. The template's unrelated UI and recipes are removed. The SDK commit marker deliberately blocks hosted CI until the host contract is committed and pinned. No repository publication occurred.

The host's `TestSandboxProviderExternalPackageRetainsOwnedRuntime` passed with the packaged provider and real Nono. It exercises install, SDK compilation, runtime digest verification, private artifact materialization, a live confined child, uninstall refusal for new calls, and completion of the existing owned child. This is a host/plugin boundary test, not ACP or native-terminal acceptance.

Core launch-envelope tests now pass for final environment filtering, immutable file checks, refusal after artifact replacement, and unchanged ordinary launches. Agentctl integration is in progress. Full lifecycle, callback authority, and MCP wiring remain open.

Agentctl utility probe, prompt, and secondary discovery now apply the captured sandbox outside the generic profile wrapper. Persistent and one-shot command preparation and native PTY startup apply the same envelope. Focused race tests pass for final environment filtering, artifact tampering, deferred-start validation, retained configuration, and refusal to clear sandbox selection by omission. These tests use generic supervisor fixtures, not the Nono product acceptance suite. Capability advertisement remains absent until the transport boundary is complete. Backend attempt preparation and lifecycle ownership remain pending.

The packaged provider now declares bounded identity/audit files below its private state root. Registry normalization returns supervisor, child, and provider session identity separately from confinement evidence. The real packaged-provider test passed a private registry read, supervisor match, child birth capture, and live kernel ancestry check. Registry data still cannot certify active protection. The focused source/identity contracts pass, and scoped Go lint reports zero issues. Approval request binding remains pending.

The provider-neutral runtime coordinator now prepares private launches from selected installations, persists attempts, binds registry identity to retained process ownership, and blocks new grants after provider disablement. Its real packaged-provider test passes. Process and PTY boundaries capture supervisor identity at spawn. Persistent sandbox supervisors use a private process session so cleanup can prove that descendants exited. Artifact cleanup refuses an owned process session that is still live.

The exact-port network assumption was corrected during callback isolation testing. The provider now requires core's optional `egress_proxy` capability for TCP grants. See the investigation record. Backend lifecycle wiring remains disabled until the full boundary passes.

### Component evidence, 2026-10-24

Native PTY launch helpers now bind retained process ownership before success. They stop the exact owned terminal process when binding fails. Restart paths prepare a new immutable attempt after the previous owned process session stops. These are generic transport fixtures; actual agent acceptance remains pending.

Explicit `agent_state` grants now allocate a separate child home with an ownership lock. The pinned Nono package test writes home/config/state/temporary files, retains session content across a new attempt, and still rejects private supervisor profile/proc reads. The same actual runtime test passes approval decisions and a task-scoped MCP fixture call. This is component restart evidence, not a completed ACP or terminal restart journey.

Policy capture now checks every relevant binding version and revocation at insertion. A unique active-execution index refuses simultaneous attempts for one execution. Focused SQLite and PostgreSQL checks pass. The TCP relay also rejects runtime control ports regardless of hostname aliases; backend wiring must supply those ports before activation.

Utility subprocesses now wait for an authenticated owner acknowledgement before ACP initialization. The host records dispatch intent, binds the supervisor, acknowledges it, and waits for cleanup. The secondary OpenCode model subprocess is explicitly refused under a selected sandbox until it owns a separate attempt. Prior generic launch-envelope tests do not establish that ownership.

Automatic replacement retains the previous policy revision and provider package. The latest session-attempt lookup and unique active-session index prevent a second execution from claiming a live session. SQLite and PostgreSQL tests pass for frozen restarts, session ownership, and idempotent migration of existing snapshots (`/tmp/kandev-nono-session-restart-postgres.log`).

A real packaged Nono test now persists the supervisor identity before registry evidence is available. Cleanup of a live or uncertain process records `recovery_required` and retains its files. A new coordinator refuses live adoption and removes private artifacts only after the captured process session terminates (`/tmp/kandev-nono-reconcile-green.log`). This does not establish full product recovery. Lifecycle shutdown tests also confirm that a sandbox execution stops before its backend-owned callbacks disappear (`/tmp/kandev-nono-shutdown-green.log`). Backend factory integration remains in progress.


## Detached descendant cleanup correction (2026-10-24)

A credential-free pinned Nono reproduction created a child with `setsid`, then ended the supervisor. The child remained alive in another process session. The earlier cancellation test covered its fixture only. It did not establish complete descendant cleanup.

`TestSandboxRuntimePreparesAndBindsPackagedProvider` reproduced the false stopped result with the packaged provider. Its new detached-child assertion failed before the fix and passed afterward. Core now owns a cgroup v2 leaf and uses `clone3` to start inside it. Recovery verifies the captured kernel identity, terminates remaining descendants, and removes the empty leaf before retiring the attempt. `TestProcessContainmentOwnsDetachedDescendant` also verifies refusal to signal a changed kernel identity and refusal to remove a populated group.

These Linux checks require a writable delegated cgroup v2 subtree and `clone3`. Set `KANDEV_TEST_SANDBOX_CGROUP=1` to require this prerequisite during verification. The implementation does not modify the main service's controllers or move its processes. Missing support refuses sandbox launches. Generic command wrapping remains independent.

The startup receipt and transport integration checks remain in progress. Full product discovery, ACP, native terminal, restart, UI, and acceptance checks are still pending.


### Utility startup completion receipts

Focused tests now cover authenticated, bounded receipt retrieval, refusal of stale or oversized receipts, and no redirect following. A confirmed failed spawn releases the immutable attempt without a fabricated process owner. Utility cleanup consumes the receipt after dispatch failure and retains the failure result. Completed utility requests terminate their captured detached descendants before issuing a stopped receipt. Selected sandbox errors omit private receipt bodies, including malformed responses.

Persistent ACP and native terminal failed-spawn receipts now have focused component coverage. Both paths reject reuse of a completed attempt. The authenticated persistent receipt route and local native receipt are consumed by lifecycle cleanup. Fast supervisor exit before ownership binding now has passing integration coverage for persistent ACP and native terminals. Captured completion receipts retire stopped attempts after the original request deadline, while preserving the startup failure. These results do not enable launch admission or satisfy full product acceptance.


### Discovery selection wiring

The backend utility factory now creates a separate execution-scoped attempt for each selected profile probe. Its packaged-provider test verifies captured working directories, distinct identities, cleanup, and rejection of stale or missing selection fingerprints. Admission clears prior caller data and resolves current server policy. The discovery fingerprint includes policy revisions, binding versions, and the live provider installation. Provider loss blocks cache access.

The host utility suite passes under the race detector after adding regressions for policy changes during capability and model-option probes. The host rechecks policy before publishing either result. Main runtime activation and full product acceptance remain pending.


Session-state deletion is implemented (2026-10-26). Core inventories attempt history in 64-record pages, validates frozen task/session and child-state scope, and removes only captured allocations after stop proof. The owner marker, canonical root, and exclusive lease guard removal. Direct session deletion performs cleanup after runtime stop and before row deletion. Durable task cleanup uses captured session inventory and retries failures. Failed runtime stops, archive jobs, and retained-workspace jobs preserve state. Disabled/unavailable runtime cleanup blocks captured history without provider dispatch. SQLite/PostgreSQL inventory race tests, common filesystem tests, session/task deletion race tests, and the actual packaged Nono approval/MCP/restart-state/cleanup test pass. Full production-agent acceptance and explicit restart review remain open.


Manual restart review is implemented (2026-10-26). The new path compiles an explicitly reviewed current revision through the packaged provider. It captures operator authority and exact selection evidence before dispatch and rechecks mutable bindings at insert. The review is consumed before automatic replacement. Frozen restart remains unchanged. The complete policy suite passes with SQLite/PostgreSQL; focused runtime, lifecycle, resume, and backend wiring race tests pass. Desktop/phone review checks pass. Full managed-agent acceptance remains open.


## Completed acceptance (2026-10-27)

The production provider is in the dedicated repository, with draft PR https://github.com/kdlbs/kandev-plugin-nono/pull/2 at `8f68c6a10add119a1187c7428c5fad51aa6ca25a`. Its SDK source is host commit `beca1157060f61a7a4dafbec0e759e71a469de98`. `make validate` and `make verify-package` passed with the pinned runtime. Actual managed Pi discovery, ACP and native MCP/file work, both PTY sizes, cancellation including a detached child, same-session restart, sibling isolation, and owned state deletion passed. The repeatable transport runner also passed. No release or marketplace publication occurred. Registry identity still provides no accepted current-policy attestation. The status-normalization fallback is unsupported/unknown, as required by Task 01.
