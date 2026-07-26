---
id: "20-registration-documentation"
title: "Registration documentation"
status: done
wave: 7
depends_on: ["18-workspace-authentication-ux"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 20: Registration Documentation

## Acceptance

1. Public docs explain PAT/CLI/App choice, known/import/create paths, strict versus shared App
   isolation, private/public meaning, exact callbacks/webhooks, and permissions; the unpublished App
   environment variables and instructions are absent.
2. Architecture and scoped agent guidance no longer claim a singleton deployment App or System
   Settings ownership.
3. Generated documentation coverage metadata is updated using the repository's supported command.

## Verification

```bash
test -z "$(rg -l 'single deployment GitHub App|one deployment App|System Settings > GitHub App|KANDEV_GITHUB_APP' docs/public docs/ARCHITECTURE.md apps/backend/AGENTS.md || true)"
git diff --check
```

## Files Likely Touched

- `docs/public/integrations.md`
- `docs/public/configuration.md`
- `docs/public/feature-status.md`
- `docs/public/coverage.json`
- `docs/ARCHITECTURE.md`
- `apps/backend/AGENTS.md`

## Dependencies

Task 18.

## Inputs

- Spec and new ADR.
- Final endpoint names and UX terminology from Tasks 16 and 18.
- Invoke docs-maintainer; use primary GitHub documentation links already present in the repo.

## Output Contract

Report documentation/metadata changes, verification run, files touched, blockers/risks, and update
this task plus `plan.md` to done.
