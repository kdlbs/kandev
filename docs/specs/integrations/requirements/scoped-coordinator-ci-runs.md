---
status: active
system: integrations
created: 2026-08-30
owners:
  - kandev
---

# Scoped coordinator CI run requirements

## Overview

A task can reach CI Fixup when its linked pull request has an unchanged head but
GitHub created no new run after an external or shared prerequisite changed. The
task's agent and the workspace Coordinator cannot legitimately recover because
rerun and workflow-dispatch operations may require repository Actions write
authority that task credentials must not hold. Integrations owns this contract
because it owns GitHub credentials, provider policy, and the scoped CI run
lifecycle. Task, workflow, and MCP systems consume the grant and receive the
receipt contract.

## Terminology

- **Coordinator grant:** A durable, server-owned, capability-specific grant that
  binds one coordinator task to one workspace, workflow, and CI Fixup step.
- **Source run:** The existing GitHub Actions run whose repository, workflow,
  event, head repository, head ref, and exact head SHA identify the allowed
  recovery target.
- **Evidence kind:** The proof basis for a fresh run: `pr_head` or
  `current_merge`.
- **Rerun-first:** Re-running the failed jobs of the named source attempt, which
  preserves the original run identity and actor privileges.

## Requirements

### REQ-INTEGRATIONS-SCOPED-CI-RUNS-001: Admit only granted coordinator recovery

**Intent:** Only a session whose task holds an active, administrator-created
coordinator grant may request a fresh CI run, and every identity the request
names must match durable server state.

**User story:** As a workspace administrator, I want the recovery capability
limited to the designated coordinator task for an exact workspace, workflow,
and CI Fixup step, so no other task or actor can trigger provider writes.

#### Acceptance criteria

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.1:** When a request arrives whose calling
  task does not own an active grant for the exact workspace, workflow, and
  current CI Fixup step, the system shall reject it before any provider call.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.2:** When the request's named task,
  task repository, linked pull request, workflow step, expected head SHA, or
  source run identity does not match the server-resolved durable state, the
  system shall fail closed with the typed class identifying the mismatch.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.3:** When the caller supplies repository
  owner or name, ref, workflow ID or path, workflow inputs, provider
  credentials, actor identity, or GitHub App identity, the system shall reject
  the request as an unknown field before handler execution.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.4:** The system shall resolve the
  target task, workspace, workflow, current step, task-repository attachment,
  active linked pull request association, and canonical repository from
  durable state, never from caller-supplied metadata.

### REQ-INTEGRATIONS-SCOPED-CI-RUNS-002: Use only the verified App installation

**Intent:** The provider mutation runs exclusively through the workspace's
verified GitHub App installation with the minimum permission set, and no
credential or generic Actions authority reaches the calling agent.

**User story:** As a repository owner, I want the recovery operation performed
by the installed App with repository-scoped Actions write permission, so task
agents never receive tokens, keys, sockets, or arbitrary Actions access.

#### Acceptance criteria

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-002.1:** The system shall perform the
  provider call only with a repository-scoped installation token minted for
  exactly the canonical repository, carrying metadata read, pull request read,
  contents read when dispatch-policy inspection needs it, and Actions write.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-002.2:** When the workspace has no verified
  App installation, or the installation does not report the required
  permission set, the system shall fail closed with the installation failure
  class and never fall back to PAT, named CLI, personal OAuth, executor-profile,
  ambient, or legacy shared credentials.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-002.3:** No token, App JWT or private key,
  secret-store pointer, broker lease, Docker access, socket, sudo, or generic
  Actions endpoint shall reach the calling agent or appear in receipts,
  failures, audit rows, or logs.

### REQ-INTEGRATIONS-SCOPED-CI-RUNS-003: rerun-first with typed evidence policy

**Intent:** Recovery preserves the reviewed run identity by rerunning the
failed jobs of the named source attempt, and only a verified identity chain
admits the request.

**User story:** As a coordinator, I want the fresh run to execute at the exact
reviewed head through the original workflow, so the recovered CI result remains
attributable to the reviewed pull request head.

#### Acceptance criteria

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-003.1:** The system shall rerun the failed
  jobs of the named source attempt first, and shall not submit a
  workflow-dispatch fallback after GitHub definitively rejects the rerun.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-003.2:** When the source run is not a
  completed failed `pull_request` run of the named workflow, repository, head
  ref, and exact head SHA, the system shall reject the request with the
  source-run mismatch class. An association with a different pull request
  shall not fall back to tuple matching; the base-plus-head tuple matches only
  when GitHub returns an empty pull request association list.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-003.3:** When the observed pull request head
  differs from the expected head, the system shall fail with the head drift
  class and report the observed provider head repository, ref, and SHA without
  relabelling runtime execution SHAs as pull request head evidence.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-003.4:** For fork pull requests the system
  shall prefer rerun because it preserves the original run privileges, and shall
  deny workflow dispatch to fork repositories.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-003.5:** For `current_merge` evidence the
  system shall fail closed with `merge_evidence_unavailable` until the provider
  exposes an immutable run execution ref and SHA that can be compared with the
  pull request's current merge commit; it shall never infer that evidence.

