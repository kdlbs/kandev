---
created: 2026-08-25
status: done
requirements:
  - REQ-AUTH-AUTH-001
  - REQ-AUTH-PUBLIC-SHARE-LINKS-001
system_design:
  - ../../specs/auth/system-design/public-share-links.md
legacy_specs: []
---

# Implementation Plan: Share Authorization Boundary

## Overview

Before this implementation, the share service read task-session data through
the raw task repository. Therefore, the caller identity did not limit preview
and list operations. The same service omission also left publish and revoke
dependent on a GitHub authorization check.

This plan adds the task-service authorization boundary first. Then it adds an
auth-project regression test against the assembled auth-enabled server with an
admin setup context and two member contexts: attacker A and owner B.

## Scope

### In scope

- Authorize every user-facing share service method before sensitive reads or
  mutations.
- Authorize the task ID, the session ID, and their relationship on nested
  share routes.
- Authorize revoke before the revoked-state shortcut and provider call.
- Return `404 Not Found` for foreign, missing, and mismatched objects.
- Preserve the auth-disabled synthetic-identity behavior.
- Add service, handler, and auth-project regression tests.

### Out of scope

- Changes to snapshot content or redaction rules.
- Changes to the `task_shares` schema or share lifecycle.
- Changes to GitHub credential selection or provider authorization.
- Changes to the share dialog or responsive UI.
- Hosted share storage, workspace sharing, and team-owned shares.

## Confirmed root cause (pre-change)

`backendapp` passed `repos.Task` to `share.Provide`. The share service had no
task access authorizer. `PreviewSnapshot` and `ListBySession` used
caller-supplied session IDs without an ownership check. The HTTP handlers also
ignored the task ID in nested routes.

A focused temporary test used a real SQLite task repository and a member
identity. The test read a foreign transcript marker through `PreviewSnapshot`.
It also created, listed, and revoked a foreign share through the raw service.
The temporary test passed and was then removed.

Before this implementation, the assembled GitHub backend blocked foreign
publish and active-share revoke. This provider check was not a valid service
authorization boundary. Preview and list never reached it.

## Technical approach

### Share service boundary

- Add a narrow authorizer interface in `internal/task/share`.
- Require `AuthorizeTaskSessionAccess` and `AuthorizeSessionAccess`.
- Pass the authorizer through `share.New` and `share.Provide`.
- Keep `TaskReader` for snapshot construction after authorization succeeds.
- Change nested service methods to accept both `taskID` and `sessionID`.
- Run the task-session pair check before task, session, message, share, or
  provider access.
- In `RevokeShare`, load the share row only to resolve its session ID. Then
  authorize before the revoked-state shortcut, provider call, or row update.

### HTTP contract

- Read both route parameters in create, preview, and list handlers.
- Pass both IDs to the share service.
- Map task-service not-found sentinels to the existing share `404` response.
- Keep provider and snapshot errors on their current response paths.

### Startup wiring

- `backendapp` creates the share handlers. Pass `taskSvc` as the authorizer.
- Keep `repos.Task` as the raw snapshot reader.
- Keep the GitHub workspace authorizer as defense in depth.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-AUTH-AUTH-001.7` | Service tests deny foreign and mismatched task-session access before dependencies run. |
| `AC-AUTH-AUTH-001.7` | HTTP tests return 404 and omit transcript, share, and provider details. |
| `AC-AUTH-PUBLIC-SHARE-LINKS-001.2` | Existing preview tests remain green for an authorized caller. |
| `AC-AUTH-PUBLIC-SHARE-LINKS-001.4` | Existing publish and revoke tests remain green for an authorized caller. |

## E2E tests

Add `apps/web/e2e/tests/auth/share-authorization.spec.ts` to the isolated auth
project. The test uses the admin context only to create two member accounts.
Member B owns the task, session, transcript marker, and share. Member A
receives 404 for preview, publish, list, and revoke, including a mismatched
task-session pair. The test also proves that B's share remains active.

This flow covers `AC-AUTH-AUTH-001.7` at the assembled HTTP boundary.

## Work orders

- [x] [Task 01: Enforce share service authorization](task-01-enforce-share-service-authorization.md)
- [x] [Task 02: Prove two-user share isolation](task-02-prove-two-user-share-isolation.md)

## Verification results

Task 01 and Task 02 are complete. The handler regressions and assembled
member-to-member auth-project regression pass.

Remediation verification:

```text
cd apps/backend && go test -tags fts5 ./internal/task/share -count=1
145 passed

cd apps/web && pnpm e2e:raw --project=auth tests/auth/share-authorization.spec.ts
1 passed

make -C apps/backend lint
0 issues

python3 scripts/lint-spec-files.py --all
All specification files passed.
```

## Risks

- A late authorization check can read transcript data before it returns an
  error.
- The revoked-state shortcut can preserve a share-ID existence oracle.
- Broad error mapping can turn provider or database errors into false 404
  responses.
- Constructor changes can leave a test or alternate startup path without an
  authorizer.
- An E2E fixture can bypass the same production boundary that the test must
  prove.
