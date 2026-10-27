---
created: 2026-10-27
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
legacy_specs: []
---

# Implementation Plan: GitLab merge SHA compatibility

## Overview

Restore the existing interactive and automated GitLab merge on hosts requiring
an API SHA. First establish an independent failing registered-route regression,
then implement the shared provider-boundary read and guarded write in one
sequential work order. The supplied controlled reproduction used the registered
workspace route, real workspace credential resolution, service and PAT client
against a disposable provider: the SHA-required case returned provider 400 and
Kandev 502 with `{}`, while legacy and exact-SHA provider controls succeeded.
It is not evidence of a live GitLab mutation.

## Scope

### In scope

- AC .24-.27: same-workspace, host, project and IID source-head binding;
  failure without a write on unusable head; mismatch without a write retry.
- PAT and embedded GLab, including service and automation consumers, and
  existing merge method, squash and message behavior.

### Out of scope

- UI, freshness claims, source-branch deletion, permissions, administrative
  configuration, retries after writes, unrelated GitLab behaviors and
  squash-default policy changes.

## Technical approach

Keep `Client.MergeMR` unchanged. In `internal/gitlab/pat_client_actions.go`,
read the raw single-MR response using the same `PATClient` and target identity;
use top-level `sha` rather than `convertRawMR`'s diff-preferring `HeadSHA`.
Reject failed or blank reads before PUT and include the obtained value in the
existing request. Propagate provider errors. `GLabClient` embeds PATClient;
`Service.mergeMRWithClient` already supplies its workspace-selected client to
interactive and automation actions. Preserve existing response/error mapping.

| Client and identity | Behavior | Evidence / unsupported response |
| --- | --- | --- |
| PAT, workspace host and full target project/IID | Read top-level source head, send on merge | Registered route and PAT contract tests; failed or blank head stops before write |
| GLab, selected host and embedded PAT | Same provider operation | Embedded-client test without real `glab` or credentials |
| Mock/noop clients | Existing service contracts remain | Existing service/automation tests; no wire SHA contract |

## Tests

Independently author `TestWorkspaceMergeMRRequiredSHA` in an owning GitLab Go
test using registered `PUT /api/v1/gitlab/mrs/merge`, actual workspace config
and secret resolution, real service and PAT client, and disposable httptest
provider. It must fail before production edits specifically on missing SHA,
with provider-required and legacy controls. Prove a distinct top-level source
SHA wins over stale diff head, base and fork target; exact project/IID, host,
workspace credential, method, squash and message survive; a moving head
returns a provider 409 with one merge PUT and no automatic retry; failed,
missing and blank reads issue zero PUTs. Test direct PAT errors and GLab's
embedded operation; retain existing success/failure and automation tests.
The route may map upstream 409 through its existing error response; assert the
provider status/error at the client boundary and the route's failure, not a new
HTTP status contract. Map these tests to AC .24-.27.

## E2E tests

The registered real-route regression exercises the existing native action's
backend path and both provider behaviors. No browser suite or UI modification
is needed for the wire-field repair.

## Work orders

- [x] [Task 01: Bind merge to source head](task-01-bind-merge-head.md)

## Verification results

The independently authored registered-route regression failed before the fix
because the required-SHA provider received no SHA; the legacy and exact-SHA
controls passed. The same route regression passed after the fix. The owning
GitLab package, its race run and canonical changed-Go lint passed using the
commands in the work order. Specification catalog, specification lint,
documentation coverage preflight and diff checks passed. Provider behavior was
simulated by disposable HTTP servers; no live GitLab mutation was performed.

## Risks

- A read/write race is expected; GitLab's SHA comparison rejects it. A fresh
  read is not proof that the user reviewed that commit.
- The added read can fail where the old write might have succeeded; failing
  closed is required. General MR diff projection must remain unchanged.
- Concurrent work can allocate the same acceptance IDs; reconcile the owning
  documents and stable IDs before integration.
