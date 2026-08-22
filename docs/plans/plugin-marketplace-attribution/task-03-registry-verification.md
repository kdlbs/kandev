---
id: "03-registry-verification"
title: "Verify regenerated catalog attribution"
status: pending
wave: 2
depends_on: ["01-bitbucket-attribution", "02-youtrack-attribution"]
plan: "plan.md"
spec: "../../specs/plugins/marketplace.md"
---

# Task 03: Verify regenerated catalog attribution

## Acceptance

- The generated official catalog record for Bitbucket has
  `author: "kandev"` and `repo_url: "https://github.com/kdlbs/kandev-plugin-bitbucket"`.
- The generated official catalog record for YouTrack has
  `author: "ahmedbally"` and
  `repo_url: "https://github.com/ahmedbally/kandev-plugin-youtrack"`.
- `plugin-registry/plugins.yaml` remains a pointer list with no author override
  or ownership rewrite.

## Verification

From the Kandev repository:

```bash
node --test plugin-registry/build-index.test.mjs
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run components/settings/plugins/marketplace-entry-row.test.tsx
```

After both releases are published, run `node plugin-registry/build-index.mjs`
with the normal GitHub API credentials and inspect the two records in the
generated `plugin-registry/index.json`. Preserve or remove the generated file
according to the existing registry workflow and `.gitignore` policy. Finish
with `git diff --check`.

## Files likely touched

- No Kandev production source files are expected.
- `plugin-registry/index.json` is a generated verification artifact only.

## Dependencies

Tasks 01 and 02, including their release artifacts.

## Parallelism

sequential. The task depends on both external release outputs and reads the
shared official catalog.

## Inputs

- `docs/specs/plugins/marketplace.md`, author and repository scenarios.
- `plugin-registry/build-index.mjs`, release-manifest enrichment behavior.
- `apps/web/components/settings/plugins/marketplace-entry-row.tsx`, unchanged
  presentation consumer.

## Output contract

Report the exact catalog records, commands and results, generated-artifact
handling, `git diff --check` result, and synchronized task/plan statuses.

## Results

2026-08-22: Local verification passed with:

- `node --test plugin-registry/build-index.test.mjs` (8 tests)
- `cd apps && pnpm install --frozen-lockfile`
- `pnpm --filter @kandev/web test -- --run components/settings/plugins/marketplace-entry-row.test.tsx` (3 tests)
- `git diff --check`

The live catalog rebuild is blocked until the Bitbucket and YouTrack release
publications exist. `plugin-registry/plugins.yaml` remains unchanged and
contains no author override.
