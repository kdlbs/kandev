---
id: "01-classify-session-challenges"
title: "Classify session challenges and preserve PAT errors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/requirements/auth.md"
related_spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 01: Classify session challenges and preserve PAT errors

## Intent

Prevent third-party provider 401 responses from invoking the global Kandev login redirect while
preserving the redirect for a real Kandev session challenge. Prove that an invalid GitHub PAT save
remains visible and retryable in the existing connection dialog.

## Acceptance

1. `fetchJson` invokes `onUnauthorized` only for a 401 carrying Kandev's
   `WWW-Authenticate: Bearer` challenge and still throws `ApiError` for both challenged and
   unchallenged failures.
2. An unchallenged provider 401 preserves its parsed error message for the calling integration UI.
3. A PAT-only GitHub connection save failure leaves the dialog open, retains the submitted token,
   displays the error, and does not call the saved callback.

## TDD

1. Add the challenged/unchallenged 401 unit cases and the PAT-only dialog rejection case.
2. Run them against current code and confirm the unchallenged-401 case fails because
   `onUnauthorized` is called.
3. Implement the smallest shared-client classification change.
4. Rerun the focused tests and refactor only if needed.

## Files Likely Touched

- `apps/web/lib/api/client.ts`
- `apps/web/lib/api/client.test.ts`
- `apps/web/components/github/github-connection-dialog.test.tsx`

## Dependencies

None.

## Parallelism

`sequential` — Task 02 depends on this behavior.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/api/client.test.ts components/github/github-connection-dialog.test.tsx
```

## Inputs

- `docs/specs/auth/requirements/auth.md` — session challenge versus provider failure behavior.
- `docs/specs/integrations/requirements/github-authentication.md` — invalid PAT save behavior.
- `apps/backend/internal/auth/httpmw/middleware.go` — authoritative Kandev session challenge.
- `apps/web/src/main.tsx` — global unauthorized callback.

## Output Contract

Verification record:

- RED: before the challenge gate, the focused provider-401 test failed because
  `onUnauthorized` was invoked for an unchallenged 401.
- Changed files: `apps/web/lib/api/client.ts`,
  `apps/web/lib/api/client.test.ts`, and
  `apps/web/components/github/github-connection-dialog.test.tsx`.
- Final focused result: 14 tests passed; the managed provider-auth E2E spec passed all 5
  scenarios; web typecheck and changed-file lint passed.
- Follow-up review fix: `apps/backend/internal/backendapp/middleware.go` now exposes
  `WWW-Authenticate` to split-origin browsers, with a regression test in
  `apps/backend/internal/backendapp/middleware_test.go`.
- Remaining risk: session redirect classification depends on the backend emitting the
  `WWW-Authenticate: Bearer` challenge; same-origin and split-origin paths now both preserve
  that signal.
