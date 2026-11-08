---
created: 2026-11-04
status: draft
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-03.md
legacy_specs: []
---

# Implementation Plan: GitLab draft merge requests

## Overview

The detail-panel Merge button does not check draft state, so a draft MR
reaches GitLab and is refused (405/406) with a generic failure. Gate every
merge control on draft state, surface GitLab's reason, and add an explicit
"Mark as ready" action. Evidence: km-tenant MR !195 (draft, mergeable
otherwise) was inspected read-only; it is not modified.

## Scope

### In scope

- AC .31-.35: server draft guard, draft-disabled controls, status-mapped
  refusal messages, explicit Mark as ready, no implicit clearing.

### Out of scope

- GitHub draft handling, source-branch removal settings, the existing
  source-head binding (AC .24-.27), and any change to km-tenant data.

## Technical approach

See the [design section](../../specs/integrations/system-design/gitlab-integration-03.md#draft-merge-requests).

## Tests

Table test for title marker stripping (`Draft:`, `draft:`, `[Draft]`,
`(Draft)`, none, marker only mid-title). PAT client test with a disposable
server for the title PUT and re-read. Controller route test. Component tests
for the disabled draft state and the Mark as ready button. Hostile-body test on a 405 merge response; guard tests (PAT and mock) for
no merge PUT on a draft. Map each to AC .31-.35. Also: guard order (invalid method before draft before empty SHA), merge-route status mapping (405/406/409 -> 409, 422 -> 422, GET failures unmapped), Mark as ready fixed messages and 400/502/409 cases, mock `checking` for two `GetMRFeedback` reads then `mergeable` (other reads do not advance it), closed-but-still-draft re-read returning 200, the nil-MR 502, the task-MR link request after the wait, and the 5-read transient wait: hook `reload` outcomes (ok, failed, superseded), no hold on a panel opened on `checking`, manual refresh ends the wait without a hold, and cancellation discarding an in-flight reload.

## E2E tests

One Playwright spec against the mock GitLab client: a draft MR shows a
disabled Merge with the hint; Mark as ready then enables Merge.

## Work orders

- [ ] [Task 01: Draft gating and Mark as ready](task-01-draft-gating-mark-ready.md)

## Risks

- `detailed_merge_status` lags after the title write; the UI must refresh
  rather than assume mergeable.
- New i18n copy needs all seven locales; zh-hant via `pnpm run i18n:zh-hant`.
