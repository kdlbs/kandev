---
id: "09-per-org-filesystem-roots"
title: "Per-org filesystem roots"
status: todo
wave: 4
depends_on: ["04-service-layer-org-scoping"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 09: Per-Org Filesystem Roots

## Acceptance

- One resolver owns org storage-root resolution: `orgs.storage_root` when set,
  the legacy flat tree when `''`. No caller builds an org path by string
  concatenation.
- A new org's root is `<home>/orgs/<org_id>/` created mode `0700`, containing
  worktrees, clones, attachments, and the agent credential home.
- An upgraded instance's default org keeps `storage_root = ''` permanently. The
  upgrade moves no files, and a test asserts the file count and mtimes under
  `<home>` are unchanged across the migration.
- Worktree creation, repository clones, attachment storage, task environments,
  and quick terminals all resolve through the resolver.
- Storage maintenance, GC, quarantine, and temp-artifact sweeps run per-org and
  never traverse another org's root. Containment checks reject a path that
  escapes the resolved root, using the carried `FileInfo` rather than a re-stat.
- A missing or unwritable org root fails task creation and session launch in
  that org with a storage error and leaves other orgs working. There is no
  fallback to the shared tree.

## Verification

- `go test ./internal/worktree/... ./internal/repoclone/... ./internal/system/...`
- `go test ./internal/tenancy/... -run 'TestStorageRoot|TestUpgradeMovesNoFiles'`
- `go test ./internal/task/service/... -run TestAttachmentOrgRoot`

## Files Likely Touched

- `apps/backend/internal/tenancy/storage.go`
- `apps/backend/internal/worktree/`, `internal/repoclone/`
- `apps/backend/internal/task/service/attachment_service.go`
- `apps/backend/internal/system/` storage maintenance, quarantine, temp artifacts
- `apps/backend/internal/quickterminal/`

## Inputs

- Spec: What (filesystem isolation), Persistence guarantees, Failure modes.
- Patterns: the path/security test conventions in `apps/backend/AGENTS.md`
  (fake absolute roots under `t.TempDir()`, carry the `Lstat` result through,
  no re-stat before side effects); ADR 0045 install-wide storage maintenance.

## Output Contract

Report the resolver's call sites versus a grep for direct `<home>` path
construction, the upgrade no-move evidence, RED/GREEN commands, and set this
task plus its plan checkbox to done.