### REQ-INTEGRATIONS-SCOPED-CI-RUNS-004: Durably claim and reconcile operations

**Intent:** Every provider mutation is durably claimed before the call, and an
interrupted or ambiguous send is reconciled read-only, never blindly resent.

**User story:** As a coordinator, I want retries of the same request to be
idempotent and races to converge on one provider mutation, so recovery cannot
duplicate runs or lose state.

#### Acceptance criteria

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-004.1:** The system shall record a durable
  claim keyed by the caller's idempotency key within the actor scope before any
  provider call, and a second unique identity shall cover the semantic source
  run attempt, so concurrent or retried claims return the same logical request.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-004.2:** Once a provider call may have
  started, the system shall reconcile only from GitHub and shall never blindly
  submit the mutation again.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-004.3:** Rerun reconciliation shall accept
  only the exact next attempt of the source run with the verified head and
  workflow identity; zero or multiple matching attempts remain ambiguous.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-004.4:** When a provider rate limit is
  observed, the system shall record the provider-supplied reset evidence and
  make the same request eligible again only after that time, preserving the
  provider-start marker.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-004.5:** Before the provider call, one
  execution lease shall own the transition; an expired lease may be taken over
  after a worker crash, and provider start shall succeed only for the current
  lease owner.

### REQ-INTEGRATIONS-SCOPED-CI-RUNS-005: Non-secret receipt and audit trail

**Intent:** Every confirmed operation and typed failure returns a complete,
non-secret receipt, and terminal outcomes persist atomically with a redacted
audit row.

**User story:** As a workspace administrator, I want a complete receipt and
audit trail without secrets, so I can verify what ran and why without exposing
provider credentials.

#### Acceptance criteria

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-005.1:** A confirmed operation shall return
  the request identity, idempotency status, strategy, repository and pull
  request, expected and observed head, provider head repository, ref, and SHA,
  workflow identity, source and result run identities and attempts, event,
  execution or merge identity only when provider-proven, evidence kind and
  verdict, non-secret App principal, provider request identity, and timestamps.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-005.2:** A typed failure shall return the
  same durable receipt when a logical request exists, with a stable failure
  class drawn from the documented class set.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-005.3:** Audit rows shall record actor task
  and session, workspace, workflow and step, repository and pull request, head
  identities, source and result attempts, operation and evidence decisions,
  non-secret App and provider identities, failure class, and timestamps; they
  shall never contain tokens, App private keys, authorization headers, provider
  response bodies, idempotency keys or hashes, or arbitrary caller input.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-005.4:** A terminal request transition and
  its terminal audit row shall commit or roll back together.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-005.5:** The system shall not write a
  successful check or alter linked pull request check state to manufacture CI
  success; the real Actions run is consumed by existing GitHub synchronization.

### REQ-INTEGRATIONS-SCOPED-CI-RUNS-006: Administrator grant lifecycle

**Intent:** Workspace administrators create, replace, and revoke coordinator
grants through an authenticated server API, outside task MCP.

**User story:** As a workspace administrator, I want to bind one coordinator
task to this capability for an exact workspace, workflow, and CI Fixup step,
so the authority is explicit, revocable, and generation-tracked.

#### Acceptance criteria

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-006.1:** A grant shall identify one
  coordinator task, workspace, workflow, allowed CI Fixup step, target task, and
  task repository, and shall grant no other task or GitHub authority.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-006.2:** Replacing a grant for the same
  exact scope shall revoke the prior generation and insert its monotonically
  increasing successor in one transaction; a failed replacement shall roll
  back the revocation.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-006.3:** Ordinary agents, sibling or child
  tasks, unrelated coordinators, and other workspaces shall not be able to
  create, use, or replace a grant through task MCP.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-006.4:** Grant revocation shall take
  effect for subsequent request admission without redeploying the backend.

## Exclusions

- Generalized Actions administration, arbitrary workflow, ref, or input
  selection, synthetic check statuses, merging, source or history changes, and
  agent-visible credentials.
- Treating a deployment-local proxy, permission customization, or operator
  script as canonical platform behavior. Only the reviewed in-repository server
  capability qualifies for upstream acceptance.
- First-slice UI changes for grant management; the first slice is backend and
  documentation only.

## Related contracts

- [Workspace GitHub authentication](github-authentication.md)
- [GitHub task pull request sync coordination](github-task-pr-sync-coordination.md)
- [System design](../system-design/scoped-coordinator-ci-runs.md)
- [Implementation package](../../../plans/scoped-coordinator-ci-runs/plan.md)
