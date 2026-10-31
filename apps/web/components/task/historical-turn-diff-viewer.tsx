"use client";

import { memo, useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { PanelBody, PanelRoot } from "./panel-primitives";
import {
  FileSelectionControls,
  HistoricalScopeSelect,
  HistoricalDiffLoadMoreButton,
  HistoricalViewerHeader,
} from "./historical-turn-diff-viewer-controls";
import { FileDiffViewer } from "@/components/diff/file-diff-viewer";
import { useSessionTurnChanges } from "@/hooks/domains/session/use-turn-changes";
import {
  useTurnChangeContent,
  useTurnChangeFiles,
} from "@/hooks/domains/session/use-turn-change-files";
import { ApiError } from "@/lib/api/client";
import type { HistoricalTurnDiffTarget } from "@/lib/state/diff-target-types";
import type { TurnChangeSetSummary, TurnFileChange } from "@/lib/types/turn-changes";
import { readTurnChangeViewState, updateTurnChangeViewState } from "@/lib/turn-changes/view-state";
import { resolveTurnChangeScope } from "@/lib/turn-changes/history-scope";
import { contentUnavailableText, isSelectedContentPending } from "@/lib/turn-changes/content-copy";
import { isUnavailableHistoryComparison } from "@/lib/turn-changes/projection";
import { emptyHistoricalSummary } from "@/lib/turn-changes/empty-summary";
import { useReadyContentRetry } from "@/hooks/domains/session/use-ready-content-retry";
import { useTranslation } from "react-i18next";

function useHistoricalChangeSet(target: HistoricalTurnDiffTarget) {
  const [activeChangeSetId, setActiveChangeSetId] = useState(target.changeSetId);
  const [scopeValue, setScopeValue] = useState(target.changeSetId);
  useEffect(() => {
    setActiveChangeSetId(target.changeSetId);
    setScopeValue(target.changeSetId);
  }, [target.changeSetId]);
  const activeTarget = useMemo<HistoricalTurnDiffTarget>(
    () =>
      activeChangeSetId === target.changeSetId
        ? target
        : { sessionId: target.sessionId, changeSetId: activeChangeSetId },
    [activeChangeSetId, target],
  );
  return { activeChangeSetId, setActiveChangeSetId, scopeValue, setScopeValue, activeTarget };
}

function statusFromKind(kind: string): string {
  switch (kind) {
    case "added":
      return "A";
    case "deleted":
      return "D";
    case "renamed":
      return "R";
    case "copied":
      return "C";
    default:
      return "M";
  }
}

function flattenFiles(
  summary: TurnChangeSetSummary,
  filesByRepository: Record<string, TurnFileChange[]>,
  target: HistoricalTurnDiffTarget,
): TurnFileChange[] {
  const repositories = target.repositoryChangeId
    ? summary.repositories.filter((repository) => repository.id === target.repositoryChangeId)
    : summary.repositories;
  return repositories.flatMap((repository) => filesByRepository[repository.id] ?? []);
}

function isExpiredError(error: unknown): boolean {
  return error instanceof ApiError && error.status === 410;
}

function useSelectedFile(files: TurnFileChange[], target: HistoricalTurnDiffTarget) {
  const [selectedFile, setSelectedFile] = useState<TurnFileChange | null>(null);
  const [whitespace, setWhitespace] = useState(false);
  const [initialized, setInitialized] = useState(false);
  useEffect(() => {
    const saved = readTurnChangeViewState(target.sessionId, target.changeSetId);
    const selected = targetFile(target, saved);
    setSelectedFile(selected);
    setWhitespace(saved?.ignoreWhitespace ?? false);
    setInitialized(true);
    if (selected) persistSelectedFile(target, selected);
  }, [
    target.changeSetId,
    target.checkoutId,
    target.fileChangeId,
    target.fileKind,
    target.oldPath,
    target.path,
    target.repositoryChangeId,
    target.sessionId,
  ]);
  useEffect(() => {
    if (!initialized || files.length === 0) return;
    if (!selectedFile) {
      const fallback = files[0]!;
      setSelectedFile(fallback);
      persistSelectedFile(target, fallback);
      return;
    }
    const canonical = files.find((file) => file.id === selectedFile.id);
    if (canonical && canonical !== selectedFile) setSelectedFile(canonical);
  }, [files, initialized, selectedFile, target.changeSetId, target.sessionId]);
  const selectFile = (file: TurnFileChange) => {
    setSelectedFile(file);
    persistSelectedFile(target, file);
  };
  const setIgnoreWhitespace = (value: boolean) => {
    setWhitespace(value);
    updateTurnChangeViewState(target.sessionId, target.changeSetId, { ignoreWhitespace: value });
  };
  return { selectedFile, selectFile, whitespace, setIgnoreWhitespace };
}

function targetFile(
  target: HistoricalTurnDiffTarget,
  saved: ReturnType<typeof readTurnChangeViewState>,
): TurnFileChange | null {
  const id = target.fileChangeId ?? saved?.selectedFileId;
  const path = target.path ?? saved?.selectedFilePath;
  if (!id || !path) return null;
  const oldPath = oldPathForTarget(target, saved);
  const file: TurnFileChange = {
    id,
    repository_change_id: firstDefined(
      target.repositoryChangeId,
      saved?.selectedRepositoryChangeId,
      "",
    ),
    checkout_id: firstDefined(target.checkoutId, saved?.selectedCheckoutId, ""),
    path,
    kind: firstDefined(target.fileKind, saved?.selectedFileKind, "modified"),
    content_availability: "pending",
    synthetic: true,
  };
  if (oldPath) file.old_path = oldPath;
  return file;
}

function firstDefined<T>(value: T | undefined, fallback: T | undefined, defaultValue: T): T {
  return value ?? fallback ?? defaultValue;
}

function oldPathForTarget(
  target: HistoricalTurnDiffTarget,
  saved: ReturnType<typeof readTurnChangeViewState>,
): string | undefined {
  if (target.oldPath !== undefined) return target.oldPath ?? undefined;
  return saved?.selectedFileOldPath ?? undefined;
}

function repositoryForFile(summary: TurnChangeSetSummary | undefined, file: TurnFileChange | null) {
  return summary?.repositories.find((repository) => repository.id === file?.repository_change_id);
}

function loadingLabel(t: (key: string) => string): string {
  return t("task:turnChangesLoading");
}
function buildSelectionOptions(selectedFile: TurnFileChange | null, available: TurnFileChange[]) {
  if (selectedFile && !available.some((file) => file.id === selectedFile.id)) {
    return [selectedFile, ...available];
  }
  return available;
}
function persistSelectedFile(target: HistoricalTurnDiffTarget, file: TurnFileChange) {
  updateTurnChangeViewState(target.sessionId, target.changeSetId, {
    selectedFileId: file.id,
    selectedFilePath: file.path,
    selectedFileKind: file.kind,
    selectedFileOldPath: file.old_path ?? null,
    selectedRepositoryChangeId: file.repository_change_id,
    selectedCheckoutId: file.checkout_id,
  });
}

type HistoricalTurnDiffViewerProps = {
  target: HistoricalTurnDiffTarget;
  onClose?: () => void;
  hideCloseButton?: boolean;
  detailActive?: boolean;
};

export const HistoricalTurnDiffViewer = memo(function HistoricalTurnDiffViewer({
  target,
  onClose,
  hideCloseButton = false,
  detailActive = true,
}: HistoricalTurnDiffViewerProps) {
  const {
    summaries,
    loading: historyLoading,
    error: historyError,
    reload,
    loadChangeSet,
    hasMore: hasMoreHistory,
    loadMore,
  } = useSessionTurnChanges(target.sessionId, { detailActive });
  const { activeChangeSetId, setActiveChangeSetId, scopeValue, setScopeValue, activeTarget } =
    useHistoricalChangeSet(target);
  const summary = summaries.find((item) => item.id === activeChangeSetId);
  useEffect(() => {
    if (!summary) void loadChangeSet(activeChangeSetId);
  }, [activeChangeSetId, loadChangeSet, summary]);
  const files = useTurnChangeFiles(
    activeTarget.sessionId,
    summary ?? emptyHistoricalSummary(activeTarget),
  );
  const availableFiles = useMemo(
    () => (summary ? flattenFiles(summary, files.filesByRepository, activeTarget) : []),
    [activeTarget, files.filesByRepository, summary],
  );
  const { selectedFile, selectFile, whitespace, setIgnoreWhitespace } = useSelectedFile(
    availableFiles,
    activeTarget,
  );
  const [retryKey, setRetryKey] = useState(0);
  const content = useTurnChangeContent(
    activeTarget,
    selectedFile?.id ?? null,
    whitespace,
    retryKey,
  );
  const selectionOptions = buildSelectionOptions(selectedFile, availableFiles);
  const expired = isExpiredError(content.error) || summary?.availability === "expired";
  const pendingContent =
    !expired &&
    isSelectedContentPending(selectedFile, summary, content.patch !== null, content.error);
  const unavailable = Boolean(content.error && !expired && !pendingContent);
  const selectedRepository = repositoryForFile(summary, selectedFile);
  useReadyContentRetry({
    selectedFile,
    contentError: content.error,
    contentLoading: content.loading,
    whitespace,
    repositorySettled: selectedRepository?.export_state === "complete",
    retry: () => setRetryKey((key) => key + 1),
  });
  const handleScopeSelect = (value: string) => {
    const selection = resolveTurnChangeScope(value, summaries);
    if (!selection) return;
    if (selection.kind === "current") {
      onClose?.();
      return;
    }
    setActiveChangeSetId(selection.changeSetId);
    setScopeValue(selection.scopeValue);
  };
  const handleFileSelect = (fileId: string) => {
    const file = selectionOptions.find((candidate) => candidate.id === fileId);
    if (file) selectFile(file);
  };
  return (
    <HistoricalViewerLayout
      changeSetId={activeChangeSetId}
      summaries={summaries}
      historyLoading={historyLoading}
      hasMoreHistory={hasMoreHistory}
      loadMore={loadMore}
      scopeValue={scopeValue}
      onScopeSelect={handleScopeSelect}
      summary={summary}
      onClose={hideCloseButton ? undefined : onClose}
      files={selectionOptions}
      selectedFile={selectedFile}
      onFileSelect={handleFileSelect}
      whitespace={whitespace}
      onWhitespace={setIgnoreWhitespace}
      content={content}
      fileState={files}
      historyError={historyError}
      reload={reload}
      expired={expired}
      unavailable={unavailable}
      pendingContent={pendingContent}
      onRetryContent={() => setRetryKey((key) => key + 1)}
    />
  );
});

function HistoricalViewerLayout({
  changeSetId,
  summaries,
  historyLoading,
  hasMoreHistory,
  loadMore,
  scopeValue,
  onScopeSelect,
  summary,
  onClose,
  files,
  selectedFile,
  onFileSelect,
  whitespace,
  onWhitespace,
  content,
  fileState,
  historyError,
  reload,
  expired,
  unavailable,
  pendingContent,
  onRetryContent,
}: {
  changeSetId: string;
  summaries: TurnChangeSetSummary[];
  historyLoading: boolean;
  hasMoreHistory: boolean;
  loadMore: () => void;
  scopeValue: string;
  onScopeSelect: (value: string) => void;
  summary?: TurnChangeSetSummary;
  onClose?: () => void;
  files: TurnFileChange[];
  selectedFile: TurnFileChange | null;
  onFileSelect: (fileId: string) => void;
  whitespace: boolean;
  onWhitespace: (value: boolean) => void;
  content: ReturnType<typeof useTurnChangeContent>;
  fileState: ReturnType<typeof useTurnChangeFiles>;
  historyError: unknown;
  reload: () => Promise<void>;
  expired: boolean;
  unavailable: boolean;
  pendingContent: boolean;
  onRetryContent: () => void;
}) {
  return (
    <PanelRoot
      className="flex h-full min-h-0 flex-col overflow-hidden"
      data-testid="historical-turn-diff"
      data-change-set-id={changeSetId}
    >
      <HistoricalScopeSelect
        summaries={summaries}
        loading={historyLoading}
        hasMore={hasMoreHistory}
        onLoadMore={loadMore}
        onSelect={onScopeSelect}
        scopeValue={scopeValue}
      />
      <HistoricalViewerHeader summary={summary} onClose={onClose} />
      <FileSelectionControls
        files={files}
        repositories={summary?.repositories ?? []}
        selectedFile={selectedFile}
        onSelect={onFileSelect}
        whitespace={whitespace}
        onWhitespace={onWhitespace}
      />
      <HistoricalViewerBody
        summary={summary}
        selectedFile={selectedFile}
        content={content}
        files={fileState}
        historyLoading={historyLoading}
        historyError={historyError}
        reload={reload}
        expired={expired}
        unavailable={unavailable}
        pendingContent={pendingContent}
        onRetryContent={onRetryContent}
      />
    </PanelRoot>
  );
}

type ViewerNotices = {
  key: string;
  text: string;
  role?: "status" | "alert";
  retry?: "history" | "content" | "files";
}[];

function HistoricalViewerBody({
  summary,
  selectedFile,
  content,
  files,
  historyLoading,
  historyError,
  reload,
  expired,
  unavailable,
  pendingContent,
  onRetryContent,
}: {
  summary?: TurnChangeSetSummary;
  selectedFile: TurnFileChange | null;
  content: ReturnType<typeof useTurnChangeContent>;
  files: ReturnType<typeof useTurnChangeFiles>;
  historyLoading: boolean;
  historyError: unknown;
  reload: () => Promise<void>;
  expired: boolean;
  unavailable: boolean;
  pendingContent: boolean;
  onRetryContent: () => void;
}) {
  const { t } = useTranslation();
  const notices = viewerNotices(
    {
      summary,
      selectedFile,
      content,
      historyLoading,
      historyError,
      filesLoading: files.loading,
      filesError: files.error,
      expired,
      unavailable,
      pendingContent,
    },
    t,
  );
  const diff =
    selectedFile && content.patch !== null && !expired && !content.error ? (
      <FileDiffViewer
        filePath={selectedFile.path}
        previousPath={selectedFile.old_path}
        diff={content.patch}
        status={statusFromKind(selectedFile.kind)}
        wordWrap
        className="min-h-full"
      />
    ) : null;
  return (
    <PanelBody
      padding={false}
      scroll={false}
      className="min-h-0 flex-1 overflow-auto pb-[env(safe-area-inset-bottom,0px)]"
    >
      {notices.map((notice) => (
        <ViewerNotice
          key={notice.key}
          notice={notice}
          onRetryHistory={() => void reload()}
          onRetryFiles={() => void files.retry()}
          onRetryContent={onRetryContent}
        />
      ))}
      {diff}
      {files.hasMore && (
        <HistoricalDiffLoadMoreButton
          loading={files.loading}
          onClick={() => void files.loadMore()}
        />
      )}
    </PanelBody>
  );
}

function viewerNotices(
  state: {
    summary?: TurnChangeSetSummary;
    selectedFile: TurnFileChange | null;
    content: ReturnType<typeof useTurnChangeContent>;
    historyLoading: boolean;
    historyError: unknown;
    filesLoading: boolean;
    filesError: unknown;
    expired: boolean;
    unavailable: boolean;
    pendingContent: boolean;
  },
  t: (key: string) => string,
): ViewerNotices {
  return [...historyNotices(state, t), ...contentNotices(state, t)];
}

function historyNotices(
  state: Parameters<typeof viewerNotices>[0],
  t: (key: string) => string,
): ViewerNotices {
  const notices: ViewerNotices = [];
  if (state.historyLoading && !state.summary) {
    notices.push({ key: "history-loading", text: loadingLabel(t), role: "status" });
  }
  if (state.historyError) {
    notices.push({
      key: "history-error",
      text: t("task:turnChangesLoadFailed"),
      role: "alert",
      retry: "history",
    });
  }
  if (state.filesError && !state.expired) {
    notices.push({
      key: "files-error",
      text: t("task:turnChangesLoadFailed"),
      role: "alert",
      retry: "files",
    });
  }
  if (state.expired) {
    notices.push({ key: "expired", text: t("task:turnChangesExpiredDetail"), role: "status" });
  }
  if (state.summary && !state.expired && isUnavailableHistoryComparison(state.summary)) {
    notices.push({ key: "unavailable", text: t("task:turnChangesUnavailable"), role: "status" });
    return notices;
  }
  if (!state.expired && !state.summary && !state.historyLoading && !state.historyError) {
    notices.push({ key: "unavailable", text: t("task:turnChangesUnavailable"), role: "status" });
  }
  return notices;
}

function contentNotices(
  state: Parameters<typeof viewerNotices>[0],
  t: (key: string) => string,
): ViewerNotices {
  const pending = pendingContentNotice(state, t);
  if (pending) return [pending];
  const notices: ViewerNotices = [];
  const loading = contentLoadingNotice(state, t);
  if (loading) notices.push(loading);
  if (state.summary && isUnavailableHistoryComparison(state.summary)) {
    return notices;
  }
  if (state.summary && state.unavailable) {
    notices.push({
      key: "content-error",
      text: contentUnavailableText(state.selectedFile, t),
      role: "alert",
      retry: "content",
    });
  }
  const empty = emptyFilesNotice(state, t);
  if (empty) notices.push(empty);
  return notices;
}

function pendingContentNotice(
  state: { summary?: TurnChangeSetSummary; expired: boolean; pendingContent: boolean },
  t: (key: string) => string,
): ViewerNotices[number] | null {
  if (!state.summary || state.expired || !state.pendingContent) return null;
  return { key: "content-pending", text: loadingLabel(t), role: "status" };
}

function contentLoadingNotice(
  state: {
    summary?: TurnChangeSetSummary;
    expired: boolean;
    content: ReturnType<typeof useTurnChangeContent>;
  },
  t: (key: string) => string,
): ViewerNotices[number] | null {
  if (!state.summary || state.expired || state.content.patch || !state.content.loading) return null;
  return { key: "content-loading", text: loadingLabel(t), role: "status" };
}

function emptyFilesNotice(
  state: {
    summary?: TurnChangeSetSummary;
    expired: boolean;
    content: ReturnType<typeof useTurnChangeContent>;
    filesLoading: boolean;
    filesError: unknown;
    selectedFile: TurnFileChange | null;
  },
  t: (key: string) => string,
): ViewerNotices[number] | null {
  if (!state.summary || state.expired || state.content.loading || state.content.error) return null;
  if (state.filesError || state.selectedFile) return null;
  return {
    key: "files-empty",
    text: state.filesLoading ? loadingLabel(t) : t("task:turnChangesNoFiles"),
  };
}

function ViewerNotice({
  notice,
  onRetryHistory,
  onRetryFiles,
  onRetryContent,
}: {
  notice: ViewerNotices[number];
  onRetryHistory: () => void;
  onRetryFiles: () => void;
  onRetryContent: () => void;
}) {
  const { t } = useTranslation();
  const retry = { history: onRetryHistory, files: onRetryFiles, content: onRetryContent }[
    notice.retry ?? "content"
  ];
  return (
    <div className="p-4 text-sm text-muted-foreground" role={notice.role}>
      <p>{notice.text}</p>
      {notice.retry && (
        <Button
          size="sm"
          variant="ghost"
          className="mt-2 cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={retry}
        >
          {t("task:turnChangesRetry")}
        </Button>
      )}
    </div>
  );
}
