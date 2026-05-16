/**
 * Canonical executor type identifiers. Mirrors the backend's database
 * `executors.type` values plus the synthetic `worktree` executor used in the
 * settings UI for git worktree isolation. See `CLAUDE.md` -> Executor Types.
 *
 * Keep this union in sync with the backend:
 *   apps/backend/internal/task/models/models.go (ExecutorType constants)
 *
 * Note: `"local"` is the seeded `Executor.type` value (see
 * `task/repository/sqlite/defaults.go`). `"local_pc"` appears in the office
 * `ExecutorTypeMeta` listing (`office/models/meta.go`) and the SQLite
 * backfill default for orphaned tasks — both can flow through `Executor.type`
 * in the wild, so the union keeps them both.
 */
export type ExecutorType =
  | "local"
  | "local_pc"
  | "local_docker"
  | "sprites"
  | "remote_docker"
  | "remote_vps"
  | "k8s"
  | "worktree";
