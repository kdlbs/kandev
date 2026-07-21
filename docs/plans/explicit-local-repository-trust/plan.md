---
spec: docs/specs/workspaces/local-repositories.md
created: 2026-07-20
status: completed
---

# Implementation Plan: Explicit Local Repository Trust

## Overview

Separate bounded automatic discovery from explicit local-repository validation, then make the
repository record the durable exact-path grant. Migrate mutating Git operations to repository
identity, update the existing dialog contract without changing its layout, and finish with native
Windows and browser regression coverage.

## Backend

- Add one canonical explicit-path resolver for non-empty local repository paths. It resolves an
  absolute clean path, canonicalizes symlinks, verifies a directory and Git repository, and returns
  typed validation errors suitable for 4xx responses.
- Use the resolver in manual validation and in local repository create/update. Provider-backed
  repositories with empty local paths remain valid.
- Retain `repositoryDiscovery.roots` only in automatic discovery. Replace remaining raw string
  containment with platform-correct relative-path comparison where containment is still required.
- Resolve saved repositories by ID for branch refresh and fresh-branch mutation. Raw path endpoints
  remain read-only pre-registration probes.
- Preserve the `allowed` validation response field for compatibility while removing it from the
  frontend validity decision.

## Frontend

- Treat `exists && is_git` as the successful manual-validation contract in the existing Add Local
  Repository dialog.
- Keep loading, success, and failure messages and the current responsive dialog layout unchanged.
- Add a focused pure validation helper test so the deprecated `allowed=false` value cannot recreate
  the Windows regression while older backends remain understandable.

## Documentation

- Clarify in public and reference configuration docs that discovery roots bound automatic scans and
  do not need to include repositories selected explicitly.
- Keep the ADR, spec, and root indexes synchronized.

## Tests

- Service regression: a Git repository outside configured discovery roots validates, canonicalizes,
  persists, lists branches, refreshes, and supports identity-bound fresh-branch behavior.
- Negative service/handler cases: missing, file, plain directory, inaccessible/canonicalization
  failure, and invalid update all fail without persistence.
- Discovery regression: an explicit saved repository does not widen the roots used by automatic
  discovery.
- Windows regression: drive-letter casing, trailing separators, and native canonicalization run in
  the existing Windows backend job.
- Browser regression: desktop and mobile Add Local Repository flows save a real Git repository
  outside the backend's configured/default discovery root.

## Implementation Waves

Wave 1:

- [x] [task-01-explicit-path-contract](task-01-explicit-path-contract.md) - done

Wave 2:

- [x] [task-02-identity-bound-git-operations](task-02-identity-bound-git-operations.md) - done

Wave 3:

- [x] [task-03-frontend-validation-contract](task-03-frontend-validation-contract.md) - done

Wave 4:

- [x] [task-04-cross-platform-regression](task-04-cross-platform-regression.md) - done

## Verification

Run the Go commands from `apps/backend`:

```bash
rtk go test -v ./internal/task/service -run 'Test(ValidateLocalRepositoryPath|CreateRepository|UpdateRepository|DiscoverLocalRepositories)'
rtk go test -v ./internal/task/service -run 'Test(RefreshRepositoryBranches|PerformFreshBranch|ListBranches)'
rtk go test -race ./internal/task/service ./internal/task/handlers
```

Run the backend Make targets from the repository root:

```bash
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
```

Run from `apps/web`:

```bash
rtk pnpm test -- app/settings/workspace/workspace-repositories-validation.test.ts
rtk pnpm run typecheck
rtk pnpm e2e -- e2e/tests/settings/repository-add-local.spec.ts --project=chromium
rtk pnpm e2e -- e2e/tests/settings/mobile-repository-add-local.spec.ts --project=mobile-chrome
```

Run from `apps/`:

```bash
rtk pnpm --filter @kandev/web lint
```

Formatting runs before full test and lint verification because formatters may change line layout and
expose complexity thresholds.

## Risks

- Repository creation is reused by local and provider flows; validation must not reject pathless or
  not-yet-materialized provider repositories.
- Fresh-branch compensation currently happens after task creation. Identity migration must preserve
  task rollback and dirty-file consent behavior.
- Canonicalization must not break linked Git worktrees whose `.git` metadata points to a common
  directory outside the worktree root.
- Keeping the deprecated `allowed` field avoids an abrupt API removal, but its new compatibility
  meaning must be documented and tested.

## Open Questions

None.
