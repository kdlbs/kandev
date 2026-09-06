# Server-Owned Scoped CI Runs

- Status: accepted
- Date: 2026-08-30
- Area: backend, protocol, security, GitHub, workflow

## Context

GitHub may not emit a new Actions run when an unchanged pull-request head needs
fresh evidence after an external dependency changes. Repository administrators
can rerun or dispatch workflows, but task agents must not inherit that broad
authority or receive raw installation credentials. Provider calls can also
time out after GitHub accepted them, making naive retry unsafe.

## Decision

Kandev owns a single purpose-built provider operation behind an authenticated
server boundary. An administrator grants one coordinator task authority for
one workspace, target task, workflow, CI Fixup step, and task repository. An
exact-scope replacement atomically revokes the prior generation and creates the
next one. Each request is
then bound server-side to the calling session, current task and step, linked PR,
canonical provider repository, exact unchanged head, trusted source run and
attempt, and a closed evidence policy. The source must be a completed failed
pull-request run.

The backend mints a repository-scoped token from the workspace's verified
GitHub App installation and requires Actions write. It never falls back to PAT,
CLI, user, legacy, or caller-provided credentials. Rerun-failed-jobs is the
first operation. Workflow dispatch is a narrow same-repository `pr_head`
fallback selected from verified workflow metadata and fixed server inputs. The
workflow at the live PR ref must byte-match the trusted base-branch copy;
fork dispatch and unverifiable `current_merge` evidence fail closed.

Provider mutations use a durable two-key ledger: the caller idempotency key is
unique in its actor scope, while source run plus attempt is unique for the
semantic operation. The row is committed before the provider call. Once the
call is marked started, retries reconcile provider state rather than resending.
Before that boundary, a short execution lease allows another caller to resume
the same row after a worker crash; provider start is a compare-and-swap against
the current lease owner. A definitive rate-limit rejection before a mutation
may have been accepted clears the start boundary and persists GitHub's reset
time so the same row can retry only after that instant. A rate-limited read
after provider start retains the marker and resumes read-only reconciliation
after the reset. Mutation timeouts, connection loss, and HTTP 5xx responses
remain ambiguous and reconciliation-only.
Rerun reconciliation accepts only the exact next attempt. GitHub workflow
dispatch is not an available recovery strategy: its branch/tag-only ref API
cannot atomically bind a request to the reviewed commit SHA, so the server
returns `dispatch_ref_unavailable` before mutation. Receipts and audit records
contain only stable identities and classified error metadata; terminal request
state and its audit row commit in one transaction.

## Consequences

- Agents gain one task-bound recovery action, not Actions administration.
- Existing App installations must approve Actions write before the feature can
  succeed; insufficient installations return an explicit permission class.
- Fork PRs are supported by rerunning a verified source attempt but never by
  dispatching an arbitrary ref; same-repository dispatch is also unavailable
  until GitHub provides an immutable conditional-ref operation.
- Merge-ref evidence remains unavailable until GitHub exposes enough runtime
  identity to verify it.
- Ambiguous provider results may require reconciliation instead of immediate
  success, but cannot duplicate a logical run.
- Deployment-local scripts, custom proxies, broad tokens, Docker access, and
  operator-only shortcuts do not implement this platform contract.

## Alternatives rejected

1. Expose `gh run rerun` or `workflow_dispatch` to agents. This leaks broad
   authority and permits arbitrary workflow, ref, and input selection.
2. Use a PAT or host CLI credential. This couples automation to a human and
   expands authority beyond the linked repository.
3. Write a synthetic check result. This fabricates evidence rather than running
   the trusted workflow.
4. Automatically dispatch fork branches when rerun is unavailable. This changes
   event/ref semantics and can execute untrusted workflow content.
