import type { StatsResponse } from "@/lib/types/http";

export type RangeKey = "week" | "month" | "all";

export const RANGE_KEYS = ["week", "month", "all"] as const;
export const DEFAULT_RANGE: RangeKey = "month";

export function isRangeKey(value: string | null | undefined): value is RangeKey {
  return value === "week" || value === "month" || value === "all";
}

export function getRangeLabel(range: RangeKey): string {
  switch (range) {
    case "week":
      return "Last Week";
    case "month":
      return "Last Month";
    case "all":
      return "All Time";
    default:
      return "Last Month";
  }
}

export function formatDuration(ms: number): string {
  if (ms === 0) return "—";
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  if (hours > 0) return `${hours}h ${minutes % 60}m`;
  if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
  return `${seconds}s`;
}

export function getSubtitle(global: StatsResponse["global"] | null, hasError: boolean): string {
  if (global) {
    return `${global.total_tasks} tasks · ${global.total_sessions} sessions · ${formatDuration(global.total_duration_ms)}`;
  }
  return hasError ? "Failed to load stats" : "Loading stats…";
}

export type StatsState = {
  stats: StatsResponse | null;
  error: string | null;
};

export type StatsAction =
  | { type: "fetch" }
  | { type: "success"; stats: StatsResponse }
  | { type: "failure"; error: string };

export function statsReducer(state: StatsState, action: StatsAction): StatsState {
  switch (action.type) {
    case "fetch":
      return { stats: null, error: null };
    case "success":
      return { stats: action.stats, error: null };
    case "failure":
      return { stats: null, error: action.error };
  }
}

export type PanelState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; stats: StatsResponse };

export function toPanelState(stats: StatsResponse | null, error: string | null): PanelState {
  if (error) return { kind: "error", message: error };
  if (stats) return { kind: "ready", stats };
  return { kind: "loading" };
}

export function buildStatsSummary(
  resolvedStats: StatsResponse,
  rangeLabel: string,
  completedInRange: number,
): string {
  const { global, repository_stats, git_stats } = resolvedStats;
  const completion =
    global.total_tasks > 0
      ? `${Math.round((global.completed_tasks / global.total_tasks) * 100)}%`
      : "—";
  const topRepo = repository_stats
    .filter((r) => r.total_tasks > 0)
    .sort((a, b) => b.total_tasks - a.total_tasks)[0];
  const topRepoLabel = topRepo ? `${topRepo.repository_name} (${topRepo.total_tasks} tasks)` : "—";
  const hasGitStats =
    git_stats && (git_stats.total_commits > 0 || git_stats.total_files_changed > 0);
  const gitLine = hasGitStats
    ? `${git_stats.total_commits} commits, +${git_stats.total_insertions.toLocaleString()}/-${git_stats.total_deletions.toLocaleString()}`
    : "no git activity";
  return [
    `*Kandev Stats — ${rangeLabel}*`,
    `- Tasks: ${global.total_tasks} total (${global.completed_tasks} done, ${global.in_progress_tasks} in progress) · ${completion} completion`,
    `- Completed (${rangeLabel}): ${completedInRange}`,
    `- Time: ${formatDuration(global.total_duration_ms)} total · ${formatDuration(global.avg_duration_ms_per_task)} avg/task`,
    `- Repos: ${repository_stats.length} tracked · Top repo: ${topRepoLabel}`,
    `- Git: ${gitLine}`,
  ].join("\n");
}
