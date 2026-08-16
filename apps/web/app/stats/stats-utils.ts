import type { StatsRange } from "@/lib/api/domains/stats-api";
import type { StatsResponse } from "@/lib/types/http";
import { t } from "@/lib/i18n";

export type RangeKey = StatsRange;

export const RANGE_KEYS = ["week", "month", "all"] as const satisfies readonly RangeKey[];
export const DEFAULT_RANGE: RangeKey = "month";

export function isRangeKey(value: string | null | undefined): value is RangeKey {
  return value === "week" || value === "month" || value === "all";
}

export function getRangeLabel(range: RangeKey): string {
  switch (range) {
    case "week":
      return t("stats:rangeLastWeek");
    case "month":
      return t("stats:rangeLastMonth");
    case "all":
      return t("stats:rangeAllTime");
  }
}

export function formatDuration(ms: number): string {
  if (ms === 0) return "-";
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  if (hours > 0) return `${hours}h ${minutes % 60}m`;
  if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
  return `${seconds}s`;
}

export function getSubtitle(global: StatsResponse["global"] | null, hasError: boolean): string {
  if (global) {
    // Composed from two count-bearing keys rather than one template, because
    // i18next carries a single `count` per key and both nouns inflect.
    return t("stats:subtitleReady", {
      tasks: t("stats:subtitleTaskCount", { count: global.total_tasks }),
      sessions: t("stats:subtitleSessionCount", { count: global.total_sessions }),
      duration: formatDuration(global.total_duration_ms),
    });
  }
  return hasError ? t("stats:failedToLoadStats") : t("stats:loadingStats");
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
      : "-";
  const topRepo = repository_stats
    .filter((r) => r.total_tasks > 0)
    .sort((a, b) => b.total_tasks - a.total_tasks)[0];
  const topRepoLabel = topRepo
    ? t("stats:summaryTopRepo", {
        repository: topRepo.repository_name,
        count: topRepo.total_tasks,
      })
    : "-";
  const hasGitStats =
    git_stats && (git_stats.total_commits > 0 || git_stats.total_files_changed > 0);
  const gitLine = hasGitStats
    ? t("stats:summaryGitActivity", {
        count: git_stats.total_commits,
        insertions: git_stats.total_insertions.toLocaleString(),
        deletions: git_stats.total_deletions.toLocaleString(),
      })
    : t("stats:noGitActivity");
  // The whole report, not only the range and the empty-git fragment. A summary
  // the user pastes elsewhere must not be half English and half translated.
  return [
    t("stats:summaryHeading", { range: rangeLabel }),
    t("stats:summaryTasks", {
      total: global.total_tasks,
      done: global.completed_tasks,
      inProgress: global.in_progress_tasks,
      completion,
    }),
    t("stats:summaryCompletedInRange", { range: rangeLabel, completed: completedInRange }),
    t("stats:summaryTime", {
      total: formatDuration(global.total_duration_ms),
      average: formatDuration(global.avg_duration_ms_per_task),
    }),
    t("stats:summaryRepos", { count: repository_stats.length, topRepo: topRepoLabel }),
    t("stats:summaryGit", { git: gitLine }),
  ].join("\n");
}
