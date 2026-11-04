---
created: 2026-10-28
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-004
system_design:
  - ../../specs/integrations/system-design/github-enterprise-hosts.md
legacy_specs: []
---

# Implementation Plan: GitHub Enterprise PR base identity

## Overview

Fix the launch failure for a task created from a pull request on a GitHub
Enterprise Server (GHES) or GHE.com host. Today, the launch fails with
`pull request identity did not match the task repository binding`. One work
order corrects both halves of the PR base identity check so that they carry
the real provider host. This is a backend-only correction.

## Evidence and root cause

Reported symptom: a task created from a PR on a GitHub Enterprise Server
fails to start with `pull request identity did not match the task repository
binding`.

The error is returned only from `resolvePRBaseForLaunch` in
`apps/backend/internal/orchestrator/executor/executor_resume.go`, when
`validPRBaseIdentity` rejects the live PR and the task repository has no stored
comparison target. A newly created PR task has no stored target, so the
mismatch is fatal.

Read-only trace of a GHES repository row
(`Provider: "github"`, `ProviderHost: "https://git.example.com"`):

1. `githubComparisonRepositoryFromRepository` passes the host to
   `normalizeGitHubComparisonRepository`.
2. `isGitHubComparisonHost` returns true only when the hostname equals
   `github.com`. The GHES host returns `ok=false`, so the attached repository
   has no identity.
3. `validUnboundPRBaseIdentity` returns `repository_identity_missing`. Because
   `expected == nil`, the launch fails with the reported error. The backend
   logs `live pull request base identity mismatch` with
   `mismatch_reason=repository_identity_missing`.

There is a second public-host assumption on the provider side.
`githubComparisonRepository` in `apps/backend/internal/backendapp/orchestrator.go`
hardcodes `Host: "github.com"` and a `https://github.com/...` remote URL for
the PR returned by `GitHubService.GetPRForAutomation`. Correcting only the
executor would turn the failure into a host mismatch instead of a success.

The comparison-target model already supports Enterprise hosts:
`ComparisonTarget.Validate` uses `githubhost.Parse`, and
`TestEnterpriseComparisonTarget` covers it. The defect is confined to these two
launch-path conversions.

## Requirement conformance

Classification: the implementation violates active acceptance criteria. No
requirement or system-design change is needed.

| Finding | Authoritative criteria |
| --- | --- |
| PR-task launch rejects every Enterprise repository | `AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.4` (launch preserves host identity) |
| PR base identity is rebuilt as github.com | `AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1` (PR/fork workflows and task associations preserve host) |
| A PR from another host must not satisfy the binding | `AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2`, `.003.3` |
| Existing github.com and empty-host legacy rows keep working | `AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.5`, `.003.4` |

The [Enterprise host design](../../specs/integrations/system-design/github-enterprise-hosts.md)
already requires host threading through "comparison targets and remote
contributions" (Workflow consumers and capabilities) and host preservation on
launch (Task execution).

## Scope

### In scope

- Executor-side attached and head repository identity uses `githubhost.Parse`
  and keeps the canonical authority, including a non-default GHES port.
- The provider-side PR base carries the host that the PR was actually fetched
  from, stamped by `GetPRForAutomation` on a response copy.
- Cross-host PRs with equal owner, name, and number fail closed.
- Empty `ProviderHost` legacy rows keep their github.com meaning.
- Launch identity checks accept numeric and node IDs only when the live PR
  response confirms they identify the same head repository.

### Out of scope

The following remaining public-host assumptions are separate follow-ups. They
do not cause the reported error.

- Fork contribution credential scopes in `executor_credentials.go`
  (`issueGitHubRemoteContributionCredentialScope`,
  `issueGitHubContributionDestinationCredentialScope`).
- PR URL classification in `backendapp/remote_contribution.go` and the
  github.com check in `task_change_link_coordinator.go`.
- `isPublicGitHubProviderHost` in `backendapp/services.go`. It is deliberately
  public-only for the canonical Kandev repository check.
- Schema, frontend, and public documentation changes. None are needed for this
  correction.

## Technical approach

### Executor (`orchestrator/executor/executor_resume.go`)

Replace `isGitHubComparisonHost` and the github.com rewrite in
`normalizeGitHubComparisonRepository` with `githubhost.Parse`. Set `Host` to
`host.String()` and `RemoteURL` to `host.WebURL() + "/owner/name.git"`. An
empty `ProviderHost` still defaults to github.com. Invalid hosts (userinfo,
path, query, IP literal, or a port on github.com) still return `ok=false`.
Identity equality already compares host case-insensitively.

### Provider (`backendapp/orchestrator.go`, `github/service_pr.go`)

`GetPRForAutomation` returns a copy with `GitHubHost` set to
`resolved.host().String()`, following `prsForHost`. `githubPRBaseFromPR`
builds both repositories from that host through `githubhost.Parse`. An empty
value means github.com, which keeps stubs and legacy callers working. A
malformed value fails the conversion.

The executor then compares host-qualified identities. A PR fetched from a
different host than the attached repository fails with
`target_repository_mismatch` or `head_repository_mismatch`, not
`repository_identity_missing`.

### Live repository ID compatibility

PR reconciliation and remote contributions can store a GitHub node ID, while
repository imports and launch conversion use numeric IDs. Carry the live head
node ID in the transient `PRBase` observation, outside serialized metadata.
During launch comparison, normalize a stored node ID to the numeric ID only
when the live response supplies both aliases for the same host and path.
Compare copies so stored targets remain unchanged. Unknown aliases, changed
IDs, different hosts, and different paths still fail identity checks.

## Work orders

| ID | Work order | Depends on |
| --- | --- | --- |
| 01 | [Host-qualified PR base identity](task-01-host-qualified-pr-base.md) | none |

## Verification strategy

Unit regressions at the two conversion boundaries plus a launch-level
regression through `resolveTaskRepoInfo`. Each regression must fail before the
correction. The existing github.com and fork suites must keep passing. Live
GHES access is not available. This plan does not claim provider smoke
evidence, consistent with the Enterprise host design.

## Risks

- `TestNormalizeGitHubComparisonRepositoryRejectsOtherHosts` currently treats
  `github.com.evil` as invalid. Under `githubhost`, it is a valid GHES
  authority. The test must change to assert that the identity is distinct from
  github.com, not rejected. Security comes from host-qualified equality.
- PR reconciliation already preserves Enterprise hosts in stored comparison
  targets. This fix corrects the launch-time conversion and accepts verified
  live ID aliases without rewriting those targets. No migration is required.
