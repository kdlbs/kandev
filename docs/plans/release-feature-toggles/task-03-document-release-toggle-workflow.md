---
id: "03-document-release-toggle-workflow"
title: "Document release-toggle workflow"
status: completed
wave: 2
depends_on: ["01-centralize-backend-bindings", "02-consolidate-frontend-contract"]
plan: "plan.md"
spec: "../../specs/feature-toggles/spec.md"
---

# Task 03: Document release-toggle workflow

- **Acceptance:** Architecture, contributor, operator, and agent guidance agree
  on install-wide ownership, all-profile default-off rollout, authoritative
  entry-path gates, one default-on kill-switch release, removal, and key
  non-reuse.
- **Acceptance:** Documentation names the streamlined declaration sites and no
  longer lists graduated/duplicated flags or obsolete backend paths.
- **Acceptance:** The unused E2E helper for the graduated unread-divider flag is
  removed, and public-doc validation passes.
- **Verification:** Run `node --test scripts/validate-public-docs.test.mjs`,
  `node scripts/validate-public-docs.mjs`, and `git diff --check` from the repo
  root.
- **Files likely touched:**
  - `docs/decisions/0007-runtime-feature-flags.md`
  - `docs/decisions/0018-runtime-settings-overrides.md`
  - `docs/public/configuration.md`
  - `docs/public/extending-kandev.md`
  - `AGENTS.md`
  - `apps/web/e2e/tests/settings/feature-toggles-helpers.ts` (remove)
- **Dependencies:** Tasks 01 and 02, so the documented add/remove steps match
  the landed implementation.
- **Parallelism:** `sequential`; it reconciles both prior task outputs.
- **Inputs:** Feature Toggles spec; ADR-0007, ADR-0018, and
  ADR-2026-08-01-release-toggle-gating-contract; Docs Maintainer guidance.
- **Output contract:** Report changed/removed files, exact validation commands
  and counts, security/trust impact (`None` expected), external side effects
  (`None` expected), then update this task and `plan.md` statuses/results in the
  same conversation.

## Results

- Updated ADR-0007, public configuration/extending docs, and root agent
  guidance with install-wide ownership, fail-closed entry paths, staged
  rollout, kill-switch retention, removal, and retired key/environment
  non-reuse.
- Added the release-toggle contract ADR and indexes, and removed the orphaned
  unread-divider feature-toggle E2E helper.
- Verification: `node --test scripts/validate-public-docs.test.mjs` passed
  (58 tests); `node scripts/validate-public-docs.mjs` validated 41 pages;
  `git diff --check` passed.
- Security/trust impact: none; docs clarify existing boundaries.
- External side effects: none.
- Review verification: public-doc tests passed (58 tests), validation covered
  41 published pages, and `git diff --check` passed.
