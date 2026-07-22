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
