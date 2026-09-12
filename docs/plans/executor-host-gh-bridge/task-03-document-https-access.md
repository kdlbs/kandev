---
id: "03-document-https-access"
title: "Document executor HTTPS access"
status: complete
wave: 3
depends_on: ["02-runtime-propagation"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.1
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.3
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.4
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.5
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.7
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
---

# Task 03: Document executor HTTPS access

## Summary

Explain the implemented HTTPS fallback and its limits in the existing public integration guide.
Record the completed work-order results without claiming live GitHub access from fake-credential tests.

## In scope

- Update the task credential policy explanation in `docs/public/integrations.md`.
- Explain Local/Worktree scope, available host CLI credentials, explicit token precedence, and preserved Git configuration.
- State that remote executors still need their own credentials and that a CLI login does not grant additional repository permissions.
- Explain that this behavior uses the backend service account's visible CLI configuration.
- Synchronize the authentication design and package results with the final implementation.

## Out of scope

No new documentation page, UI copy, screenshots, or translation keys. Result sections are
updated when review validation changes the recorded implementation behavior.

## Acceptance

1. Public instructions describe the completed behavior and retain the managed-mode isolation contract.
2. Documentation validators pass, and all work-order results contain actual command outcomes.

## Verification

From the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `docs/public/integrations.md`
- `docs/specs/integrations/requirements/github-authentication.md`
- `docs/specs/integrations/system-design/github-authentication-01.md`
- `docs/specs/integrations/system-design/github-authentication-02.md`
- `docs/plans/executor-host-gh-bridge/plan.md`
- This package's work-order Results sections.

## Dependencies

Tasks 01 and 02.

## Risks

A desktop terminal login can differ from credentials visible to a backend service.
The guide must not promise that a token can authenticate SSH or access every repository.

## Parallelism

`sequential`

## Inputs

- The implemented behavior and recorded results from Tasks 01 and 02.
- The existing public integration guide's task credential policy section.
- `/docs-maintainer` and `/simple-english`.

## Results

- Updated `docs/public/integrations.md` with the Local and Worktree host `gh`
  fallback, backend service-account scope including the required `gh auth login`
  account, host-specific explicit token precedence, preserved Git configuration,
  remote-executor limits, and permission boundary.
- Synchronized the implementation plan and all work-order statuses with the
  completed production and regression work, including the strict resolver,
  configure-boundary composition, marker ownership, and profile-aware probing.
- `node --test scripts/validate-public-docs.test.mjs` passed with 62 tests.
- `node scripts/validate-public-docs.mjs` validated 46 published docs pages.
- `python3 scripts/list-docs.py validate` passed for 264 decisions and 818 specifications.
- `python3 scripts/lint-spec-files.test.py` passed all 36 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
