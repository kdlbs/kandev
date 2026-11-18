---
id: "01-managed-lease-recovery"
title: "Enable automatic managed lease recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001
acceptance_criteria:
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.9
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.10
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.11
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.12
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.13
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.14
system_design:
  - ../../specs/platform/system-design/git-credential-lease-reissue.md
---

# Task 01 Enable automatic managed lease recovery

## Summary

Use Kandev's existing installation key to enable managed Git reissue without an operator-supplied key. Preserve the configured override, add an optional backend off switch, and tell users how to restart a lease-only process.

## Scope

- Add `GitHubCredentialBrokerConfig.DisableReissue` as a startup-only boolean (`mapstructure:"disableReissue"`, `json:"-"`). Register it and the existing sensitive `reissueSigningKey` in the config catalog. Bind each canonical environment variable once and preserve the override's existing environment-over-YAML precedence.
- Select the signer in `provideRepositories`. Derive the default with `crypto/hkdf` and the salt, info, output length, and encoding in the [design](../../specs/platform/system-design/git-credential-lease-reissue.md#signer-selection). Pass `repos.gitCredentialReissueSigner` through `initGitCredentialAndRemoteWiring` into `newGitCredentialBroker`. Use the existing constructor unchanged for configured keys. Emit the startup Info event with its `source` field.
- In `resolveWithReissue`, add conditional **Restart session** guidance for lease-invalid and lease-expired HTTP 401 responses with an empty capability. Retain the typed broker error and its existing text. Cover Git and `gh` using synthetic broker responses.
- Prove default issuance and recovery after idle expiry and broker restart. Exercise configured-key compatibility, independent installation keys, live reissue guards, and lease-only operation with reissue disabled. Complete the public documentation updates below.

### Exclusions

- Updates to the credential-scoping rules or capability lifetime.
- Agent-provider sign-in changes, Chat presentation, and automated restarts.

## Acceptance

- Both default and configured signers issue capabilities for eligible managed launches. With the default signer, the helper redeems a replacement after idle pruning or restart. A foreign or expired capability is denied. An unchanged configured key opens capabilities issued before upgrade.
- Git and `gh` preserve the typed broker error. With no capability, HTTP 401 responses with `git_credential_lease_invalid`, `git_credential_lease_expired`, `github_credential_lease_invalid`, or `github_credential_lease_expired` include conditional **Restart session** guidance and send no reissue request. Revoked and unrelated failures retain their existing handling.
- After successful **Restart session** of an adopted lease-only execution, the same session has a fresh process, lease, and usable capability with reissue enabled.
- `git_credential_reissue.signer_selected` appears once at Info level with `source` set to `installation`, `configured`, or `disabled`. Startup logs and command errors omit key, lease, capability, and credential values.
- With `disableReissue` enabled, the active broker issues redeemable leases without capabilities and denies exchanges of capabilities issued before disabling. Check a deployment with no override and one with a configured signing key.

## Verification

Start with a failing assertion at backend composition. An empty override on the investigation baseline yields a lease-only broker even with a loaded installation key. The helper regression also fails because the existing code returns the original broker error when the capability is missing.

```sh
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp ./internal/common/config ./internal/gitcredentials ./internal/github ./internal/orchestrator/executor ./cmd/agentctl -run 'Test(GitCredentialBrokerDefaultReissue|GitCredentialBrokerConfiguredReissue|GitCredentialBrokerDisabledReissue|GitCredentialReissueStartupDiagnostic|GitCredentialReissueDiagnosticSources|GitCredentialReissueInstallationKeyDerivation|GitHubCredentialBrokerConfigEnvironmentBinding|GitHubCredentialBrokerDisableReissueConfig|BrokerReissueRecoversPrunedIdleLease|BrokerReissueRejectsForgedExpiredAndMismatchedCapabilities|CredentialBrokerReissueRejectsCapabilityWhenDisabledOverHTTP|ConfigureGitHubCredentialBrokerPublishesReissueCapability|ConfigureGitHubCredentialBrokerFallsBackToLeaseOnlyWhenReissueUnavailable|GitHubCredentialReissueRejectsArchivedTaskWithLiveSession|GitHubCredentialHelperReportsLeaseRelaunch|GitHubCLIShimReportsLeaseRelaunch|GitHubCredentialHelperKeepsOtherLeaseErrorsUnchanged|GitHubCLIShimReissuesInvalidLease)$' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Pin the default key derivation with a fixed synthetic key and expected
encoding. Use both `/api/v1/git/credentials/reissue` and
`/api/v1/github/credentials/reissue` in the disabled broker HTTP test.

For retained-process recovery, follow the Worktree setup in
[`agent-survival-restart.spec.ts`](../../../apps/web/e2e/tests/session/agent-survival-restart.spec.ts)
with a test managed Git credential. Start an agent with reissue disabled and
local agent survival enabled. Restart the backend with reissue enabled and the
same installation key. Verify adoption preserves the process identity and its
lease-only behavior. Then use **Restart session** from the session menu. Verify
the same session and workspace have a fresh execution, and verify that the new
helper can exchange its capability without exposing bearer values. The menu
action is also covered by
[`session-tab-restart.spec.ts`](../../../apps/web/e2e/tests/session/session-tab-restart.spec.ts).

## Public documentation

- In [`docs/public/executors.md`](../../public/executors.md#managed-github-credentials), correct the new workspace default to **Inherit executor Git credentials**. Describe managed mode's 12-hour idle lease, automatic recovery, 30-day capability, and **Restart session** or **Resume** for a process that needs a fresh launch. Keep the compatibility fallback and remote broker reachability requirements. Update [Sprites.dev](../../public/executors.md#spritesdev), which currently instructs operators to configure a stable signing key to recover after restart. In [SSH](../../public/executors.md#ssh), distinguish resume that reuses a live process from **Restart session**.
- In [`docs/docker.md`](../../docker.md#persistent-agent-and-selectable-gh-cli-auth), replace the instruction to set `KANDEV_GITHUB_CREDENTIAL_BROKER_REISSUE_SIGNING_KEY` for recovery. Keep the public URL and proxy route requirements. Cover default `master.key` persistence on the data volume, optional override rotation, and the off switch.
- In [`docs/public/configuration.md`](../../public/configuration.md), add `githubCredentialBroker.reissueSigningKey` and `githubCredentialBroker.disableReissue` beside `publicBaseUrl` in the startup settings table. Give the canonical environment names, defaults, precedence, and backend restart requirement. Mark the override as sensitive. Describe selecting a different signing key to invalidate old capabilities and explain the consequences of changing or losing `master.key` in the database backup section.
- In [`docs/public/operations.md`](../../public/operations.md#state-and-storage), update the `master.key` table entry and restore guidance for both encrypted secrets and outstanding default capabilities. Explain that replacement can occur on startup if Kandev cannot load the existing key.
- Link the credential note in [`docs/public/git-operations.md`](../../public/git-operations.md) and **Choose task Git credentials** in [`docs/public/integrations.md`](../../public/integrations.md#choose-task-git-credentials) to the shared managed Git recovery instructions. [`docs/public/authentication.md`](../../public/authentication.md) covers Kandev sign-in and contains no managed Git lease instructions to revise.

## Files likely touched

- `apps/backend/internal/backendapp/storage.go`, `types.go`, `git_credentials.go`, `services.go`, and `services_github_broker_test.go`.
- `apps/backend/internal/common/config/config.go`, `catalog.go`, and `config_test.go`.
- `apps/backend/cmd/agentctl/github_credential.go`, `github_credential_test.go`, and `github_cli_shim_test.go`.
- `apps/backend/internal/gitcredentials/reissue_test.go` and `apps/backend/internal/github/credential_broker_reissue_wire_test.go`.
- The guides in [Public documentation](#public-documentation).

## Dependencies

This work uses the shipped lease reissue broker, helper, and wire contract.

## Risks

An invalid-lease response can follow expiry, restart, or revocation. A replacement requires an authorized session and working credential source. An agent may collapse helper stderr into its own error. Newly issued default capabilities extend the usable lifetime of copied bearer data in managed executors. Re-enabling reissue with the same key can make still-valid capabilities usable again.

## Parallelism

`sequential`

## Results

The four review findings are fixed. The shared scope authorizer now denies capabilities for archived tasks even while a session remains nonterminal. The idle-expiry regression prunes and recovers the lease in the same broker. The retained-process E2E resolves the session's agentctl from its recorded port, inspects only the matching `mock-agent` descendant, checks the process identity across adoption, and requires a populated replacement execution ID plus a fresh process after **Restart session**. Helper counters use atomics, and the negative matrix covers both revoked code prefixes and an expired-lease response with a non-401 status.

Implemented installation-derived signer selection in backend startup, preserving configured signing keys and honoring the optional disable setting. The selected source is logged once without sensitive values. Lease-only helper failures now retain their typed broker errors and advise **Restart session** for invalid or expired leases. The disabled broker rejects capability exchanges while continuing to issue redeemable leases.

The retained-process E2E adopts a lease-only process across backend restart, confirms the same execution remains active, then uses **Restart session** and verifies the same task/session receive a populated fresh execution with a usable capability. Its restart wait is passive, and its cleanup restores the exact workspace GitHub connection and credential mode captured before setup. Public documentation now describes the new-workspace credential default, automatic recovery, restart/resume behavior, and installation-key backup and rotation.

Verification passed:

- Focused Go tests across `backendapp`, `common/config`, `gitcredentials`, `github`, `orchestrator/executor`, and `cmd/agentctl` passed, including startup signer selection, the fixed HKDF vector, disabled-route aliases, pruned-lease recovery, lease-only fallback, and Git/`gh` diagnostics. The exact command is in Verification above.
- From `apps/backend`, the full configuration race tests and the focused archive, same-broker idle recovery, helper diagnostics, and executor fallback race tests passed:

  ```sh
  go test -race -trimpath -tags fts5 ./internal/common/config -count=1
  go test -race -trimpath -tags fts5 ./internal/backendapp ./internal/gitcredentials ./cmd/agentctl ./internal/orchestrator/executor -run 'Test(GitHubCredentialReissueRejectsArchivedTaskWithLiveSession|BrokerReissueRecoversPrunedIdleLease|GitHubCredentialHelperReportsLeaseRelaunch|GitHubCLIShimReportsLeaseRelaunch|GitHubCredentialHelperKeepsOtherLeaseErrorsUnchanged|ConfigureGitHubCredentialBrokerFallsBackToLeaseOnlyWhenReissueUnavailable)$' -count=1
  ```

- From the repository root, `bash scripts/lint-go-changed` reported 0 issues for all changed Go files:

  ```sh
  bash scripts/lint-go-changed \
    apps/backend/cmd/agentctl/github_credential.go \
    apps/backend/cmd/agentctl/github_credential_recovery_test.go \
    apps/backend/internal/backendapp/git_credential_reissue.go \
    apps/backend/internal/backendapp/git_credential_reissue_startup_test.go \
    apps/backend/internal/backendapp/git_credentials.go \
    apps/backend/internal/backendapp/services.go \
    apps/backend/internal/backendapp/services_github_broker_test.go \
    apps/backend/internal/backendapp/storage.go \
    apps/backend/internal/backendapp/types.go \
    apps/backend/internal/common/config/catalog.go \
    apps/backend/internal/common/config/catalog_test.go \
    apps/backend/internal/common/config/config.go \
    apps/backend/internal/common/config/git_credential_broker_test.go \
    apps/backend/internal/gitcredentials/reissue_test.go \
    apps/backend/internal/github/controller_auth.go \
    apps/backend/internal/github/credential_broker.go \
    apps/backend/internal/github/credential_broker_reissue_wire_test.go \
    apps/backend/internal/orchestrator/executor/executor_credentials_reissue_test.go \
    apps/backend/internal/orchestrator/executor/executor_credentials_test.go
  ```

- `make -C apps/backend build` passed, including native and cross-compiled helper targets. The managed E2E runner also rebuilt the backend and web assets successfully.
- From `apps/web`, the retained-process test passed with retries disabled:

  ```sh
  pnpm e2e:run --project=chromium e2e/tests/session/agent-survival-restart.spec.ts -- --grep 'restarts an adopted lease-only session with a fresh managed Git capability' --retries=0
  ```

- From `apps/web`, Prettier, both focused Oxlint configurations, and the E2E sleep ratchet passed:

  ```sh
  pnpm exec prettier --check e2e/tests/session/agent-survival-restart.spec.ts
  pnpm exec oxlint -c oxlint.config.mjs --deny-warnings e2e/tests/session/agent-survival-restart.spec.ts
  pnpm exec oxlint -c oxlint.e2e-sleeps.config.mjs --deny-warnings e2e/tests/session/agent-survival-restart.spec.ts
  pnpm run e2e:sleep-ratchet
  ```

- From the repository root, 63 public-doc tests passed, 55 public docs validated, specification validation and lint passed, and `git diff --check` passed:

  ```sh
  node --test scripts/validate-public-docs.test.mjs
  node scripts/validate-public-docs.mjs
  python3 scripts/list-docs.py validate
  python3 scripts/lint-spec-files.py --all
  git diff --check
  ```
