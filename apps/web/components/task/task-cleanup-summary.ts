// Executor types match models.ExecutorType in apps/backend/internal/task/models/models.go.
import { t } from "@/lib/i18n";

export type CleanupSummary = {
  effects: string[];
  notes: string[];
};

// mock_remote is test-only and intentionally falls through to the generic effect.
type KnownExecutor = "local" | "worktree" | "local_docker" | "remote_docker" | "sprites" | "ssh";

type CleanupCopy = {
  effectKey?: string;
  noteKey?: string;
};

/**
 * Catalog keys, not copy. The values resolve inside each public function so a
 * locale switch updates both the effect list and the supporting notes.
 */
const SINGLE_COPIES: Record<KnownExecutor, CleanupCopy> = {
  local: { noteKey: "task:cleanupSingleLocal" },
  worktree: {
    effectKey: "task:cleanupSingleWorktree",
    noteKey: "task:cleanupSingleWorktreeNote",
  },
  local_docker: {
    effectKey: "task:cleanupSingleLocalDocker",
    noteKey: "task:cleanupSingleLocalDockerNote",
  },
  remote_docker: { effectKey: "task:cleanupSingleRemoteDocker" },
  sprites: { effectKey: "task:cleanupSingleSprites" },
  ssh: {
    effectKey: "task:cleanupSingleSsh",
    noteKey: "task:cleanupSingleSshNote",
  },
};

const GENERIC_EFFECT_KEY = "task:cleanupAgentSessionsStopped";

function normalize(executorType: string | null | undefined): KnownExecutor | null {
  if (!executorType) return null;
  const key = executorType.toLowerCase();
  if (Object.hasOwn(SINGLE_COPIES, key)) return key as KnownExecutor;
  return null;
}

function resolveCopy(copy: CleanupCopy, options?: { count: number }): CleanupSummary {
  const effects = copy.effectKey ? [t(copy.effectKey, options)] : [];
  effects.push(t(GENERIC_EFFECT_KEY));
  const notes = copy.noteKey ? [t(copy.noteKey, options)] : [];
  return { effects, notes };
}

/** Single-task variant. */
export function getCleanupSummary(executorType: string | null | undefined): CleanupSummary {
  const known = normalize(executorType);
  return known
    ? resolveCopy(SINGLE_COPIES[known])
    : { effects: [t(GENERIC_EFFECT_KEY)], notes: [] };
}

/** Catalog keys for the grouped variant. */
const BULK_COPIES: Record<KnownExecutor, CleanupCopy> = {
  local: { noteKey: "task:cleanupBulkLocal" },
  worktree: {
    effectKey: "task:cleanupBulkWorktree",
    noteKey: "task:cleanupBulkWorktreeNote",
  },
  local_docker: {
    effectKey: "task:cleanupBulkLocalDocker",
    noteKey: "task:cleanupBulkLocalDockerNote",
  },
  remote_docker: { effectKey: "task:cleanupBulkRemoteDocker" },
  sprites: { effectKey: "task:cleanupBulkSprites" },
  ssh: {
    effectKey: "task:cleanupBulkSsh",
    noteKey: "task:cleanupBulkSshNote",
  },
};

/** Bulk variant. Groups known tasks by executor type and preserves display order. */
export function getBulkCleanupSummary(
  executorTypes: Array<string | null | undefined>,
): CleanupSummary {
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
  const effects: string[] = [];
  const notes: string[] = [];
  for (const key of order) {
    const count = counts.get(key);
    if (!count) continue;
    const copy = BULK_COPIES[key];
    if (copy.effectKey) effects.push(t(copy.effectKey, { count }));
    if (copy.noteKey) notes.push(t(copy.noteKey, { count }));
  }
  effects.push(t(GENERIC_EFFECT_KEY));
  return { effects, notes };
}
