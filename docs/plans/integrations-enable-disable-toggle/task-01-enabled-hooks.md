---
id: "01-enabled-hooks"
title: "Enabled hooks for Azure DevOps, GitHub, GitLab"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/enable-disable-toggle.md"
---

# Task 01: Enabled hooks for Azure DevOps, GitHub, GitLab

- **Acceptance:**
  1. `useAzureDevOpsEnabled()`, `useGitHubEnabled()`, `useGitLabEnabled()`
     each exist, default to `enabled: true` on first read, persist via
     `setEnabled`, and broadcast their own sync event — one-line wrappers
     over `useIntegrationEnabled`, mirroring `useJiraEnabled()`.
  2. Each hook has a unit test asserting the default-true value and a
     persisted round-trip via `setEnabled`.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- hooks/domains/azure-devops/use-azure-devops-enabled hooks/domains/github/use-github-enabled hooks/domains/gitlab/use-gitlab-enabled`
- **Files likely touched:**
  - `apps/web/hooks/domains/azure-devops/use-azure-devops-enabled.ts` (new)
  - `apps/web/hooks/domains/azure-devops/use-azure-devops-enabled.test.ts` (new)
  - `apps/web/hooks/domains/github/use-github-enabled.ts` (new)
  - `apps/web/hooks/domains/github/use-github-enabled.test.ts` (new)
  - `apps/web/hooks/domains/gitlab/use-gitlab-enabled.ts` (new)
  - `apps/web/hooks/domains/gitlab/use-gitlab-enabled.test.ts` (new)
- **Dependencies:** None.
- **Parallelism:** sequential (first task; nothing else can start before it).
- **Inputs:**
  - Spec: "Data model" (storage keys), "API surface" (hook shapes).
  - Plan: "Task 01" section.
  - Pattern to mirror exactly: `apps/web/hooks/domains/jira/use-jira-enabled.ts`
    and the generic implementation it wraps,
    `apps/web/hooks/domains/integrations/use-integration-enabled.ts`.
  - If a test already exists for a sibling wrapper (e.g.
    `use-linear-enabled.test.ts`), mirror its structure; if no wrapper-level
    test exists anywhere in the codebase today (only the generic hook is
    tested), write the minimal smoke test described in the plan's Tests
    section rather than re-testing `useIntegrationEnabled`'s full behavior.
- **Output contract:** summary, files changed, exact test command run and its
  output, blockers/risks, and update this task's `status` to `done` plus the
  corresponding checkbox in `plan.md`.

## Results

- Added `useAzureDevOpsEnabled`/`useGitHubEnabled`/`useGitLabEnabled` as
  one-line wrappers over `useIntegrationEnabled`, mirroring `useJiraEnabled`
  exactly (same storage-key/legacy-prefix/sync-event shape). No prior
  per-integration enabled key existed for these three, so the legacy-prefix
  migration scan is inert but present for signature symmetry.
- Added a unit test per hook (default-true, persisted-false round-trip,
  cross-tab custom-event propagation), mirroring `use-linear-enabled.test.ts`
  minus the legacy-migration case (no real legacy key ever existed for these
  three integrations).
- Command: `cd apps && pnpm --filter @kandev/web test -- hooks/domains/azure-devops/use-azure-devops-enabled hooks/domains/github/use-github-enabled hooks/domains/gitlab/use-gitlab-enabled` → `Test Files 3 passed (3)`, `Tests 12 passed (12)`.
- Files changed: `apps/web/hooks/domains/azure-devops/use-azure-devops-enabled.ts`,
  `apps/web/hooks/domains/azure-devops/use-azure-devops-enabled.test.ts`;
  `apps/web/hooks/domains/github/use-github-enabled.ts`,
  `apps/web/hooks/domains/github/use-github-enabled.test.ts`;
  `apps/web/hooks/domains/gitlab/use-gitlab-enabled.ts`,
  `apps/web/hooks/domains/gitlab/use-gitlab-enabled.test.ts` (all new).
- Blockers/risks: none.
