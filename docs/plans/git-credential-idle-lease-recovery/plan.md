---
created: 2026-11-18
status: complete
requirements:
  - REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001
system_design:
  - ../../specs/platform/system-design/git-credential-lease-reissue.md
legacy_specs: []
---

# Managed Git credential recovery after idle expiry

## Overview

Make managed Git requests recover automatically after idle expiry with the workspace credentials already configured. Platform owns the lease contract for Git, the `gh` shim, and managed executors. Use an installation-derived signer by default and keep the existing one-shot reissue. [Lease recovery design](../../specs/platform/system-design/git-credential-lease-reissue.md) · [Signer decision](../../decisions/2026-11-18-default-git-credential-reissue-signer.md)

## Evidence

- The reporter verified that host `gh` retrieves its token and a newer task session succeeds through the managed shim. The original shim receives `401 git_credential_lease_invalid` from the broker. Its process lacks a reissue capability.
- The original process was idle overnight for more than the default 12-hour lease duration. Idle expiry followed by pruning explains the response. A restart or revocation could also remove the record, so the response alone does not identify the event that removed it.
- A disposable broker test advances time by 13 hours and sweeps the lease. The original lease fails with `ErrLeaseInvalid`. A new launch lease succeeds in the lease-only case. A launch that received a valid capability recovers by reissuing and redeeming a replacement.
- The helper already supports the same one-shot reissue for Git and `gh`. Configured keys are loaded at backend startup. The executor issues a lease without a capability when the signer is unavailable. [Backend signer wiring](../../../apps/backend/internal/backendapp/git_credentials.go) · [Executor issuance](../../../apps/backend/internal/orchestrator/executor/executor_credentials.go) · [Helper recovery](../../../apps/backend/cmd/agentctl/github_credential.go)
- A disposable prototype derived a signer using Kandev's existing `MasterKeyProvider`. A new broker reopened the same installation key and reissued after the idle gap. A broker from a different data directory rejected the capability. The prototype exercised derivation and broker behavior. Production startup wiring is the work below.
- New workspaces start with **Inherit executor Git credentials** when the settings write succeeds. Prior settings and the managed compatibility fallback can still put a workspace in managed mode. Keep both modes available. [New workspace Git access defaults](../../decisions/2026-08-02-new-workspace-github-access-defaults.md)

## Scope

- Derive the default signer from the existing installation key. Preserve the configured-key override and backward compatibility for its capabilities.
- Add an optional backend off switch for capability issuance and exchange. Managed sessions use automatic recovery by default.
- Prove recovery from an idle, pruned lease and after broker restart using the default signer.
- Give **Restart session** guidance when a lease-only execution receives an invalid or expired lease response. Cover a retained process that still carries launch-time credentials after backend restart.
- Update the executor, Docker, configuration, and recovery documentation listed in [Task 01](task-01-managed-lease-recovery.md#public-documentation). Cover the workspace default, automatic recovery, process restart, and key restore and rotation.

### Exclusions

- GitHub login changes and automatic agent restarts.
- Lease or capability lifetime, concurrent replacement semantics, and authorization contract changes.
- Provider error classification, Chat indicators, and MCP policy changes.

## Technical approach

Select the signer in backend composition while `provideRepositories` has the loaded master key provider. A true `githubCredentialBroker.disableReissue` selects no signer. Otherwise, a nonblank override uses its existing key interpretation and a blank override selects the HKDF-derived default. Pass `Repositories.gitCredentialReissueSigner` to `newGitCredentialBroker` and record the startup selection once. The existing lease issue, retry, scope, and wire contracts handle the resulting capabilities. [Signer selection](../../specs/platform/system-design/git-credential-lease-reissue.md#signer-selection)

In the shared helper, preserve the broker error and add conditional **Restart session** guidance for HTTP 401 lease-invalid and lease-expired codes when the capability is missing. An agent retained across upgrade can still have its launch-time lease-only environment. A successful **Restart session** stops that execution and launches a new process in the same session. An already stopped session can use **Resume**. Subsequent eligible requests recover within the capability lifetime.

## Verification

- `.9` advances a fake clock past the default lease duration and reissues after pruning. Use the original capability and live scope checks.
- `.10` exercises both command entry points over the broker wire. Check invalid and expired codes under both prefixes. With no capability, those two codes produce guidance and send no reissue request. Revoked, unrelated, and non-401 errors retain their diagnostic. Verify **Restart session** replaces a retained lease-only process and gives the fresh execution a usable capability.
- `.11` asserts the startup Info event and its `source` field for installation, configured, and disabled selections. Verify that sensitive values are absent.
- `.12` exercises startup selection with no configured override and a real temporary installation master key. Pin the derived encoding with a test vector. Issue through the broker selected by startup wiring. Reopen the same key and prove recovery with an empty lease map. Use the existing launch environment and helper HTTP tests to prove delivery and retry.
- `.13` exercises the legacy configured-key constructor and a different installation key. An override verifies old capabilities. An independent default signer rejects a foreign capability. The broker still denies forged, expired, and mismatched scopes.
- `.14` checks that the optional setting defaults to false, accepts its YAML and environment forms, and wins over either signer source. Verify lease-only launch and redemption, and denial of existing capabilities through both route aliases.

Existing focused tests for shim and helper reissue and broker recovery after restart passed during diagnosis. The implementation checks are in the work order.

## Work orders

- [x] [Task 01 Enable automatic managed lease recovery](task-01-managed-lease-recovery.md)

## Results

Managed Git launches now receive a reissue capability signed by an installation-derived key unless reissue is disabled or an explicit key is configured. Lease-only retained processes receive **Restart session** guidance, and the restart flow is covered through backend adoption and managed executor recovery. Public executor, Docker, configuration, operations, Git, and integration documentation now describe the recovery behavior and key lifecycle.

Review remediation added an archived-task guard, same-broker idle-lease recovery coverage, retained-process identity checks, and race-safe helper coverage for revoked and non-401 lease errors. Focused backend tests, Go lint for changed files, the backend build, the Linux retained-process Playwright E2E, frontend lint and sleep-ratchet checks, public documentation validation, specification validation, link checks, and `git diff --check` passed. Detailed commands and results are recorded in [Task 01](task-01-managed-lease-recovery.md#results).

## Risks

- Restore the matching `master.key` with installation data to preserve default capabilities and decrypt stored secrets. If the secrets loader successfully replaces a missing, unreadable, or wrong-length file, startup still logs `source: installation`. Existing default capabilities and stored secrets require the old key.
- Deployments with a nonblank override continue to use it when reissue is enabled. Changing the active signing key prevents the new signer from verifying prior capabilities. Disabling and then re-enabling with the same key can restore access for still-valid capabilities.
- The default puts a 30-day bearer into eligible managed executor environments. A leaked capability can issue replacement leases after the original lease's 12-hour idle expiry while the scope stays authorized. Use the backend off switch when an installation requires lease-only operation.
- Long-lived processes can eventually need **Restart session** when their capability expires. Existing concurrent reissue can still race within a session.
