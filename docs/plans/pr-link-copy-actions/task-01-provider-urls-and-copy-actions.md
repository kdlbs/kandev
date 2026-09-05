---
id: "01-provider-urls-and-copy-actions"
title: "Provider URLs and shared copy actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001
acceptance_criteria:
  - AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.1
  - AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.2
  - AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.3
  - AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.4
  - AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.5
  - AC-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001.6
system_design: "../../specs/integrations/system-design/pr-link-copy-actions.md"
---

# Task 01: Provider URLs and shared copy actions

## Outcome

Users can copy the exact linked GitHub pull-request URL from the shared detail
header and the exact permalink from every URL-bearing conversation or review
comment row on desktop, tablet, and phone.

## In scope

- Preserve GitHub `html_url` for review and conversation comments through gh,
  PAT, mock, backend, and frontend feedback mappings.
- Add the optional provider-neutral comment URL field and shared localized copy
  action to the header and feedback rows.
- Keep the existing PR URL fallback, closed/merged behavior, single-scroll
  detail geometry, keyboard access, and phone touch targets.
- Add unit/component, backend transport/converter, and desktop/mobile Playwright
  regression coverage.

## Explicit exclusions

- Markdown link formatting.
- New persistence, endpoints, polling, or provider mutations.
- GitLab or Azure DevOps adapter changes.
- A parallel mobile or provider-specific detail component.

## Acceptance conditions

- The existing feedback response exposes both GitHub comment URL forms and the
  shared model copies those exact strings, with no action for an omitted URL.
- Header and comment-row copy actions use the repository clipboard utility,
  expose localized tooltip and accessible labels, show transient success only
  after a successful copy, and preserve retry on failure.
- Desktop and mobile tests prove PR, review-comment, and conversation-comment
  clipboard values, closed/merged availability, phone visibility without hover,
  44-pixel targets, and no document horizontal overflow.

## Verification

```bash
cd apps/backend
go test ./internal/github

cd ../
pnpm --filter @kandev/web test -- --run \
  components/github/pr-detail-panel-mapping.test.ts \
  components/integrations/change-request-detail.test.tsx \
  components/github/pr-detail-panel.test.ts

cd web
pnpm e2e:run tests/pr/pr-detail-copy-links.spec.ts
pnpm e2e:run --project=mobile-chrome tests/pr/mobile-pr-detail-copy-links.spec.ts
```

Also run the applicable frontend i18n, typecheck, lint, formatting, and
repository validation commands. Use TDD: add a failing regression before each
production change, then refactor only after the focused tests pass.

## Likely files

- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/client_helpers.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/mock_controller.go`
- `apps/backend/internal/github/*_test.go` for comment conversion and transport
  regressions
- `apps/web/lib/types/github.ts`
- `apps/web/components/github/pr-detail-panel.tsx`
- `apps/web/components/integrations/change-request-detail.tsx`
- `apps/web/components/integrations/change-request-detail-header.tsx`
- `apps/web/components/integrations/change-request-detail-feedback.tsx`
- `apps/web/components/integrations/change-request-detail-comments.tsx`
- `apps/web/components/integrations/change-request-detail.test.tsx`
- `apps/web/components/github/pr-detail-panel-mapping.test.ts`
- `apps/web/components/github/pr-detail-panel.test.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/integrations.json`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/pr/pr-detail-copy-links.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-detail-copy-links.spec.ts`

## Dependencies

None. The existing GitHub PR feedback endpoint and shared mobile Review
composition provide the required data and entry points.

## Results

- RED: backend gh/PAT/converter tests failed because `html_url` was empty, and
  the shared copy-action test failed because no copy controls existed.
- GREEN: GitHub raw and domain comment URLs now survive gh, PAT, and mock
  feedback responses. The shared detail maps them into localized copy buttons
  with exact clipboard values, transient success only after a successful
  write, and no action for missing URLs.
- REFACTOR: the copy control is shared by the header and comment rows; labels
  follow locale changes, comment actions reveal on fine-pointer hover/focus,
  and they remain visible at coarse pointers with 44px targets. The public PR
  URL fallback avoids API URLs, and whitespace around provider URLs is
  normalized before rendering.
- Verification: backend unit and race tests, `make -C apps/backend lint`,
  43 focused frontend tests, desktop and Pixel 5 E2E scenarios, web typecheck,
  lint, i18n checks, specification lint, and diff checks all pass.
