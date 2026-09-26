---
status: deprecated
system: integrations
created: 2026-08-30
owners:
  - kandev
---

# Scoped coordinator CI run requirements

This action-specific server proxy is being replaced by
[provider access for managed plugin sessions](provider-session-access.md).
Its old acceptance criteria remain here only to identify the unshipped
implementation being removed from PR #3165.

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

### REQ-INTEGRATIONS-SCOPED-CI-RUNS-001: Recover a stranded CI Fixup head through a granted coordinator

**Intent:** Only a session whose task holds an active, administrator-created
coordinator grant may obtain a fresh, real GitHub Actions run for an unchanged
linked pull request head, with every identity verified server-side, provider
access limited to the workspace's verified GitHub App installation, execution
claimed durably and reconciled without resends, complete non-secret receipts
and audit, and an administrator-managed revocable grant lifecycle. This is one
cohesive recovery capability: admission, provider policy, evidence
verification, idempotent execution, receipt and audit, and grant lifecycle
together define the scoped coordinator-initiated CI recovery operation.

**User story:** As a workspace administrator, I want the designated coordinator
task to recover a stranded exact-head CI run through the installed GitHub App
without receiving any credential or broader authority, so the recovery is
attributable, idempotent, and revocable.

#### Acceptance criteria

**Grant admission and closed request**

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.1:** When a request arrives whose calling
  task does not own an active grant for the exact workspace, workflow, and
  current CI Fixup step, the system shall reject it before any provider call.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.2:** When the request's named task, task
  repository, linked pull request, workflow step, expected head SHA, or source
  run identity does not match the server-resolved durable state, the system
  shall fail closed with the typed class identifying the mismatch.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.3:** When the caller supplies repository
  owner or name, ref, workflow ID or path, workflow inputs, provider
  credentials, actor identity, or GitHub App identity, the system shall reject
  the request as an unknown field before handler execution.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.4:** The system shall resolve the
  target task, workspace, workflow, current step, task-repository attachment,
  active linked pull request association, and canonical repository from
  durable state, never from caller-supplied metadata.

**Provider policy**

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.5:** The system shall perform the
  provider call only with a repository-scoped installation token minted for
  exactly the canonical repository, carrying metadata read, pull request read,
  contents read when dispatch-policy inspection needs it, and Actions write.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.6:** When the workspace has no verified
  App installation, or the installation does not report the required
  permission set, the system shall fail closed with the installation failure
  class and never fall back to PAT, named CLI, personal OAuth,
  executor-profile, ambient, or legacy shared credentials.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.7:** No token, App JWT or private key,
  secret-store pointer, broker lease, Docker access, socket, sudo, or generic
  Actions endpoint shall reach the calling agent or appear in receipts,
  failures, audit rows, or logs.

**Rerun-first with typed evidence policy**

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.8:** The system shall rerun the failed
  jobs of the named source attempt first, and shall not submit a
  workflow-dispatch fallback after GitHub definitively rejects the rerun.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.9:** When the source run is not a
  completed failed `pull_request` run of the named workflow, repository, head
  ref, and exact head SHA, the system shall reject the request with the
  source-run mismatch class. An association with a different pull request
  shall not fall back to tuple matching; the base-plus-head tuple matches only
  when GitHub returns an empty pull request association list.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.10:** When the observed pull request
  head differs from the expected head, the system shall fail with the head
  drift class and report the observed provider head repository, ref, and SHA
  without relabelling runtime execution SHAs as pull request head evidence.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.11:** For fork pull requests the system
  shall prefer rerun because it preserves the original run privileges, and
  shall deny workflow dispatch to fork repositories.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.12:** For `current_merge` evidence the
  system shall fail closed with `merge_evidence_unavailable` until the
  provider exposes an immutable run execution ref and SHA that can be
  compared with the pull request's current merge commit; it shall never infer
  that evidence.

**Durable claims and reconciliation**

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.13:** The system shall record a durable
  claim keyed by the caller's idempotency key within the actor scope before
  any provider call, and a second unique identity shall cover the semantic
  source run attempt, so concurrent or retried claims return the same logical
  request.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.14:** Once a provider call may have
  started, the system shall reconcile only from GitHub and shall never blindly
  submit the mutation again. Rerun reconciliation shall accept only the exact
  next attempt. A provider rate limit shall record the provider reset
  evidence and defer the same request until that time, preserving the
  provider-start marker. Before the provider call, one execution lease owns
  the transition, and an expired lease may be taken over after a worker
  crash.

**Non-secret receipt and audit**

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.15:** A confirmed operation shall
  return the request identity, idempotency status, strategy, repository and
  pull request, expected and observed head, provider head repository, ref,
  and SHA, workflow identity, source and result run identities and attempts,
  event, execution or merge identity only when provider-proven, evidence kind
  and verdict, non-secret App principal, provider request identity, and
  timestamps. A typed failure shall return the same durable receipt when a
  logical request exists, with a stable failure class from the documented
  set.
- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.16:** Audit rows shall record the actor
  task and session, workspace, workflow and step, repository and pull
  request, head identities, source and result attempts, operation and
  evidence decisions, non-secret App and provider identities, failure class,
  and timestamps; they shall never contain tokens, App private keys,
  authorization headers, provider response bodies, idempotency keys or
  hashes, or arbitrary caller input. A terminal request transition and its
  terminal audit row shall commit or roll back together. The system shall
  not write a successful check or alter linked pull request check state to
  manufacture CI success.

**Administrator grant lifecycle**

- **AC-INTEGRATIONS-SCOPED-CI-RUNS-001.17:** A grant shall identify one
  coordinator task, workspace, workflow, allowed CI Fixup step, target task,
  and task repository, grant no other task or GitHub authority, and be
  creatable and revocable only through the authenticated server API outside
  task MCP. Replacing a grant for the same exact scope shall revoke the prior
  generation and insert its monotonically increasing successor in one
  transaction, and a failed replacement shall roll back the revocation.
  Ordinary agents, sibling or child tasks, unrelated coordinators, and other
  workspaces cannot create, use, or replace a grant through task MCP, and
  revocation shall take effect for subsequent admission without redeploying
  the backend.

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
