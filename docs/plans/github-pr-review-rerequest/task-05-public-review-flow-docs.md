---
id: "05-public-review-flow-docs"
title: "Public review-flow docs"
status: done
wave: 3
depends_on:
  - "01-backend-review-request"
  - "02-frontend-dismissed-review-action"
  - "03-mobile-github-pr-review"
plan: "plan.md"
spec: "../../specs/ui/requirements/github-pr-review-actions.md"
---

# Task 05: Public review-flow docs

## Acceptance

- Public session/review docs explain that an open GitHub PR can re-request a
  dismissed reviewer from the linked PR detail panel.
- Docs name GitHub permission as authoritative and mention the phone **Review**
  destination without documenting internal implementation details.
- Public-doc validators pass.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/sessions-and-review.md`

## Dependencies

Tasks 01, 02, and 03.

## Inputs

- Shipped behavior and the feature spec.
- Existing GitHub in-app PR review paragraph under
  `docs/public/sessions-and-review.md`.
- Docs-maintainer skill.

## Output contract

Report summary, files changed, commands/results, blockers, risks, and set only
this task file's status to `done`. Do not edit `plan.md`.
