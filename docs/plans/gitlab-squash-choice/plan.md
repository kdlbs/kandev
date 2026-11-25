---
created: 2026-10-28
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
legacy_specs: []
---

# Implementation Plan: Respect GitLab squash choice

## Overview

Default interactive and automatic merges currently force squash when the
project merely allows it, overriding an unchecked MR choice. The empty-method
service branch treats `AllowSquash` as intent. An independently authored
real-route/provider regression precedes the minimal service correction in one
sequential work order. A qualification using the real service and PAT client
against a disposable HTTP provider reported two causal failures (default off
and default on with an unchecked MR) and four passing controls. It modeled
GitLab behavior; it was not a live GitLab merge.

## Scope

### In scope

- AC .28-.30: preserve the MR choice for default merges, explicit squash and
  project policy; retain AC .24-.27 source-head and identity guarantees.
- Correct the automation test and production comment that confuse capability
  with a requested choice; document the user-visible behavior in the existing
  GitLab review guidance.

### Out of scope

- New UI, setting, API, interface, metadata, source-branch behavior, or retry.
- Reworking the separate required-SHA repair or unrelated GitLab CI, branch,
  parser and provider behavior. No external merge or protected fixture reuse.

## Technical approach

Keep `Client.MergeMR` and `PATClient` unchanged. In
`apps/backend/internal/gitlab/service_merge.go`, the empty-method case leaves
`squash` false; `PATClient` already omits that false JSON field. Explicit
`squash` retains `AllowSquash` validation and sends true. Revise the misleading
automation comment in `service_mr_automation_auth.go` and the corresponding
test assumption. Preserve scoped credential/host routing, target identity,
messages, source-head read and guarded write, error propagation and no retry.

PAT, environment-token and GLab-backed clients share the PAT wire behavior;
mock/noop clients do not model GitLab's policy resolution. The disposable
provider fixture must assert omission versus true and simulate policy outcome,
not infer behavior from a mock boolean. Workspace-route and automation cases
prove the relevant callers. Unsupported or invalid methods fail before a PUT.

## Tests

Author `TestGitLabSquashChoice` in
`apps/backend/internal/gitlab/squash_choice_test.go` independently. With the
real service and PAT client and a local HTTP provider, cover default-off/MR-off,
default-on/MR-off, default-on/MR-on, always, never and explicit squash; assert
both the request field and effective modeled outcome. Confirm the two unchecked
cases fail causally before production edits and controls pass. Cover invalid and
disallowed methods, current workspace/host/credentials/full project/IID,
messages and source-branch payload, same-head SHA and mismatch with one PUT and
no retry. Exercise the registered interactive route and automation as needed;
existing `merge_head_test.go` remains intact. Map choice cases to AC .28-.30
and identity/head/race controls to AC .24-.27. Correct the automation assertion
without dropping its actual call and scope checks.

## E2E tests

The real registered route through a local provider is the end-to-end boundary
for this backend data correction. Desktop popover and phone drawer share the
same `MRCIPopover` and API helper; the detail confirmation calls that helper
too. No rendered structure or interaction changes. The pure-data parity
exception applies; no new browser test or visual claim is made. Refine this
design before widening scope if implementation discovers a UI change is needed.

## Work orders

- [x] [Task 01: Respect squash choice](task-01-respect-squash-choice.md)

## Verification results

Independent RED failed on the two unchecked default-policy cases, with nine
controls passing. GREEN passed all 11 cases; the owning GitLab package passed
with race detection. The first changed-Go lint identified a test-fixture
staticcheck issue; after its equivalent switch correction, focused GREEN and
five-file changed-Go lint passed. Specification and public-doc validators and
the documentation-coverage preflight passed. The provider was a local HTTP
model; no live GitLab merge was made.

## Risks

- GitLab policy may enforce the merge outcome; a local fixture models its
  documented semantics but cannot prove a particular live host's behavior.
- The SHA guard and existing scoped routing are separate safety contracts and
  must not be weakened while changing the default squash decision.
