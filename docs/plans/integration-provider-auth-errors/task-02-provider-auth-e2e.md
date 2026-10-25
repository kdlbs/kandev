---
id: "02-provider-auth-e2e"
title: "Prove provider page behavior end to end"
status: done
wave: 2
depends_on: ["01-classify-session-challenges"]
plan: "plan.md"
spec: "../../specs/auth/requirements/auth.md"
related_spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 02: Prove provider page behavior end to end

## Intent

Exercise the four provider data pages and GitHub PAT settings through the browser so a future
regression cannot turn an integration credential failure into a Kandev login redirect.

## Acceptance

1. GitHub, GitLab, Jira, and Linear pages each remain on their own route and show a provider loading
   error after their primary data endpoint returns an unchallenged 401.
2. GitHub settings retain an invalid replacement PAT, keep the connection surface open, show the
   backend error, and keep the existing connection active.
3. The focused production-build E2E spec passes without adding mobile-specific UI or changing page
   composition.

## TDD

1. Add the browser scenarios using existing integration configuration helpers and route
   interception for sanitized provider 401 responses.
2. Run the managed spec and correct fixture or selector assumptions exposed by the real browser
   surface before interpreting a failure as a product regression.
3. Rerun after Task 01 and confirm the provider routes and error UI remain visible.

## Files Likely Touched

- `apps/web/e2e/tests/integrations/provider-auth-errors.spec.ts`

## Dependencies

Task 01 complete.

## Parallelism

`sequential` — validates Task 01 through real SPA routing.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/integrations/provider-auth-errors.spec.ts
```

## Inputs

- `apps/web/e2e/helpers/api-client.ts` — GitHub, GitLab, Jira, and Linear configuration helpers.
- `apps/web/e2e/pages/github-auth-settings-page.ts` — GitHub connection dialog selectors.
- The existing provider page search hooks and list error renderers under `apps/web/app/` and
  `apps/web/components/{github,gitlab,jira,linear}/`.

## Output Contract

The first managed run exposed two test assumptions (GitHub personal identity requires an
App-backed workspace seed, and the PAT input must be selected by its textbox role); it did not
reach a product redirect failure. After correcting those assumptions, the final managed runner
passed all five scenarios. No failure artifacts remain relevant, and the remaining risk is limited
to provider-specific backend error-body wording outside the sanitized responses covered here.
