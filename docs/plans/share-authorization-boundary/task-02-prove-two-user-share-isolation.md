---
id: "02-prove-two-user-share-isolation"
title: "Prove two-user share isolation"
status: done
wave: 2
depends_on:
  - "01-enforce-share-service-authorization"
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

# Task 02: Prove Two-User Share Isolation

## Summary

Add an auth-project Playwright test for the complete HTTP route matrix. The test
uses an admin context only for setup, then separate authenticated member A and
member B contexts with real production route wiring.

## In scope

- Start the auth project with a file-specific database.
- Invite member A and member B with separate browser contexts.
- Create member B's workspace, task, completed session, transcript marker, and
  share.
- Prove that member A cannot preview, publish, list, or revoke B's share data.
- Prove that a mismatched task-session pair returns 404.
- Prove that B can still use the owner paths after A's denied requests.
- Prove that A's denied revoke does not revoke B's share.

## Out of scope

- New UI interactions, screenshots, or mobile coverage.
- Real GitHub network access.
- Additional authorization audits outside the share routes.

## Acceptance

- Every foreign or mismatched share request made by member A returns 404 without
  the transcript marker or share metadata.
- No denied publish creates a share. No denied revoke changes B's active share.
- B can preview, list, publish, and revoke through the same assembled routes.

## Verification

```bash
cd apps/web
pnpm e2e:raw --project=auth tests/auth/share-authorization.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/auth/share-authorization.spec.ts`
- If a reusable account helper is required, `apps/web/e2e/helpers/auth.ts`
- If context-bound seed support is required, `apps/web/e2e/helpers/api-client.ts`

## Dependencies

- Task 01 must complete first.

## Risks

- Test-only seed routes must use B's authenticated context.
- The test must use the mock GitHub client. It must not call the network.
- File-specific database cleanup must restore the normal backend fixture.

## Parallelism

`sequential`

## Inputs

- Task 01 service and HTTP behavior.
- `apps/web/e2e/helpers/auth.ts`.
- `apps/web/e2e/tests/auth/auth-lifecycle.spec.ts` two-user patterns.
- The auth-project instructions in `apps/web/e2e/README.md`.

## Results

Implemented an auth-project Playwright regression against the assembled
production routes. The admin context only creates two member accounts. Member
B seeds the member-owned workspace, mock GitHub connection, task, completed
session, and transcript through the owner context. Member A receives `404` for
preview, publish, list, and revoke, including a mismatched task-session pair,
without transcript or share metadata. Member B can preview, publish, observe
the active share after the denied revoke, and revoke it.

Verification:

```bash
cd apps/web && pnpm e2e:raw --project=auth tests/auth/share-authorization.spec.ts
1 passed
```
