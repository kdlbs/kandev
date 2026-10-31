---
id: "01-align-reissue-wire-contract"
title: "Align the reissue wire contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001
acceptance_criteria:
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.2
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.7
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.8
system_design:
  - ../../specs/platform/system-design/git-credential-lease-reissue.md
---

# Task 01: Align the reissue wire contract

## Summary

Make the broker handler read the capability from `reissue_capability`, the field
every helper sends, and give the helper and handler one shared request type. A
live execution then recovers its lease over HTTP, including sessions whose
retained agentctl predates this change.

## In scope

- Add `githubauth.CredentialBrokerRequest` with the fields and JSON tags of
  `githubBrokerResolveRequest`, unchanged.
- Alias `githubBrokerResolveRequest` to the shared type.
- Remove `credentialBrokerHTTPRequest` and use the shared type in the resolve and
  reissue handlers.
- Replace `reissueCredentialLeaseBody` in `credential_broker_http_test.go` with
  the shared type, so `TestCredentialBrokerReissueRouteAcceptsOnlyScopedCapability`
  keeps testing the real contract.

## Out of scope

- Renaming `reissue_capability` anywhere, or accepting `capability` as an alias.
  `KANDEV_GITHUB_CREDENTIAL_SCOPES` and retained helpers depend on the current
  name.
- Changing helper retry behavior, broker authorization, or capability contents.
- The exclusions listed in [plan.md](plan.md#out-of-scope).

## Acceptance

- A request marshaled from `githubauth.CredentialBrokerRequest` and a literal
  `reissue_capability` JSON body each receive a lease on both
  `/api/v1/github/credentials/reissue` and `/api/v1/git/credentials/reissue`, and
  that lease resolves to a credential.
- A body carrying only `capability` is denied with
  `*_credential_reissue_denied` and issues no lease.
- The rotated-generation and fresh-broker scenarios recover over HTTP using one
  reissue and one replacement resolve.

## Verification

Write the tests first and confirm that the regression test fails with 401 before
changing the handler:

```bash
(cd apps/backend && go test -trimpath -count=1 -run 'TestCredentialBrokerReissueAcceptsHelperWireField' ./internal/github)
```

After the fix:

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/github ./internal/githubauth ./internal/gitcredentials ./internal/orchestrator/executor ./internal/auth/httpmw ./cmd/agentctl)
(cd apps/backend && GOOS=linux GOARCH=amd64 go build -trimpath -o /dev/null ./cmd/agentctl)
bash scripts/lint-go-changed apps/backend/internal/githubauth/broker_request.go apps/backend/internal/githubauth/broker_request_test.go apps/backend/cmd/agentctl/github_credential.go apps/backend/internal/github/controller_auth.go apps/backend/internal/github/credential_broker_http_test.go apps/backend/internal/github/credential_broker_reissue_wire_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Tests to add

`apps/backend/internal/github/credential_broker_reissue_wire_test.go` is a new
file. Reuse `newPATCredentialBroker`, `newBrokerHTTPTestRouter` and
`gitcredentials.NewReissueCapabilitySigner`.

- `TestCredentialBrokerReissueAcceptsHelperWireField`: table over both routes and
  both body forms from the first acceptance condition. Follow the reissue with a
  resolve of the returned lease and assert a credential comes back.
- `TestCredentialBrokerReissueRejectsLegacyCapabilityField`: posts `capability`
  only and expects 401 with the route's denied code.
- `TestCredentialBrokerReissueRecoversRotatedLeaseOverHTTP` (AC .7): increments
  `connection.CredentialGeneration`, then expects resolve of the old lease to
  return `*_lease_revoked`, followed by a successful reissue and resolve.
- `TestCredentialBrokerReissueRecoversAfterRestartOverHTTP` (AC .8): mints the
  capability on one broker, then expects a second `newPATCredentialBroker`
  sharing the signing key, with an empty lease map, to reissue and resolve.

`apps/backend/internal/githubauth/broker_request_test.go` is a new file.

- `TestCredentialBrokerRequestWireFields`: marshals a fully populated value and
  asserts the exact JSON key set, including `reissue_capability`.

## Files likely touched

- `apps/backend/internal/githubauth/broker_request.go` (new)
- `apps/backend/internal/githubauth/broker_request_test.go` (new)
- `apps/backend/cmd/agentctl/github_credential.go`
- `apps/backend/internal/github/controller_auth.go`
- `apps/backend/internal/github/credential_broker_http_test.go`
- `apps/backend/internal/github/credential_broker_reissue_wire_test.go` (new)

## Dependencies

None.

## Risks

- `go build ./cmd/agentctl` must stay free of `internal/github`. The shared type
  lives in `githubauth` for this reason. Do not import `internal/github` from
  agentctl, including in tests.
- `newPATCredentialBroker` returns a shared `*WorkspaceConnection`. Confirm the
  rotation test's generation change reaches the broker's binding check, as the
  broker-level test in `credential_broker_test.go` does.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/git-credential-lease-reissue.md)
- [Plan root cause and approach](plan.md#root-cause)
- `apps/backend/internal/github/credential_broker_http_test.go:213` for the
  router test pattern.

## Results

Done. The handler now binds the helper's field and the two tiers share one
request type.

- `internal/githubauth/broker_request.go` adds `CredentialBrokerRequest` with
  the previous field names and JSON tags unchanged.
- `cmd/agentctl/github_credential.go` aliases `githubBrokerResolveRequest` to
  it, so the helper's wire output and `KANDEV_GITHUB_CREDENTIAL_SCOPES`
  encoding stay byte-identical.
- `internal/github/controller_auth.go` drops `credentialBrokerHTTPRequest`,
  decodes the shared type, and passes `request.ReissueCapability`.
- `credential_broker_http_test.go` posts the shared type end to end, and
  `credential_broker_reissue_wire_test.go` adds the four regression tests.

Verification:

- Focused tests: `go test -trimpath -count=1 ./internal/github
  ./internal/githubauth ./internal/gitcredentials ./internal/orchestrator/executor
  ./internal/auth/httpmw ./cmd/agentctl` passed.
- After review remediation, `go test -trimpath -count=1 ./internal/github
  ./internal/githubauth ./cmd/agentctl` passed again. The regression bodies now
  include the `path` the helper always sends, and both broker HTTP test files
  use the shared request type.
- `bash scripts/lint-go-changed` reported 0 issues before and after remediation.
- `go build -trimpath ./cmd/agentctl` passed without importing
  `internal/github`.
- The work order links the new
  `docs/specs/platform/system-design/git-credential-lease-reissue.md`, and the
  local documentation coverage evaluator reports `status: covered`.
