---
id: "03-public-integration-documentation"
title: "Public integration documentation"
status: completed
wave: 3
depends_on: ["02-workspace-github-defaults"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 03: Public integration documentation

- **Acceptance:** Public docs identify executor credentials as the new-workspace default and
  managed workspace credentials as an explicit later choice.
- **Acceptance:** The docs explain operator-only active host `gh` auto-binding, disconnected fallback,
  remote-executor responsibility, and unchanged upgrade behavior without implying that a token is
  stored.
- **Verification:** Run both public-doc validators.

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

- **Files likely touched:** `docs/public/integrations.md`.
- **Dependencies:** Task 02.
- **Parallelism:** sequential; documentation must reflect the final green behavior.
- **Inputs:** amended GitHub-authentication spec, ADR-2026-08-02-new-workspace-github-access-defaults,
  and Task 02 results.
- **Risks:** Avoid saying Local/Worktree behavior also applies to Docker/SSH/cloud executors, and do
  not describe existing workspaces as migrated.
- **Output contract:** Summarize the doc changes and exact validator results; mark this task and the
  plan complete and synchronize `## Verification Results`.

## Results

- Updated `docs/public/integrations.md` to document executor inheritance as the new-workspace
  default, managed credentials as opt-in, operator/admin-only active `gh` auto-binding, soft
  disconnected fallback, remote-executor credential responsibility, and unchanged upgrades.
- Public-doc test suite: `58 passed`.
- Public-doc validator: `Validated 41 published docs pages.`
