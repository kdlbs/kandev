---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-001
created: 2026-08-30
owners:
  - kandev
---

# Scoped coordinator CI run system design

## Ownership and mapping

`REQ-INTEGRATIONS-SCOPED-CI-RUNS-001` maps its acceptance criteria as follows:
grant admission and closed-request identity resolution (AC-001.1 through
AC-001.4) map to the admission flow; App-installation credential policy
(AC-001.5 through AC-001.7) maps to credential resolution; rerun-first and
typed evidence policy (AC-001.8 through AC-001.12) map to the evidence and
provider operations; durable claims and reconciliation (AC-001.13 and
AC-001.14) map to the request state machine; receipt and audit guarantees
(AC-001.15 and AC-001.16) map to receipt and audit assembly; and the
administrator grant lifecycle (AC-001.17) maps to the grant management
controller. Integration services own the whole capability. Task, workflow, and
MCP systems consume it.

## Components and responsibilities

- `internal/github` owns the scoped CI tables (`service_ci_run_grant.go`,
  `store_ci_run_*`), the App credential resolver with scoped Actions purposes
  (`app_credential_provider.go`, `auth_resolver.go`), the Actions operations and
  failure classification (`token_client_actions.go`,
  `service_ci_run_provider_errors.go`), the request state machine
  (`service_ci_run_request.go`), receipt and audit assembly
  (`service_ci_run_audit.go`, `service_ci_run_receipt.go`), and the
  authenticated grant controller (`controller_ci_run_grant.go`).
- `internal/mcp/server` exposes `request_fresh_ci_run_kandev` in task mode with
  injected caller task and session identity and a closed
  no-additional-properties schema; the caller can never supply actor identity,
  repository coordinates, refs, workflow identities, inputs, or credentials.
- `internal/backendapp` composes the service, controller, and MCP tooling and
  applies the required-store schema contracts.

No token, App JWT or private key, secret-store pointer, lease, socket, or
generic Actions endpoint crosses any of these boundaries toward the agent: the
receipt and audit surfaces are the only agent-visible outputs.

## Data and contracts

The durable layer keeps three replayable SQLite tables: coordinator grants
(actor task, workspace, workflow, allowed step, target task, repository,
generation), the CI run request ledger (caller idempotency key unique within
actor scope, semantic source-attempt identity, status, provider-start marker,
lease owner, retry evidence), and redacted audit events. Grant replacement for
the same exact scope is one transaction that revokes the prior generation and
inserts the successor. A terminal request transition commits or rolls back with
its terminal audit row. The `github_task_prs` association and task-repository
attachment are authoritative for target resolution; task titles, prompts,
agent profiles, workflow-step names, caller metadata, and repository remotes
are never authorization evidence.

The request contract is closed: `task_id`, `repository_id`, `pr_number`,
`expected_head_sha`, `expected_workflow_step_id`, `source_run_id`,
`expected_source_attempt`, `evidence_kind`, and `idempotency_key`. Frontmatter
of the response is a receipt with request identity, idempotency status,
strategy, repository and pull request, expected and observed head, provider
head repository, ref, and SHA, workflow identity, source and result runs and
attempts, event, execution or merge identity only when provider-proven,
evidence kind and verdict, non-secret App principal, provider request
identity, and timestamps.

## Control flow

Admission resolves the caller task, session, workspace, workflow, current
step, task repository, active pull request association, canonical repository,
and grant from durable state, rejects unknown request fields, and requires the
named expected step to equal both the grant's allowed step and the target
task's live step in the same workflow. Credential resolution then surfaces
only the verified GitHub App installation with the scoped Actions permission
purpose. Evidence policy verifies the pull request head and source run
identities before any write, preferring the provider association and falling
back to the exact base-plus-head tuple only for an empty association list.
Execution claims the request durably, takes the transition lease, records the
provider-start marker, and performs rerun-first. Once a provider call may have
been sent, every retry reconciles read-only: reconciliation accepts only the
exact next attempt, and a definitive rerun-ineligible rejection records the
typed immutable-ref denial without any dispatch.

## Failure and recovery

Provider failures classify into stable classes: rate limits record the
provider reset evidence and defer; authorization failures map to installation
classes; mutation ambiguity maps to a reconcilable ambiguous state that never
resends; and rerun-ineligible maps to `dispatch_ref_unavailable`. `current_merge`
fails closed with `merge_evidence_unavailable`. Fork dispatch is denied. A
worker crash between claim and provider start is recovered by lease takeover
after expiry; a crash after provider start reconciles from GitHub. Rollback of
a failed grant replacement leaves the prior grant active.

## Persistence

All three tables belong to the SQLite required-stores and conformance sets.
Schema changes are replayable across SQLite and PostgreSQL, and workspace
cleanup removes a workspace's grants and audit rows. No provider payload,
credential material, or caller-supplied free text is persisted; audit rows are
redacted to the non-secret identity set.

## Verification

Provider behavior is exercised through HTTP fixtures that assert authorization
headers and token confinement. Race-enabled SQLite tests cover grant
replacement, concurrent claims, semantic uniqueness, and terminal atomicity.
Composition tests bind the MCP tool and controller into `backendapp`. The two
reviewed live consumer runs are observed only; they are never triggered by
development or test execution.
