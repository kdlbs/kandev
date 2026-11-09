"use client";

import { memo, useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { LineStat } from "@/components/diff-stat";
import { FileIcon } from "@/components/ui/file-icon";
import { IconChevronDown, IconChevronRight, IconFolder, IconFolderOpen } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useTurnChangeFiles } from "@/hooks/domains/session/use-turn-change-files";
import { buildTurnChangeTree, summarizeTurnChangeTreeFolder } from "@/lib/turn-changes/tree";
import type { TurnChangeSetSummary, TurnFileChange } from "@/lib/types/turn-changes";
import type { TurnChangeTreeFolder } from "@/lib/turn-changes/tree";
import type { HistoricalTurnDiffTarget } from "@/lib/state/diff-target-types";
import { readTurnChangeViewState, updateTurnChangeViewState } from "@/lib/turn-changes/view-state";
import {
  isTurnChangePartial,
  projectTurnRepositoryAvailability,
} from "@/lib/turn-changes/projection";
import { repositoryCountText, summaryCountText } from "@/lib/turn-changes/counts";
import { CardNotice, LoadMoreButton, RetryFilesButton } from "./turn-changed-files-card-parts";

type TurnChangedFilesCardProps = {
  sessionId: string;
  summary: TurnChangeSetSummary;
  onOpenDiff: (target: HistoricalTurnDiffTarget) => void;
};

function fileCounts(file: TurnFileChange) {
  return file.added_lines != null && file.deleted_lines != null
    ? { added: file.added_lines, deleted: file.deleted_lines }
    : null;
}

function statusLabel(file: TurnFileChange, t: (key: string) => string): string {
  if (file.binary) return t("task:turnChangesBinary");
  const counts = fileCounts(file);
  if (!counts) return t("task:turnChangesUnknownCounts");
  return `+${counts.added}  -${counts.deleted}`;
}

function kindLabel(file: TurnFileChange, t: (key: string) => string): string {
  const keyByKind: Record<string, string> = {
    added: "turnChangesKindAdded",
    deleted: "turnChangesKindDeleted",
    modified: "turnChangesKindModified",
    renamed: "turnChangesKindRenamed",
    copied: "turnChangesKindCopied",
    type_changed: "turnChangesKindTypeChanged",
    mode_changed: "turnChangesKindModeChanged",
  };
  return t(`task:${keyByKind[file.kind] ?? "turnChangesKindModified"}`);
}

const KIND_COLOR_CLASS: Record<string, string> = {
  added: "text-emerald-600",
  deleted: "text-rose-600",
  modified: "text-amber-600",
  renamed: "text-purple-600",
};

function targetForFile(
  sessionId: string,
  summary: TurnChangeSetSummary,
  repositoryChangeId: string,
  file: TurnFileChange,
): HistoricalTurnDiffTarget {
  return {
    sessionId,
    changeSetId: summary.id,
    repositoryChangeId,
    fileChangeId: file.id,
    checkoutId: file.checkout_id,
    path: file.path,
    fileKind: file.kind,
    ...(file.old_path ? { oldPath: file.old_path } : {}),
  };
}

