---
created: 2026-10-30
status: done
requirements:
  - REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001
system_design:
  - ../../specs/platform/system-design/git-credential-lease-reissue.md
legacy_specs: []
---

# Implementation Plan: Agentctl Credential Reissue Wire Contract

## Overview

Fix [issue #4981](https://github.com/kdlbs/kandev/issues/4981): managed Git
credential lease reissue has never worked over HTTP. The result is that a live
execution recovers its lease after a credential rotation or backend restart, as
the requirement already states. One work order is enough, because the handler
change and its regression test must land together.

The design is declared in
[the system design](../../specs/platform/system-design/git-credential-lease-reissue.md).
The requirement's API surface now names the request field, and no acceptance
criterion changes.

## Root cause

The helper and the handler disagree on the JSON field that carries the
capability, and neither side's tests can see the other.

- `apps/backend/cmd/agentctl/github_credential.go:22` sends
  `reissue_capability`. `reissue()` posts that struct to `/reissue`.
- `apps/backend/internal/github/controller_auth.go:245` binds `capability`, so
  `httpReissueCredentialLease` passes an empty capability to the broker.
- `gitcredentials.ReissueCapabilitySigner.Validate` rejects the empty value with
  `ErrReissueCapabilityInvalid`, which the handler returns as HTTP 401
  `*_credential_reissue_denied`.

Both sides have had these names since #2850 added the path. The agentctl tests
use a fake server that decodes into the client's struct. The handler test uses
its own body struct tagged `capability`. Both pass, and no test crosses the
boundary.

### Reproduction evidence

A temporary test, since removed, posted the helper's request shape through the
real router using a valid capability:

| Field sent           | Route                                | Result                                   |
| -------------------- | ------------------------------------ | ---------------------------------------- |
| `reissue_capability` | `/api/v1/github/credentials/reissue` | 401 `github_credential_reissue_denied`   |
| `reissue_capability` | `/api/v1/git/credentials/reissue`    | 401 `git_credential_reissue_denied`      |
| `capability`         | `/api/v1/github/credentials/reissue` | 200 with a lease                         |
| `capability`         | `/api/v1/git/credentials/reissue`    | 200 with a lease                         |

Only the field name separates failure from success. The rest of the reissue
path works once the capability arrives.

## Scope

### In scope

- The handler binds `reissue_capability`, the name every helper already sends.
- The helper and the handler share one request type, so the names cannot drift.
- Regression tests through the real router for AC .7 and AC .8.

### Out of scope

- Other #3073 symptoms, such as scope-keyed reclones orphaning worktrees. This
  fix removes only the lease rebinding failure, so #3073 stays open until it is
  retested.
- Recovery after a repository transfer to another organization. The capability
  seals the launch-time path, and live authorization compares it with the
  persisted repository identity. A session whose identity no longer matches is
  denied by design (requirement, Permissions and Failure modes), so it may still
  need a new session.
- Installs without `KANDEV_GITHUB_CREDENTIAL_BROKER_REISSUE_SIGNING_KEY`. No
  capability is issued, and that path stays fail-closed as specified.
- Removing the capability from `/resolve` request bodies. The handler ignores it
  there and the destination is the same broker, so this is a separate hardening
  change.

## Technical approach

### Why the server changes rather than the client

The issue suggests having the client send `capability`. Change the handler
instead, for two reasons:

1. **The client's tag is also a storage format.** `githubBrokerResolveRequest`
   decodes `KANDEV_GITHUB_CREDENTIAL_SCOPES`, which
   `orchestrator/executor.githubCredentialScope` writes with
   `reissue_capability`. Renaming the tag makes every multi-scope helper decode
   an empty capability, and `resolveWithReissue` then skips reissue silently.
   Avoiding that needs a second, separate request shape in the client.
2. **Running helpers keep their old binary.** A retained agentctl keeps sending
   `reissue_capability` after a backend upgrade. A handler-side fix recovers
   those sessions with a backend upgrade alone. A client-side fix would also need
   a server alias to do the same.

No production client has ever sent `capability`: repository search finds only
the handler and its test. The rename therefore needs no alias, and accepting a
second spelling would add a permanent path with no caller.

### Changes

- **`internal/githubauth/broker_request.go` (new):** add
  `CredentialBrokerRequest`, moving the fields and tags of
  `githubBrokerResolveRequest` unchanged. The wire output of current clients
  stays byte-identical.
- **`cmd/agentctl/github_credential.go`:** make
  `type githubBrokerResolveRequest = githubauth.CredentialBrokerRequest`. The
  alias leaves call sites and tests unchanged. `githubauth` is the existing
  neutral contract package that both tiers can import.
- **`internal/github/controller_auth.go`:** remove `credentialBrokerHTTPRequest`,
  have `decodeCredentialBrokerHTTPRequest` return the shared type, and pass
  `request.ReissueCapability` from `httpReissueCredentialLease`.

`internal/github` gains its first import of `githubauth`. That package depends
only on `gitconfigenv`, so no cycle results.

## Tests

The regression test that fails before the fix is
`TestCredentialBrokerReissueAcceptsHelperWireField`.

| Criterion | Evidence                                                                                                                                                                                       |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AC .1     | Existing `TestGitHubCredentialHelperStopsWhenReissueFails`                                                                                                                                     |
| AC .2     | `TestCredentialBrokerReissueRejectsLegacyCapabilityField`, plus existing helper assertions that the lease is empty on reissue                                                                  |
| AC .5     | Existing helper tests for non-reissuable and failed reissue paths                                                                                                                              |
| AC .7     | `TestCredentialBrokerReissueRecoversRotatedLeaseOverHTTP`                                                                                                                                      |
| AC .8     | `TestCredentialBrokerReissueRecoversAfterRestartOverHTTP`                                                                                                                                      |
| Wire form | `TestCredentialBrokerRequestWireFields` pins the JSON keys. Existing `executor_credentials_test.go` and `TestGitHubCredentialHelperReissuesLeaseInMultiScopeMode` pin the scope environment form |

## E2E tests

None. The behavior has no browser surface, and the router-level tests exercise
the real binding, middleware and broker with the type that the helper marshals.

## Work orders

- [x] [Task 01: Align the reissue wire contract](task-01-align-reissue-wire-contract.md)

## Verification results

Implementation completed in the primary conversation.

- RED before the handler change: `TestCredentialBrokerReissueAcceptsHelperWireField`
  returned 401 `github_credential_reissue_denied` and
  `git_credential_reissue_denied` for both routes and both body forms.
- GREEN after it: the same test passes for
  `/api/v1/github/credentials/reissue` and `/api/v1/git/credentials/reissue`,
  and every reissued lease resolves to a credential.
- `go test -trimpath -count=1` passed for `internal/github`,
  `internal/githubauth`, `internal/gitcredentials`,
  `internal/orchestrator/executor`, `internal/auth/httpmw`, and
  `cmd/agentctl` before review, and for `internal/github`,
  `internal/githubauth`, and `cmd/agentctl` after review remediation.
- Review remediation: the regression bodies now include the `path` the helper
  always sends, both broker HTTP test files use the shared request type, the
  shared type's comment names the executor-side environment writer, and the
  production helper comment states its invariant without bug history.
- `GOOS=linux GOARCH=amd64 go build -trimpath -o /dev/null ./cmd/agentctl`
  passed, and `go list -deps ./cmd/agentctl` contains no `internal/github`
  package.
- `bash scripts/lint-go-changed` reported 0 issues for every changed Go file
  before and after review remediation.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed after the system design was
  added.
- Docs coverage: the work order now links
  `docs/specs/platform/system-design/git-credential-lease-reissue.md`, and the
  local `.github/scripts/pr-docs.cjs` evaluator reports `status: covered` with
  no errors for the PR file set.
- `git diff --check` reported no whitespace errors.

## Risks

- **Direct callers of the route:** an external script posting `capability` would
  start receiving 401. The field is undocumented, the route's only supported
  caller is the helper, and such a request is refused rather than granted.
- **Reissue traffic will start succeeding for the first time:** the live
  authorization and lease-replacement paths have run at broker level but not in
  production through this route. Those checks fail closed, and a capability
  reissue replaces its previous lease rather than consuming the workspace limit.
- **Parallel reissues in one session can invalidate each other:** every `git`
  or `gh` invocation starts a fresh helper whose lease comes from the unchanged
  environment, so once the launch lease dies, each call reissues with the same
  capability. `storeLease` deletes every earlier lease carrying that
  capability. If process B reissues while process A sits between its reissue
  and its retry, B deletes A's lease and A's single allowed retry fails with
  `*_lease_invalid` (AC .1). A later sequential call succeeds. The route never
  issued a lease before this fix, so the race was unreachable through it. A
  follow-up decides whether to act on it; this change alters no broker or
  helper retry behavior.
