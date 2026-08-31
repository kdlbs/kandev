---
title: Scoped coordinator CI runs
status: building
date: 2026-08-30
---

# Scoped coordinator CI runs

## Problem

A task can reach CI Fixup after a shared prerequisite changes without GitHub
creating a new run for the task's unchanged pull-request head. Agents cannot
legitimately recover because rerunning or dispatching Actions may require
repository administration. Giving an agent a token or arbitrary Actions API
access would turn a narrow recovery operation into repository-wide authority.

## Required behavior

Kandev exposes `request_fresh_ci_run_kandev` only to a session whose task owns
an active, administrator-created grant for its current workflow step. The
request names a linked task repository, pull request, exact expected head,
source run and attempt, evidence kind, and idempotency key. The MCP server adds
the calling task and session identities; callers cannot supply repository
owner/name, ref, workflow, dispatch inputs, or credentials.

The backend resolves and checks all authority again. It binds the actor, grant,
workspace, task, workflow, current step, task repository, linked PR, provider
repository identity, exact PR head, and source run before any provider write.
For fork PRs whose Actions run omits the `pull_requests` array, the source run
must match the canonical base repository plus exact head repository, ref, and
SHA. In every case, the source must be a completed failed `pull_request` run.
A changed head or any unlinked, cross-workspace, or ungranted target fails
closed.

## Provider policy

Only the server-owned GitHub App installation may perform the provider call.
Its repository-scoped installation token must report Actions write permission;
PAT, CLI, App-user, legacy, and raw credential paths are forbidden.

The backend reruns failed jobs of the named source attempt first. If GitHub says
that run cannot be rerun, `workflow_dispatch` is allowed only for a same-repo
`pr_head` request whose trusted source workflow has `workflow_dispatch` and
whose workflow belongs to the base repository. Kandev supplies the verified
head branch and reviewed fixed inputs, and requires the workflow file at that
mutable head ref to byte-match the trusted base-branch copy. Fork dispatch is
denied. `current_merge`
fails with `merge_evidence_unavailable` until the provider exposes verifiable
runtime merge-SHA evidence; Kandev never fabricates a merge-ref check.

## Idempotency and races

Every operation is durably claimed before the provider call. A caller key is
unique within the actor/grant scope, and a second unique identity covers the
semantic source run attempt. Concurrent or retried claims return the same
logical operation. Once `provider_call_started_at` is recorded, an interrupted
or ambiguous call is reconciled from GitHub. It is never blindly sent again.
Before provider start, one execution lease owns the transition and an expired
lease can be taken over after a worker crash. Provider start succeeds only for
the current lease owner. A definitive rate-limit response records the reset
time and makes the same row eligible again only after that time. Mutation
timeouts, connection loss, and HTTP 5xx responses remain ambiguous and may
only reconcile.
Rerun reconciliation accepts only the exact next attempt. Dispatch
reconciliation accepts only one new first attempt created at or after the
dispatch call began; zero or multiple candidates remain ambiguous.

## Receipt and audit

Success returns a non-secret receipt containing the Kandev request ID, task,
provider run ID, workflow ID/name/path, provider head repository/ref/SHA,
attempt, operation kind, and evidence kind. Failures expose stable classes such
as `not_authorized`, `head_drift`, `source_run_mismatch`,
`installation_permission_denied`, `provider_rate_limited`,
`provider_unavailable`, `provider_call_ambiguous`, and
`merge_evidence_unavailable`.

Audit rows record actor task/session, workspace, workflow/step, repository/PR,
expected and observed head identities, source run/attempt, operation decision,
provider receipt, failure class, timestamps, and idempotency hashes. They never
contain tokens, App private keys, authorization headers, provider response
bodies, or arbitrary caller input.

## Permissions

Workspace administrators create and revoke exact grants through an authenticated
server API. A grant identifies one coordinator task, workflow, allowed CI Fixup
step, and task repository. Ordinary agents, sibling/child tasks, unrelated
coordinators, and other workspaces cannot use it.

## Acceptance fixtures

- Linked unchanged same-repository PR: rerun eligible and rerun ineligible with
  reviewed `workflow_dispatch` fallback.
- Linked unchanged fork PR: rerun eligible; dispatch fallback denied.
- Empty Actions `pull_requests` association: exact base/head tuple succeeds.
- Head drift, unlinked PR, stale source attempt, cross-workspace target,
  disallowed workflow/input, wrong step, and missing grant fail before a write.
- Concurrent identical calls produce one logical request and at most one
  provider mutation.
- Installation permission failure, provider outage/rate limit, ambiguous send,
  and redacted audit evidence preserve exact stable failure classes.

The first reviewed live acceptance is task
`9349b6e5-a167-4d88-af14-cb355015e3dd` / PR #2841 at
`fc0539307285eb532f97295dc5e05f14cc9fd169`. The second queued acceptance is
task `f4136a59-f2ae-4ef3-b718-24d1118b4115` / PR #2872 at
`eebfe1983b3400a26823ad229469a72f42c2bcb2`. A separately bound third acceptance,
after those two, is task `153cdbbe-beac-47b8-bc06-8dafdcc8ed80` / PR #2868 at
`4b67956a154ddc382ec9084e992f431b423757ad`, using failed source run
`99292477013` (frontend job `99289554250`). None is changed by development or
test execution.

## Non-goals

- Generalized Actions administration, arbitrary workflow/ref/input selection,
  synthetic check statuses, merging, source/history changes, or agent-visible
  credentials.
- Treating a deployment-local proxy, permission customization, or operator
  script as canonical platform behavior. Only the reviewed in-repository
  server capability qualifies for upstream acceptance.
