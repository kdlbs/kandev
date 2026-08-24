---
id: "01-default-reissue-signer"
title: "Default Git credential reissue signer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/git-credential-lease-reissue/spec.md"
---

# Task 01: Default Git credential reissue signer

## Acceptance

1. An empty configured reissue signing key creates a cryptographically random,
   process-local signer; operating-system entropy failure cannot silently leave
   the broker in lease-only mode.
2. Managed sessions launched by that broker receive a non-empty repository-
   exact reissue capability; two process-local signers cannot validate each
   other's capabilities.
3. A non-empty configured signing key retains the existing stable signer and
   exact-scope, single-reissue behavior.

## Verification

```bash
cd apps/backend && GOCACHE=/tmp/kandev-git-reissue-gocache GOMODCACHE=/tmp/kandev-git-reissue-modcache go test -v ./internal/backendapp ./cmd/agentctl -run 'Test(NewGitCredentialBrokerDefaultsToEphemeralReissueSigner|NewGitCredentialBrokerPreservesConfiguredSigner|GitHubCredentialHelperReissuesLeaseInMultiScopeMode)$' -count=1
```

## Files likely touched

- `apps/backend/internal/backendapp/git_credentials.go`
- `apps/backend/internal/backendapp/git_credentials_test.go`

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- Spec sections: `What`, `Failure modes`, `Persistence guarantees`, and
  `Scenarios`.
- Plan sections: `Confirmed root cause`, `Backend`, `Tests`, and `Risks`.
- Existing patterns:
  `gitcredentials.NewReissueCapabilitySigner`,
  `Broker.IssueWithReissueCapability`, and
  `TestGitHubCredentialHelperReissuesLeaseInMultiScopeMode`.

## Output contract

Report the exact changed files, red/green regression evidence, targeted command
results, remaining risks, and synchronized task/plan status. Do not log or print
signing keys, capabilities, leases, or resolved provider credentials.

## Results

- RED: `GOCACHE=/tmp/kandev-git-reissue-gocache GOMODCACHE=/tmp/kandev-github-lease-design-modcache GOPROXY=off go test -v ./internal/backendapp -run '^TestNewGitCredentialBrokerDefaultsToEphemeralReissueSigner$' -count=1` failed with `git credential lease reissue is unavailable` before the production change.
- GREEN: the same focused regression passed after the production change.
- Targeted verification: `GOCACHE=/tmp/kandev-git-reissue-gocache GOMODCACHE=/tmp/kandev-git-reissue-modcache GOPROXY=off go test -v ./internal/backendapp ./cmd/agentctl -run 'Test(NewGitCredentialBrokerDefaultsToEphemeralReissueSigner|NewGitCredentialBrokerPreservesConfiguredSigner|GitHubCredentialHelperReissuesLeaseInMultiScopeMode)$' -count=1` passed: 2 backendapp tests and 1 agentctl test.
- Generated artifacts: Go build and module caches under `/tmp/kandev-git-reissue-gocache` and `/tmp/kandev-git-reissue-modcache`; no credential, lease, capability, or signing-key value was logged or persisted by the tests.
- Security boundary: repository-exact scope validation and the one-reissue limit remain unchanged. A session launched before the fixed backend cannot be retrofitted.