function FolderRows({
  folder,
  repositoryChangeId,
  sessionId,
  summary,
  expanded,
  onToggle,
  onOpenFile,
  depth,
  t,
}: {
  folder: TurnChangeTreeFolder;
  repositoryChangeId: string;
  sessionId: string;
  summary: TurnChangeSetSummary;
  expanded: Set<string>;
  onToggle: (key: string) => void;
  onOpenFile: (target: HistoricalTurnDiffTarget) => void;
  depth: number;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const isExpanded = expanded.has(`${repositoryChangeId}:${folder.path}`);
  const totals = summarizeTurnChangeTreeFolder(folder);
  return (
    <div>
      <button
        type="button"
        className="flex min-h-7 w-full min-w-0 flex-wrap cursor-pointer items-center gap-2 rounded px-2 text-left text-xs hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        style={{ paddingInlineStart: `${12 + Math.min(depth, 3) * 16}px` }}
        aria-expanded={isExpanded}
        onClick={() => onToggle(`${repositoryChangeId}:${folder.path}`)}
        aria-label={t("task:turnChangesFolderAccessibleLabel", {
          path: folder.path,
          count: totals.loadedFiles,
          added: totals.addedLines,
          deleted: totals.deletedLines,
          unknown: totals.unknownCountFiles,
        })}
      >
        {isExpanded ? (
          <IconChevronDown className="size-3.5 shrink-0" />
        ) : (
          <IconChevronRight className="size-3.5 shrink-0" />
        )}
        {isExpanded ? (
          <IconFolderOpen className="size-4 shrink-0 text-muted-foreground" />
        ) : (
          <IconFolder className="size-4 shrink-0 text-muted-foreground" />
        )}
        <span className="min-w-0 flex-1 truncate">{folder.name}/</span>
        <span className="ml-auto shrink-0 text-muted-foreground">
          {t("task:turnChangesLoadedFileCount", { count: totals.loadedFiles })}
        </span>
        <LineStat added={totals.addedLines} removed={totals.deletedLines} className="shrink-0" />
        {totals.unknownCountFiles > 0 && (
          <span className="shrink-0 text-muted-foreground">
            {t("task:turnChangesUnknownCount", { count: totals.unknownCountFiles })}
          </span>
        )}
      </button>
      {isExpanded && (
        <div>
          {folder.folders.map((child) => (
            <FolderRows
              key={child.path}
              folder={child}
              repositoryChangeId={repositoryChangeId}
              sessionId={sessionId}
              summary={summary}
              expanded={expanded}
              onToggle={onToggle}
              onOpenFile={onOpenFile}
              depth={depth + 1}
              t={t}
            />
          ))}
          {folder.files.map(({ file }) => (
            <FileRow
              key={file.id}
              file={file}
              depth={depth + 1}
              onOpen={() => onOpenFile(targetForFile(sessionId, summary, repositoryChangeId, file))}
              t={t}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function FileRow({
  file,
  depth,
  onOpen,
  t,
}: {
  file: TurnFileChange;
  depth: number;
  onOpen: () => void;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const counts = fileCounts(file);
  const statusContent =
    file.binary || !counts ? (
      <span className="min-w-0 [overflow-wrap:anywhere] text-muted-foreground">
        {statusLabel(file, t)}
      </span>
    ) : (
      <LineStat added={counts.added} removed={counts.deleted} className="shrink-0" />
    );
  return (
    <button
      type="button"
      className="flex min-h-7 w-full min-w-0 flex-wrap cursor-pointer items-center gap-2 rounded px-2 text-left text-xs hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      style={{ paddingInlineStart: `${12 + Math.min(depth, 3) * 16}px` }}
      onClick={onOpen}
      aria-label={t("task:turnChangesFileAccessibleLabel", {
        path: file.path,
        kind: kindLabel(file, t),
        status: statusLabel(file, t),
        mode:
          file.old_mode && file.new_mode && file.old_mode !== file.new_mode
            ? t("task:turnChangesModeStatus", { oldMode: file.old_mode, newMode: file.new_mode })
            : "",
      })}
      title={file.path}
      data-turn-file-change-id={file.id}
    >
      <FileIcon
        filePath={file.path}
        className="flex-shrink-0"
        style={{ width: "14px", height: "14px" }}
      />
      <span className="min-w-0 flex-1 py-1 [overflow-wrap:anywhere]">
        <span className="block truncate">{file.path.split("/").at(-1) || file.path}</span>
        {file.old_path && (
          <span className="block truncate text-muted-foreground">
            {t("task:turnChangesRenamedFrom")} {file.old_path}
          </span>
        )}
      </span>
      <span className="flex min-w-0 max-w-full flex-wrap items-center gap-x-2 gap-y-0.5">
        <span
          className={`min-w-0 [overflow-wrap:anywhere] ${KIND_COLOR_CLASS[file.kind] ?? "text-muted-foreground"}`}
        >
          {kindLabel(file, t)}
        </span>
        {statusContent}
      </span>
      {file.old_mode && file.new_mode && file.old_mode !== file.new_mode && (
        <span className="sr-only">
          {t("task:turnChangesModeStatus", { oldMode: file.old_mode, newMode: file.new_mode })}
        </span>
      )}
    </button>
  );
}

export const TurnChangedFilesCard = memo(function TurnChangedFilesCard({
  sessionId,
  summary,
  onOpenDiff,
}: TurnChangedFilesCardProps) {
  const { t } = useTranslation();
  const { filesByRepository, fileTotalsByRepository, loading, error, hasMore, loadMore, retry } =
    useTurnChangeFiles(sessionId, summary);
  const [expanded, setExpanded] = useState<Set<string>>(
    () => new Set((summary.repositories ?? []).map((repository) => repository.id)),
  );
  useEffect(() => {
    const saved = readTurnChangeViewState(sessionId, summary.id);
    if (saved) setExpanded(new Set(saved.expandedKeys));
  }, [sessionId, summary.id]);
  const tree = useMemo(
    () =>
      buildTurnChangeTree(
        summary.repositories ?? [],
        filesByRepository,
        t("task:turnChangesRepository"),
      ),
    [filesByRepository, summary.repositories, t],
  );
  const allFolderKeys = useMemo(() => {
    const keys: string[] = [];
    const visit = (repositoryId: string, folders: TurnChangeTreeFolder[]) => {
      for (const folder of folders) {
        keys.push(`${repositoryId}:${folder.path}`);
        visit(repositoryId, folder.folders);
      }
    };
    for (const repository of tree) visit(repository.id, repository.folders);
    return [...(summary.repositories ?? []).map((repository) => repository.id), ...keys];
  }, [summary.repositories, tree]);
  const isExpired = summary.availability === "expired";
  const isPending = summary.availability === "pending";
  const isUnavailable = summary.availability === "unavailable" || summary.availability === "failed";
  const hasRows = Object.values(filesByRepository).some((files) => files.length > 0);
  const allExpanded = allFolderKeys.length > 0 && allFolderKeys.every((key) => expanded.has(key));

  const toggleAll = () => {
    const next = allExpanded ? new Set<string>() : new Set(allFolderKeys);
    setExpanded(next);
    updateTurnChangeViewState(sessionId, summary.id, { expandedKeys: [...next] });
  };
  const toggleExpanded = (key: string) => {
    const next = new Set(expanded);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    setExpanded(next);
    updateTurnChangeViewState(sessionId, summary.id, { expandedKeys: [...next] });
  };

  return (
    <div className="pb-4">
      <section
        className="mt-2 overflow-hidden rounded-lg border border-border/70 bg-card/70 text-card-foreground"
        data-testid="turn-changed-files-card"
        data-change-set-id={summary.id}
        data-fallback-anchor={summary.fallback_anchor}
      >
        <TurnChangesCardHeader
          summary={summary}
          allExpanded={allExpanded}
          hasRows={hasRows}
          hasFolders={tree.some((repository) => repository.folders.length > 0) || tree.length > 1}
          isExpired={isExpired}
          isPending={isPending}
          isUnavailable={isUnavailable}
          onToggleAll={toggleAll}
          onOpenDiff={() => onOpenDiff({ sessionId, changeSetId: summary.id })}
        />
        <TurnChangesAvailability
          isExpired={isExpired}
          isPending={isPending}
          isUnavailable={isUnavailable}
          loading={loading}
          hasRows={hasRows}
          hasError={Boolean(error)}
        />
        {Boolean(error) && <RetryFilesButton onClick={() => void retry()} />}
        <RepositoryRows
          sessionId={sessionId}
          summary={summary}
          tree={tree}
          filesByRepository={filesByRepository}
          fileTotalsByRepository={fileTotalsByRepository}
          expanded={expanded}
          onToggle={toggleExpanded}
          onOpenDiff={onOpenDiff}
        />
        {hasMore && !isExpired && (
          <LoadMoreButton loading={loading} onClick={() => void loadMore()} />
        )}
        {(summary.overlap_intervals?.length ?? 0) > 0 && (
          <CardNotice role="status" text={t("task:turnChangesSharedCheckout")} />
        )}
      </section>
    </div>
  );
});

function TurnChangesCardHeader({
  summary,
  allExpanded,
  hasRows,
  hasFolders,
  isExpired,
  isPending,
  isUnavailable,
  onToggleAll,
  onOpenDiff,
}: {
  summary: TurnChangeSetSummary;
  allExpanded: boolean;
  hasRows: boolean;
  hasFolders: boolean;
  isExpired: boolean;
  isPending: boolean;
  isUnavailable: boolean;
  onToggleAll: () => void;
  onOpenDiff: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 border-b border-border/60 px-3 py-2 sm:flex-row sm:items-center sm:gap-3">
      <TurnChangesCardSummary
        summary={summary}
        isUnavailable={isUnavailable}
        isExpired={isExpired}
      />
      <div className="flex shrink-0 gap-1.5">
        {hasFolders && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-7 min-h-7 cursor-pointer px-2 text-xs max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            onClick={onToggleAll}
            disabled={!hasRows}
          >
            {allExpanded ? t("task:turnChangesCollapseAll") : t("task:turnChangesExpandAll")}
          </Button>
        )}
        <Button
          type="button"
          size="sm"
          variant="secondary"
          className="h-7 min-h-7 cursor-pointer px-2 text-xs max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={onOpenDiff}
          disabled={isExpired || isPending || isUnavailable || summary.file_count === 0}
          data-turn-change-open-diff
        >
          {t("task:turnChangesOpenDiff")}
        </Button>
      </div>
    </div>
  );
}

function TurnChangesCardSummary({
  summary,
  isUnavailable,
  isExpired,
}: {
  summary: TurnChangeSetSummary;
  isUnavailable: boolean;
  isExpired: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 text-xs">
      <TurnChangesCardCounts summary={summary} isUnavailable={isUnavailable} />
      {!isUnavailable && isTurnChangePartial(summary) && (
        <span className="text-amber-700 dark:text-amber-400">{t("task:turnChangesPartial")}</span>
      )}
      {isExpired && <span className="text-muted-foreground">{t("task:turnChangesExpired")}</span>}
    </div>
  );
}

function TurnChangesCardCounts({
  summary,
  isUnavailable,
}: {
  summary: TurnChangeSetSummary;
  isUnavailable: boolean;
}) {
  const { t } = useTranslation();
  if (isUnavailable) {
    return null;
  }
  const countText = summaryCountText(summary, t);
  return (
    <>
      <span className="font-medium">{countText}</span>
      {(summary.added_lines != null || summary.deleted_lines != null) && (
        <LineStat added={summary.added_lines} removed={summary.deleted_lines} />
      )}
      {summary.binary_file_count > 0 && (
        <span className="text-muted-foreground">
          {t("task:turnChangesBinaryCount", { count: summary.binary_file_count })}
        </span>
      )}
      {summary.unknown_count_file_count > 0 && (
        <span className="text-muted-foreground">
          {t("task:turnChangesUnknownCount", { count: summary.unknown_count_file_count })}
        </span>
      )}
    </>
  );
}

function TurnChangesAvailability({
  isExpired,
  isPending,
  isUnavailable,
  loading,
  hasRows,
  hasError,
}: {
  isExpired: boolean;
  isPending: boolean;
  isUnavailable: boolean;
  loading: boolean;
  hasRows: boolean;
  hasError: boolean;
}) {
  const { t } = useTranslation();
  if (isPending) return <CardNotice role="status" text={t("task:turnChangesPending")} />;
  if (isExpired) return <CardNotice role="status" text={t("task:turnChangesExpiredDetail")} />;
  if (isUnavailable) return <CardNotice role="status" text={t("task:turnChangesUnavailable")} />;
  if (hasError) return <CardNotice role="alert" text={t("task:turnChangesLoadFailed")} />;
  if (loading && !hasRows) return <CardNotice role="status" text={t("task:turnChangesLoading")} />;
  if (!loading && !hasRows) return <CardNotice text={t("task:turnChangesNoFiles")} />;
  return null;
}

function RepositoryRows({
  sessionId,
  summary,
  tree,
  filesByRepository,
  fileTotalsByRepository,
  expanded,
  onToggle,
  onOpenDiff,
}: {
  sessionId: string;
  summary: TurnChangeSetSummary;
  tree: ReturnType<typeof buildTurnChangeTree>;
  filesByRepository: Record<string, TurnFileChange[]>;
  fileTotalsByRepository: Record<string, number>;
  expanded: Set<string>;
  onToggle: (key: string) => void;
  onOpenDiff: (target: HistoricalTurnDiffTarget) => void;
}) {
  const { t } = useTranslation();
  return tree.map((repository) => {
    const isExpanded = expanded.has(repository.id);
    const files = filesByRepository[repository.id] ?? [];
    const repo = summary.repositories.find((item) => item.id === repository.id);
    const repositoryState = repo ? projectTurnRepositoryAvailability(repo) : null;
    const repositoryCount = repositoryCountText(
      repo,
      files.length,
      fileTotalsByRepository[repository.id],
      t,
    );
    return (
      <div key={repository.id} className="border-b border-border/40 last:border-b-0">
        <button
          type="button"
          className="flex min-h-7 w-full min-w-0 flex-wrap cursor-pointer items-center gap-2 px-3 text-left text-xs font-medium hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          aria-expanded={isExpanded}
          onClick={() => onToggle(repository.id)}
        >
          {isExpanded ? (
            <IconChevronDown className="size-3.5" />
          ) : (
            <IconChevronRight className="size-3.5" />
          )}
          <span className="min-w-0 flex-1 truncate">{repository.label}</span>
          {repositoryCount && (
            <span className="ml-auto shrink-0 text-muted-foreground">{repositoryCount}</span>
          )}
          {repositoryState && repositoryState !== "ready" && (
            <span className="text-amber-700 dark:text-amber-400">
              {t("task:turnChangesRepositoryStatus", {
                status: t(`task:turnChangesStatus_${repositoryState}`),
              })}
            </span>
          )}
        </button>
        {isExpanded && (
          <div className="pb-1">
            {repository.folders.map((folder) => (
              <FolderRows
                key={folder.path}
                folder={folder}
                repositoryChangeId={repository.id}
                sessionId={sessionId}
                summary={summary}
                expanded={expanded}
                onToggle={onToggle}
                onOpenFile={onOpenDiff}
                depth={0}
                t={t}
              />
            ))}
            {repository.files.map(({ file }) => (
              <FileRow
                key={file.id}
                file={file}
                depth={0}
                onOpen={() => onOpenDiff(targetForFile(sessionId, summary, repository.id, file))}
                t={t}
              />
            ))}
          </div>
        )}
      </div>
    );
  });
}
