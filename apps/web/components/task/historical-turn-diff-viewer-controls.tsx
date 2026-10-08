import { formatDateTime } from "@/lib/i18n/formats";
import { Button } from "@kandev/ui/button";
import { turnChangeRepositoryOptionName } from "@/lib/turn-changes/tree";
import type { TurnFileChange, TurnChangeSetSummary } from "@/lib/types/turn-changes";
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
          className="h-7 min-h-7 cursor-pointer px-2 text-xs max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
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
  const timestamp = summary.terminal_at ? formatDateTime(summary.terminal_at) : "";
  const label = isPartial(summary) ? t("task:turnChangesPartial") : timestamp;
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
          className="h-7 min-h-7 w-full rounded-md border bg-background px-2 text-sm max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
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
              {summary.terminal_at ? ` · ${formatDateTime(summary.terminal_at)}` : ""}
            </option>
          ))}
        </select>
      </label>
      {hasMore && (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="h-7 min-h-7 shrink-0 cursor-pointer px-2 text-xs max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
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

export function FileSelectionControls({
  files,
  repositories,
  selectedFile,
  onSelect,
  whitespace,
  onWhitespace,
}: {
  files: TurnFileChange[];
  repositories: TurnChangeSetSummary["repositories"];
  selectedFile: TurnFileChange | null;
  onSelect: (fileId: string) => void;
  whitespace: boolean;
  onWhitespace: (value: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="shrink-0 border-b px-3 py-2">
      <div className="flex flex-col gap-2 md:flex-row md:items-center">
        <label className="min-w-0 flex-1">
          <span className="sr-only">{t("task:turnChangesSelectFile")}</span>
          <select
            className="min-h-11 w-full min-w-0 rounded-md border bg-background px-2 text-sm md:min-h-7 [@media(pointer:coarse)]:min-h-11"
            aria-label={t("task:turnChangesSelectFile")}
            value={selectedFile?.id ?? ""}
            onChange={(event) => onSelect(event.target.value)}
            disabled={files.length === 0}
          >
            {files.map((file) => (
              <option key={file.id} value={file.id}>
                {t("task:turnChangesFileOptionLabel", {
                  repository: turnChangeRepositoryOptionName(
                    repositories,
                    file.repository_change_id,
                    file.checkout_id,
                  ),
                  path: file.path,
                })}
              </option>
            ))}
          </select>
        </label>
        <label className="flex min-h-11 items-center gap-2 text-xs md:min-h-7 [@media(pointer:coarse)]:min-h-11">
          <input
            type="checkbox"
            checked={whitespace}
            onChange={(event) => onWhitespace(event.target.checked)}
          />
          {t("task:turnChangesIgnoreWhitespace")}
        </label>
      </div>
      {selectedFile && (
        <div className="mt-2 min-w-0 text-xs text-muted-foreground">
          <p data-testid="turn-change-file-path" className="break-all font-mono">
            {selectedFile.path}
          </p>
          {selectedFile.old_path && (
            <p data-testid="turn-change-old-path" className="mt-1 break-all">
              {t("task:turnChangesRenamedFrom")} {selectedFile.old_path}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
