---
id: "06-resolver-correctness"
title: "Resolver correctness"
status: done
wave: remediation
depends_on: ["01-link-foundation"]
plan: "plan.md"
spec: "../../specs/ui/external-vcs-file-links.md"
---

# Task 06: Resolver correctness

## Acceptance

- A single linked task repository resolves in commit-detail context without explicit session/repository identity.
- A unique named session worktree resolves before metadata-name matching, including repeated-repository production-shaped inputs.
- Supported GitHub, self-hosted GitLab, and Azure DevOps SSH clone identities produce credential-free HTTPS file URLs.
- Unsafe schemes, credentials, malformed identities, traversal, and ambiguous context remain fail-closed with explicit tests.

## Output contract

Report RED/GREEN tests, changed files, exact focused checks, and residual provider-shape risks.

## Completion report

- **Summary:** Corrected repository/revision selection for commit-detail and repeated-repository worktrees, and completed safe SSH clone normalization for GitHub, self-hosted GitLab, and Azure DevOps.
- **Changed scope:** `use-external-vcs-file-link` resolver and tests; `external-vcs-file-url` resolver and tests; Review published-PR context where the resolver needs the GitHub pull-head revision.
- **Focused verification:** `(cd apps && pnpm --filter @kandev/web test -- lib/utils/external-vcs-file-url.test.ts hooks/domains/workspace/use-external-vcs-file-link.test.tsx)` passed; `(cd apps/web && pnpm run typecheck)` and targeted ESLint passed.
- **Residual risk:** Provider clone shapes outside the supported GitHub, GitLab, and Azure DevOps HTTPS/SSH identities remain intentionally unavailable; malformed, credential-bearing, traversal, and ambiguous inputs fail closed.
