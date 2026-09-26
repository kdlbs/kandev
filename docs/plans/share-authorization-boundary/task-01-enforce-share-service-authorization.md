---
id: "01-enforce-share-service-authorization"
title: "Enforce share service authorization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AUTH-AUTH-001
  - REQ-AUTH-PUBLIC-SHARE-LINKS-001
acceptance_criteria:
  - AC-AUTH-AUTH-001.7
  - AC-AUTH-PUBLIC-SHARE-LINKS-001.2
  - AC-AUTH-PUBLIC-SHARE-LINKS-001.4
system_design:
  - ../../specs/auth/system-design/public-share-links.md
---

# Task 01: Enforce Share Service Authorization

## Summary

Add the task-service access boundary to every user-facing share method. Reject
foreign and mismatched objects before transcript reads, provider calls, or
share mutations.

## In scope

- Add the narrow share access-authorizer interface.
- Pass the authorizer through constructors and production startup wiring.
- Pass both nested route IDs into create, preview, list, and backend probes.
- Authorize revoke through its stored session ID.
- Map ownership denials to `404 Not Found`.
- Map authorization infrastructure failures in every share handler to a fixed
  generic `500 Internal Server Error` and log the wrapped cause server-side.
- Add focused service and HTTP regression tests before production changes.
- Preserve authorized, synthetic, and internal caller behavior.

## Out of scope

- Browser E2E coverage.
- Frontend behavior or copy.
- Snapshot, persistence, and provider-credential changes.

## Acceptance

- Every nested share method authorizes the task-session pair before it reads a
  dependency.
- Revoke authorizes before the revoked-state shortcut, provider call, and row
  update.
- Preview, both publish authorization stages, and revoke do not expose an
  authorization infrastructure error or misclassify it as a provider failure.
- Foreign and mismatched requests return 404. Authorized and auth-disabled
  requests keep their current successful behavior.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/task/share -count=1
```

## Files likely touched

- `apps/backend/internal/task/share/service.go`
- `apps/backend/internal/task/share/http.go`
- `apps/backend/internal/task/share/provider.go`
- `apps/backend/internal/task/share/service_authz_test.go`
- `apps/backend/internal/task/share/http_authz_test.go`
- `apps/backend/internal/task/share/service_test.go`
- `apps/backend/internal/task/share/http_test.go`
- `apps/backend/internal/backendapp/services.go`

## Dependencies

None.

## Risks

- Create, preview, list, and backend-access methods must start with the pair
  authorization check.
- Revoke must read one share row before it can resolve the owning session.
- Not-found mapping must not hide unrelated storage or provider errors.

## Parallelism

`sequential`

## Inputs

- `AC-AUTH-AUTH-001.7` in the auth requirements.
- The authorization boundary in the public-share system design.
- `task.Service.AuthorizeTaskSessionAccess` and
  `task.Service.AuthorizeSessionAccess`.
- Existing share service and HTTP tests.

## Results

Implemented the task-service authorization boundary for preview, publish,
backend access, list, and revoke operations. Nested share routes now pass and
authorize both task and session IDs. Foreign and mismatched access maps to the
existing 404 response, and revoke authorizes before the revoked shortcut,
provider delete, or local mutation. The HTTP handlers now map authorization
infrastructure failures to a fixed generic 500 response and log the wrapped
cause without exposing it to callers.

Verification:

```text
cd apps/backend && go test -tags fts5 ./internal/task/share -count=1
145 passed

cd apps/backend && go test -tags fts5 -run '^$' ./internal/backendapp
package compiled; no tests selected

make -C apps/backend lint
0 issues
```
