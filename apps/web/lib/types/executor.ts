/**
 * Canonical executor type identifiers. Mirrors the backend's database
 * `executors.type` values plus the synthetic `worktree` executor used in the
 * settings UI for git worktree isolation. See `CLAUDE.md` -> Executor Types.
 *
 * Keep this union in sync with the backend:
 *   apps/backend/internal/agent/executor/types.go
 */
export type ExecutorType =
  | "local_pc"
  | "local_docker"
  | "sprites"
  | "remote_docker"
  | "remote_vps"
  | "k8s"
  | "worktree";
