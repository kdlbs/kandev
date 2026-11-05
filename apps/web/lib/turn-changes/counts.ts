import type { TurnChangeSetSummary, TurnRepositoryChange } from "@/lib/types/turn-changes";

type Translate = (key: string, options?: Record<string, unknown>) => string;

export function summaryCountText(summary: TurnChangeSetSummary, t: Translate): string {
  if (summary.legacy_counts) {
    return t("task:turnChangesRepositoryChangeCount", { count: summary.file_count });
  }
  if (summary.summary_complete === false) {
    return t("task:turnChangesAvailableChangeCount", { count: summary.file_count });
  }
  return t("task:turnChangesFileCount", { count: summary.file_count });
}

export function repositoryCountText(
  repository: TurnRepositoryChange | undefined,
  loaded: number,
  total: number | undefined,
  t: Translate,
): string | null {
  if (repository && repository.availability !== "ready") return null;
  if (repository?.enumeration_complete === false) {
    return t("task:turnChangesLoadedFileCount", { count: loaded });
  }
  return t("task:turnChangesFileCount", { count: total ?? loaded });
}
