---
id: "10-remote-repository-contracts"
title: "Remote repository contracts and discovery"
status: done
wave: 6
depends_on: []
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 10: Remote Repository Contracts And Discovery

## Acceptance

- Task create/add-branch APIs accept provider-neutral `remote_url` inputs and retain `github_url` compatibility.
- Provider-backed repository rows persist a canonical credential-free `remote_url` with a migration for existing data.
- GitHub, GitLab, and Azure discovery normalize into one typed frontend repository shape.
- Branch listing dispatches to the correct provider and never calls GitHub APIs for GitLab or Azure URLs.
- Provider hints from clients are revalidated before persistence.

## Verification

- Go table tests for URL parsing, provider identity, migration, compatibility, and workspace isolation.
- TypeScript tests for discovery normalization and per-provider branch dispatch.
- Backend and web typecheck/lint suites for touched packages.

## Files Likely Touched

- Task DTO, service, repository model/store, and migrations.
- `apps/backend/internal/repoclone/` URL support.
- GitHub, GitLab, and Azure repository/branch endpoints.
- Provider-neutral web API types and hooks used by task creation.

## Output Contract

Report the compatibility contract, migrations, validation rules, RED/GREEN commands, and update this task plus its plan checkbox.
