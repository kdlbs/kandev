/**
 * Builds the per-executor "what will be cleaned up" lines shown in the
 * task delete/archive confirmation dialogs. The text describes which
 * resources are torn down (worktree / container / sandbox / remote dir)
 * and — importantly for the local executor — what is NOT touched.
 *
 * Executor types match `models.ExecutorType` in the Go backend
 * (apps/backend/internal/task/models/models.go).
 */

export type CleanupSummary = {
  /** Lines to render under the dialog description, in order. */
  lines: string[];
};

type KnownExecutor =
  | "local"
  | "worktree"
  | "local_docker"
  | "remote_docker"
  | "sprites"
  | "ssh"
  | "mock_remote";

const SINGLE: Record<KnownExecutor, string> = {
  local:
    "The agent ran directly in your repo — your files, branch, and folder are not touched. Only the agent session will be stopped.",
  worktree:
    "The task's git worktree and its branch will be deleted. Your main repo and other branches are not affected.",
  local_docker:
    "The Docker container running this task will be stopped and removed. Your host repo is not touched.",
  remote_docker: "The remote Docker container running this task will be stopped and removed.",
  sprites:
    "The Sprites cloud sandbox for this task will be destroyed. Any uncommitted work inside it will be lost.",
  ssh: "The task directory on the remote host will be removed (best-effort). Your local repo is not touched.",
  mock_remote: "The mock remote environment will be cleaned up.",
};

const GENERIC_LINE = "Any running agent sessions will be stopped.";

function normalize(executorType: string | null | undefined): KnownExecutor | null {
  if (!executorType) return null;
  const key = executorType.toLowerCase();
  if (key in SINGLE) return key as KnownExecutor;
  return null;
}

/** Single-task variant. */
export function getCleanupSummary(executorType: string | null | undefined): CleanupSummary {
  const known = normalize(executorType);
  if (!known) return { lines: [GENERIC_LINE] };
  return { lines: [SINGLE[known], GENERIC_LINE] };
}

const PLURAL: Partial<Record<KnownExecutor, (n: number) => string>> = {
  local: (n) => `${n} local ${pl(n, "task", "tasks")} — your repo folders won't be touched.`,
  worktree: (n) =>
    `${n} ${pl(n, "worktree", "worktrees")} and ${pl(n, "its branch", "their branches")} will be deleted.`,
  local_docker: (n) => `${n} Docker ${pl(n, "container", "containers")} will be removed.`,
  remote_docker: (n) => `${n} remote Docker ${pl(n, "container", "containers")} will be removed.`,
  sprites: (n) =>
    `${n} Sprites ${pl(n, "sandbox", "sandboxes")} will be destroyed (uncommitted work lost).`,
  ssh: (n) =>
    `${n} remote task ${pl(n, "directory", "directories")} will be removed (best-effort).`,
};

function pl(n: number, one: string, many: string): string {
  return n === 1 ? one : many;
}

/** Bulk variant — groups N tasks by executor type and emits one line per group. */
export function getBulkCleanupSummary(
  executorTypes: Array<string | null | undefined>,
): CleanupSummary {
  if (executorTypes.length === 0) return { lines: [GENERIC_LINE] };

  const counts = new Map<KnownExecutor | "unknown", number>();
  for (const t of executorTypes) {
    const known = normalize(t);
    const key = known ?? "unknown";
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }

  const order: Array<KnownExecutor | "unknown"> = [
    "worktree",
    "local_docker",
    "remote_docker",
    "sprites",
    "ssh",
    "local",
    "mock_remote",
    "unknown",
  ];

  const lines: string[] = [];
  for (const key of order) {
    const count = counts.get(key);
    if (!count) continue;
    if (key === "unknown" || key === "mock_remote") continue;
    const fmt = PLURAL[key];
    if (fmt) lines.push(fmt(count));
  }
  lines.push(GENERIC_LINE);
  return { lines };
}
