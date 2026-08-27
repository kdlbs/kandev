// Executor types match models.ExecutorType in apps/backend/internal/task/models/models.go.
import { t } from "@/lib/i18n";

export type CleanupSummary = {
  lines: string[];
};

// mock_remote is test-only — intentionally absent so it falls through to GENERIC_LINE.
type KnownExecutor = "local" | "worktree" | "local_docker" | "remote_docker" | "sprites" | "ssh";

/**
 * Catalog keys, not copy.
 *
 * Resolved in `getCleanupSummary` / `getBulkCleanupSummary`, which the delete
 * and archive dialogs call from their render bodies — so a locale switch
 * re-renders them and the lines follow. Holding `t()` results here instead would
 * freeze this copy at the boot locale.
 */
const SINGLE_KEYS: Record<KnownExecutor, string> = {
  local: "task:cleanupSingleLocal",
  worktree: "task:cleanupSingleWorktree",
  local_docker: "task:cleanupSingleLocalDocker",
  remote_docker: "task:cleanupSingleRemoteDocker",
  sprites: "task:cleanupSingleSprites",
  ssh: "task:cleanupSingleSsh",
};

const GENERIC_KEY = "task:cleanupAgentSessionsStopped";

function normalize(executorType: string | null | undefined): KnownExecutor | null {
  if (!executorType) return null;
  const key = executorType.toLowerCase();
  if (key in SINGLE_KEYS) return key as KnownExecutor;
  return null;
}

/** Single-task variant. */
export function getCleanupSummary(executorType: string | null | undefined): CleanupSummary {
  const generic = t(GENERIC_KEY);
  const known = normalize(executorType);
  if (!known) return { lines: [generic] };
  return { lines: [t(SINGLE_KEYS[known]), generic] };
}

/**
 * Catalog keys for the grouped variant.
 *
 * Each resolves with `{ count }` against `_one`/`_other` entries, so the
 * singular/plural split is the catalog's decision. The previous version built it
 * here with a local `pl(n, "worktree", "worktrees")` helper — English-only by
 * construction, and wrong in every locale whose plural rules are not English's.
 */
const PLURAL_KEYS: Record<KnownExecutor, string> = {
  local: "task:cleanupBulkLocal",
  worktree: "task:cleanupBulkWorktree",
  local_docker: "task:cleanupBulkLocalDocker",
  remote_docker: "task:cleanupBulkRemoteDocker",
  sprites: "task:cleanupBulkSprites",
  ssh: "task:cleanupBulkSsh",
};

/** Bulk variant — groups N tasks by executor type and emits one line per group. */
export function getBulkCleanupSummary(
  executorTypes: Array<string | null | undefined>,
): CleanupSummary {
  const generic = t(GENERIC_KEY);
  if (executorTypes.length === 0) return { lines: [generic] };

  const counts = new Map<KnownExecutor, number>();
  for (const executorType of executorTypes) {
    const known = normalize(executorType);
    if (!known) continue;
    counts.set(known, (counts.get(known) ?? 0) + 1);
  }

  const order: KnownExecutor[] = [
    "worktree",
    "local_docker",
    "remote_docker",
    "sprites",
    "ssh",
    "local",
  ];

  const lines: string[] = [];
  for (const key of order) {
    const count = counts.get(key);
    if (!count) continue;
    lines.push(t(PLURAL_KEYS[key], { count }));
  }
  lines.push(generic);
  return { lines };
}
