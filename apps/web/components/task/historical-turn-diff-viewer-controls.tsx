import { Button } from "@kandev/ui/button";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";
import { resolveTurnChangeScope } from "@/lib/turn-changes/history-scope";
import {
  projectTurnRepositoryAvailability,
  projectTurnChangeSummary,
} from "@/lib/turn-changes/projection";
import { useTranslation } from "react-i18next";

export function HistoricalViewerHeader({
  summary,
  onClose,
}: {
  summary?: TurnChangeSetSummary;
  onClose?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 flex-col gap-2 border-b px-3 py-2 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-semibold">
          {t("task:turnChangesHistoricalTitle", { turn: summary?.turn_ordinal ?? "" })}
        </p>
        <HistoricalSummaryStatus summary={summary} />
        {summary && (
          <p className="truncate text-xs text-muted-foreground">
            {t("task:turnChangesFileCount", { count: summary.file_count })}
            {summary.added_lines != null ? `  +${summary.added_lines}` : ""}
            {summary.deleted_lines != null ? `  -${summary.deleted_lines}` : ""}
          </p>
        )}
        <HistoricalRepositoryStatuses repositories={summary?.repositories ?? []} />
        {(summary?.overlap_intervals?.length ?? 0) > 0 && (
          <p role="status" className="truncate text-xs text-amber-700 dark:text-amber-400">
            {t("task:turnChangesSharedCheckout")}
          </p>
        )}
      </div>
      {onClose && (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="h-7 min-h-7 cursor-pointer px-2 text-xs [@media(pointer:coarse)]:min-h-11"
          onClick={onClose}
        >
          {t("task:close")}
        </Button>
      )}
    </div>
  );
}

function HistoricalSummaryStatus({ summary }: { summary?: TurnChangeSetSummary }) {
  const { t } = useTranslation();
  if (!summary) return <p className="truncate text-xs text-muted-foreground" />;
  const label = isPartial(summary) ? t("task:turnChangesPartial") : (summary.terminal_at ?? "");
  return <p className="truncate text-xs text-muted-foreground">{label}</p>;
}

function HistoricalRepositoryStatuses({
  repositories,
}: {
  repositories: TurnChangeSetSummary["repositories"];
}) {
  const { t } = useTranslation();
  const partialRepositories = repositories.filter(
    (repository) => projectTurnRepositoryAvailability(repository) !== "ready",
  );
  return partialRepositories.map((repository) => (
    <p key={repository.id} className="truncate text-xs text-amber-700 dark:text-amber-400">
      {t("task:turnChangesRepositoryStatus", {
        status: `${repository.display_name || repository.checkout_id}: ${t(`task:turnChangesStatus_${projectTurnRepositoryAvailability(repository)}`)}`,
      })}
    </p>
  ));
}

export function HistoricalScopeSelect({
  summaries,
  loading,
  hasMore,
  onLoadMore,
  onSelect,
  scopeValue,
}: {
  summaries: TurnChangeSetSummary[];
  loading: boolean;
  hasMore: boolean;
  onLoadMore: () => void;
  onSelect: (value: string) => void;
  scopeValue: string;
}) {
  const { t } = useTranslation();
  const latest = resolveTurnChangeScope("latest", summaries);
  const visibleSummaries = summaries.filter((summary) => projectTurnChangeSummary(summary).visible);
  return (
    <div className="flex shrink-0 items-center gap-2 border-b px-3 py-2">
      <label className="min-w-0 flex-1">
        <span className="sr-only">{t("task:turnChangesScope")}</span>
        <select
          className="min-h-11 w-full rounded-md border bg-background px-2 text-sm [@media(pointer:fine)]:min-h-8"
          aria-label={t("task:turnChangesScope")}
          value={scopeValue}
          onChange={(event) => onSelect(event.target.value)}
        >
          <option value="current">{t("task:turnChangesScopeCurrent")}</option>
          <option value="latest" disabled={!latest}>
            {t("task:turnChangesScopeLatest")}
          </option>
          {visibleSummaries.map((summary) => (
            <option key={summary.id} value={summary.id}>
              {t("task:turnChangesScopeTurn", { turn: summary.turn_ordinal })}
            </option>
          ))}
        </select>
      </label>
      {hasMore && (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="min-h-11 shrink-0 cursor-pointer px-2 text-xs [@media(pointer:fine)]:min-h-8"
          disabled={loading}
          onClick={onLoadMore}
        >
          {loading ? t("task:turnChangesLoading") : t("task:turnChangesLoadMoreTurns")}
        </Button>
      )}
    </div>
  );
}

function isPartial(summary: TurnChangeSetSummary): boolean {
  return summary.availability === "failed" || !summary.complete || !summary.summary_complete;
}
