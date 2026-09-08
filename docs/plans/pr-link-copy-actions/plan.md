---
created: 2026-08-31
status: implemented
requirements:
  - ../../specs/integrations/requirements/pr-link-copy-actions.md
system_design:
  - ../../specs/integrations/system-design/pr-link-copy-actions.md
legacy_specs: []
---

# Implementation Plan: Pull request link copy actions

## Overview

Implement issue #3179 through the existing GitHub feedback and shared
change-request detail paths. Preserve provider comment permalinks at the
backend boundary, map them into the provider-neutral detail model, and add
localized copy actions that work on desktop and on the phone Review surface.

The change is read-only from the provider's perspective. It needs no new API,
database migration, polling, or task-state mutation.

## Confirmed diagnosis

The linked PR URL is already available to `PRDetailContent` through the live PR
model and the persisted `TaskPR.pr_url`. The shared detail header currently
renders the PR title, state, number, and provider actions but has no copy
control.

GitHub review and conversation comments are already fetched together. The
backend raw comment types and the normalized `PRComment` omit `html_url`, so
the frontend cannot produce a canonical comment permalink. The shared comment
rows also have no link action. Both desktop/tablet and phone use the same
`ChangeRequestDetail`, so one shared implementation covers the surfaces.

## Delivery sequence

1. Extend the GitHub raw, backend, mock, and frontend feedback types with the
   provider comment URL. Add converter and transport regressions for review and
   conversation comments.
2. Extend the provider-neutral comment model and add a shared copy affordance
   using `copyToClipboard`, localized tooltip/accessibility text, transient
   success state, and existing responsive row geometry.
3. Map GitHub comment URLs into the shared detail and add component tests for
   exact values, omitted URLs, and closed or merged PRs.
4. Add desktop and phone Playwright coverage using the existing mock feedback
   seed and Review navigation. Assert clipboard contents, confirmation,
   touch geometry, and no document horizontal overflow.

## Implementation Waves

Wave 1:

- [x] [Task 01: Provider URLs and shared copy actions](task-01-provider-urls-and-copy-actions.md)

Execution is sequential in the primary conversation. No subagent delegation is
planned or authorized.

## Verification

Run focused backend and frontend tests first:

```bash
cd apps/backend
go test ./internal/github

cd ../
pnpm --filter @kandev/web test -- --run \
  components/github/pr-detail-panel-mapping.test.ts \
  components/integrations/change-request-detail.test.tsx \
  components/github/pr-detail-panel.test.ts
```

Then run the focused desktop and mobile browser scenarios:

```bash
cd apps/web
pnpm e2e:run tests/pr/pr-detail-copy-links.spec.ts
pnpm e2e:run --project=mobile-chrome tests/pr/mobile-pr-detail-copy-links.spec.ts
```

The implementation task must also run the repository's applicable formatting,
lint, typecheck, and i18n checks after the focused tests pass.

## Verification results

- Backend unit and race tests pass: `go test ./internal/github -count=1` and
  `go test -race ./internal/github -count=1`.
- Frontend focused tests pass: 43 tests across the shared detail, GitHub panel,
  and GitHub mapping test files.
- Desktop and Pixel 5 E2E copy-link scenarios pass, including exact clipboard
  values, closed-PR availability, touch targets, and horizontal-overflow checks.
- `make -C apps/backend lint` reports 0 issues. Web typecheck, lint, i18n
  checks, specification lint, and diff checks pass.

## Risks

- The copy action must preserve the provider's complete anchor. Composing a URL
  from an ID could silently break review-comment navigation.
- A hover-only implementation would fail on phone and keyboard paths. The
  coarse-pointer path must keep actions visible, and focus must reveal them on
  fine-pointer paths.
- Adding a header control can affect the already-sensitive PR header wrapping.
  It must stay in the identity row and retain the existing mobile minimum hit
  size.
- GitHub feedback fixtures currently do not type or seed comment URLs. The
  mock helper and raw payload tests must be updated before E2E can prove the
  clipboard contract.

## Documentation impact

No public CLI, configuration, API, executor, or workflow documentation changes
are required. The feature is discoverable through its localized tooltip and
copied confirmation.
