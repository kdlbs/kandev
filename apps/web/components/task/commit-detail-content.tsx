"use client";

import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type MutableRefObject,
} from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { FileDiffViewer } from "@/components/diff";
import {
  CollapsibleFileHeader,
  splitCollapsibleFilePath,
} from "@/components/diff/collapsible-file-header";
import { DEFAULT_DIFF_WORD_WRAP } from "@/components/diff/diff-defaults";
import { useAppStore } from "@/components/state-provider";
import { useGlobalFolding } from "@/components/editors/monaco/use-global-folding";
import { useEditorProvider } from "@/hooks/use-editor-resolver";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskRepositories } from "@/hooks/domains/kanban/use-task-repositories";
import { formatRelativeTime } from "@/lib/utils";
import type { FileInfo } from "@/lib/state/store";
import type { CommitDetailTarget, CommitFileNavigationRequest } from "./changes-diff-target";
import { CommitFileToolbar } from "./commit-file-toolbar";
import { useTranslation } from "react-i18next";

export type CommitDetailHeaderCommit = {
  authorName: string;
  commitMessage: string;
  committedAt: string;
  repositoryName?: string;
};

export type CommitDetailContentProps = {
  target: CommitDetailTarget;
  fileEntries: [string, FileInfo][];
  commit?: CommitDetailHeaderCommit;
  sessionId?: string | null;
  initialWordWrap?: boolean;
  onOpenFile?: (path: string, repo?: string) => void;
  fileNavigation?: CommitFileNavigationRequest | null;
};

function statValue(value: number | undefined, unavailable: string): string {
  return typeof value === "number" ? String(value) : unavailable;
}

function CommitFileStats({ file, testId }: { file: FileInfo; testId?: string }) {
  const { t } = useTranslation();
  const unavailable = t("common:unavailable");
  return (
    <span
      data-testid={testId}
      className="shrink-0 whitespace-nowrap tabular-nums text-[11px] text-muted-foreground"
    >
      <span className="text-emerald-500">+{statValue(file.additions, unavailable)}</span>
      <span aria-hidden="true"> / </span>
      <span className="text-rose-500">-{statValue(file.deletions, unavailable)}</span>
    </span>
  );
}

function CommitHeaderMeta({
  commit,
  commitSha,
}: {
  commit?: CommitDetailHeaderCommit;
  commitSha: string;
}) {
  const values = [
    commit?.authorName?.trim(),
    commit?.committedAt ? formatRelativeTime(commit.committedAt) : undefined,
  ].filter((value): value is string => Boolean(value));
  if (values.length === 0 && !commitSha) return null;
  return (
    <p className="mt-1 text-xs text-muted-foreground">
      {values.map((value, index) => (
        <span key={`${value}-${index}`}>
          {index > 0 && <span className="mx-1.5">&middot;</span>}
          {value}
        </span>
      ))}
      {commitSha && (
        <>
          {values.length > 0 && <span className="mx-1.5">&middot;</span>}
          <code className="font-mono text-[11px]">{commitSha.slice(0, 7)}</code>
        </>
      )}
    </p>
  );
}

function CommitHeader({
  commit,
  commitSha,
  showRepository,
  repositoryName,
}: {
  commit?: CommitDetailHeaderCommit;
  commitSha: string;
  showRepository: boolean;
  repositoryName?: string;
}) {
  const { t } = useTranslation();
  const authorName = commit?.authorName?.trim() ?? "";
  const initials = authorName
    .split(/\s+/)
    .map((word) => word[0])
    .filter(Boolean)
    .slice(0, 2)
    .join("")
    .toUpperCase();
  return (
    <div className="mb-4 border-b border-border pb-3" data-testid="commit-header">
      <div className="flex items-start gap-3">
        {initials && (
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
            {initials}
          </div>
        )}
        <div className="min-w-0 flex-1">
          {commit?.commitMessage && (
            <p className="text-sm font-medium leading-snug text-foreground">
              {commit.commitMessage}
            </p>
          )}
          <CommitHeaderMeta commit={commit} commitSha={commitSha} />
          {showRepository && repositoryName && (
            <p
              className="mt-1 truncate text-[11px] text-primary"
              data-testid="commit-repository"
              title={repositoryName}
            >
              {t("common:repository")}: {repositoryName}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

function useSortedFileEntries(fileEntries: [string, FileInfo][]): [string, FileInfo][] {
  return useMemo(
    () => [...fileEntries].sort(([left], [right]) => left.localeCompare(right)),
    [fileEntries],
  );
}

function useCommitFileNavigation(
  fileEntries: [string, FileInfo][],
  externalRequest: CommitFileNavigationRequest | null | undefined,
) {
  const [collapsedPaths, setCollapsedPaths] = useState<Set<string>>(() => new Set());
  const [indexExpanded, setIndexExpanded] = useState(true);
  const [navigationRequest, setNavigationRequest] = useState<CommitFileNavigationRequest | null>(
    null,
  );
  const consumedNavigationTokenRef = useRef<number | null>(null);
  const nextTokenRef = useRef(0);
  const sectionsRef = useRef(new Map<string, HTMLElement>());

  const activateFile = useCallback((path: string, token?: number) => {
    const nextToken = token ?? nextTokenRef.current + 1;
    nextTokenRef.current = Math.max(nextTokenRef.current, nextToken);
    setCollapsedPaths((previous) => {
      if (!previous.has(path)) return previous;
      const next = new Set(previous);
      next.delete(path);
      return next;
    });
    setNavigationRequest({ path, token: nextToken });
  }, []);

  useEffect(() => {
    if (!externalRequest) return;
    activateFile(externalRequest.path, externalRequest.token);
  }, [activateFile, externalRequest?.path, externalRequest?.token]);

  useEffect(() => {
    if (!navigationRequest) return;
    if (consumedNavigationTokenRef.current === navigationRequest.token) return;
    if (!fileEntries.some(([path]) => path === navigationRequest.path)) return;
    const request = navigationRequest;
    const frame = requestAnimationFrame(() => {
      const section = sectionsRef.current.get(request.path);
      if (!section) return;
      const toggle = section.querySelector<HTMLButtonElement>("button[data-file-path]");
      toggle?.focus({ preventScroll: true });
      section.scrollIntoView?.({ behavior: "smooth", block: "start" });
      consumedNavigationTokenRef.current = request.token;
      setNavigationRequest((current) => (current?.token === request.token ? null : current));
    });
    return () => cancelAnimationFrame(frame);
  }, [fileEntries, navigationRequest]);

  return {
    collapsedPaths,
    setCollapsedPaths,
    indexExpanded,
    setIndexExpanded,
    activateFile,
    sectionsRef,
  };
}

type CommitDetailViewModelProps = Pick<
  CommitDetailContentProps,
  "target" | "fileEntries" | "commit" | "initialWordWrap" | "fileNavigation"
>;

function useCommitDetailViewModel({
  target,
  fileEntries: rawFileEntries,
  commit,
  initialWordWrap = DEFAULT_DIFF_WORD_WRAP,
  fileNavigation,
}: CommitDetailViewModelProps) {
  const { t } = useTranslation();
  const fileEntries = useSortedFileEntries(rawFileEntries);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const taskRepositories = useTaskRepositories(activeTaskId);
  const [wordWrap, setWordWrap] = useState(initialWordWrap);
  const editorProvider = useEditorProvider("diff-viewer");
  const [foldUnchanged, setFoldUnchanged] = useGlobalFolding();
  const [expandUnchangedLocal, setExpandUnchangedLocal] = useState(false);
  const expandUnchanged = editorProvider === "monaco" ? !foldUnchanged : expandUnchangedLocal;
  const canExpandUnchanged = editorProvider === "monaco" || target.source === "local";
  const {
    collapsedPaths,
    setCollapsedPaths,
    indexExpanded,
    setIndexExpanded,
    activateFile,
    sectionsRef,
  } = useCommitFileNavigation(fileEntries, fileNavigation);
  const repositoryName =
    target.source === "github"
      ? `${target.owner}/${target.repo}`
      : (target.repo ?? commit?.repositoryName);
  const repositoryLinkName =
    target.source === "github" ? (target.repositoryName ?? target.repo) : repositoryName;
  const showRepository = taskRepositories.length > 1;
  const repositoryLabel = repositoryName ?? (showRepository ? t("common:unavailable") : undefined);
  const emptyFiles = fileEntries.length === 0;
  const listId = useId();
  const toggleFile = useCallback(
    (path: string) => {
      setCollapsedPaths((previous) => {
        const next = new Set(previous);
        if (next.has(path)) next.delete(path);
        else next.add(path);
        return next;
      });
    },
    [setCollapsedPaths],
  );
  const toggleWordWrap = useCallback(() => setWordWrap((previous) => !previous), []);
  const toggleExpandUnchanged = useCallback(() => {
    if (editorProvider === "monaco") {
      setFoldUnchanged(!foldUnchanged);
      return;
    }
    setExpandUnchangedLocal((previous) => !previous);
  }, [editorProvider, foldUnchanged, setFoldUnchanged]);
  const toggleIndex = useCallback(() => setIndexExpanded((previous) => !previous), []);

  return {
    t,
    fileEntries,
    activeTaskId,
    wordWrap,
    expandUnchanged,
    canExpandUnchanged,
    collapsedPaths,
    indexExpanded,
    activateFile,
    sectionsRef,
    repositoryLabel,
    repositoryLinkName,
    showRepository,
    emptyFiles,
    listId,
    toggleFile,
    toggleWordWrap,
    toggleExpandUnchanged,
    toggleIndex,
  };
}

function CommitFileIndex({
  entries,
  expanded,
  onToggle,
  onActivate,
}: {
  entries: [string, FileInfo][];
  expanded: boolean;
  onToggle: () => void;
  onActivate: (path: string) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const indexId = useId();
  const listId = `${indexId}-files`;
  return (
    <aside
      className="mx-3 mb-3 overflow-hidden rounded-md border border-border/70 bg-muted/20"
      data-testid="commit-file-index"
    >
      <button
        type="button"
        className="flex min-h-7 w-full cursor-pointer items-center justify-between gap-2 px-3 text-left text-xs font-medium hover:bg-muted/50 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        aria-expanded={expanded}
        aria-controls={listId}
        data-testid="commit-file-index-toggle"
        onClick={onToggle}
      >
        <span>
          {t("common:files")} ({entries.length})
        </span>
        {expanded ? (
          <IconChevronDown className="size-3.5 text-muted-foreground" />
        ) : (
          <IconChevronRight className="size-3.5 text-muted-foreground" />
        )}
      </button>
      <ol id={listId} hidden={!expanded} className="border-t border-border/50 px-1 py-1">
        {entries.map(([path, file]) => {
          const { directory, name } = splitCollapsibleFilePath(path);
          return (
            <li key={path}>
              <button
                type="button"
                data-testid="commit-file-index-entry"
                data-file-path={path}
                aria-label={path}
                className="flex min-h-7 w-full cursor-pointer items-center gap-2 rounded px-2 text-left text-xs hover:bg-muted/60 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
                onClick={() => onActivate(path)}
              >
                <span className="min-w-0 flex-1" title={path}>
                  {isMobile ? (
                    <span className="flex min-w-0 flex-col justify-center leading-4">
                      <span className="truncate font-medium text-foreground">{name}</span>
                      {directory && (
                        <span className="truncate text-[11px] text-muted-foreground">
                          {directory}
                        </span>
                      )}
                    </span>
                  ) : (
                    <span className="block truncate">{path}</span>
                  )}
                </span>
                <CommitFileStats file={file} testId={`commit-file-index-stats-${path}`} />
              </button>
            </li>
          );
        })}
      </ol>
    </aside>
  );
}

type CommitFileSectionsProps = {
  target: CommitDetailTarget;
  entries: [string, FileInfo][];
  collapsedPaths: ReadonlySet<string>;
  onToggleFile: (path: string) => void;
  showRepository: boolean;
  repositoryName?: string;
  repositoryLinkName?: string;
  listId: string;
  sectionsRef: MutableRefObject<Map<string, HTMLElement>>;
  taskId?: string | null;
  sessionId?: string | null;
  canExpandUnchanged: boolean;
  wordWrap: boolean;
  onToggleWordWrap: () => void;
  expandUnchanged: boolean;
  onToggleExpandUnchanged: () => void;
  onOpenFile?: (path: string, repo?: string) => void;
};

function CommitFileSections({
  target,
  entries,
  collapsedPaths,
  onToggleFile,
  showRepository,
  repositoryName,
  repositoryLinkName,
  listId,
  sectionsRef,
  taskId,
  sessionId,
  canExpandUnchanged,
  wordWrap,
  onToggleWordWrap,
  expandUnchanged,
  onToggleExpandUnchanged,
  onOpenFile,
}: CommitFileSectionsProps) {
  const { t } = useTranslation();
  const localRepo = target.source === "local" ? target.repo : undefined;

  return (
    <div data-testid="commit-file-list">
      {entries.map(([path, file], index) => {
        const collapsed = collapsedPaths.has(path);
        const contentId = `${listId}-content-${index}`;
        return (
          <section
            key={path}
            ref={(node) => {
              if (node) sectionsRef.current.set(path, node);
              else sectionsRef.current.delete(path);
            }}
            data-testid="commit-file-section"
            data-file-path={path}
            className="mb-2"
          >
            <div className="sticky top-0 z-10 border-b border-border/50 bg-card/95 backdrop-blur-sm md:flex md:items-center md:gap-2 md:px-4 md:py-2">
              <CollapsibleFileHeader
                filePath={path}
                repositoryName={showRepository ? repositoryName : undefined}
                status={file.status}
                oldPath={file.old_path}
                collapsed={collapsed}
                expandLabel={t("review:expandFile", { path })}
                collapseLabel={t("review:collapseFile", { path })}
                onToggleCollapse={() => onToggleFile(path)}
                controlsId={contentId}
                mobileStats={<CommitFileStats file={file} />}
                desktopStats={<CommitFileStats file={file} />}
                actions={
                  <CommitFileToolbar
                    filePath={path}
                    diff={file.diff ?? ""}
                    wordWrap={wordWrap}
                    onToggleWordWrap={onToggleWordWrap}
                    expandUnchanged={canExpandUnchanged ? expandUnchanged : undefined}
                    onToggleExpandUnchanged={
                      canExpandUnchanged ? onToggleExpandUnchanged : undefined
                    }
                    onOpenFile={target.source === "local" ? onOpenFile : undefined}
                    repo={repositoryLinkName}
                    taskId={taskId}
                    sessionId={target.source === "local" ? sessionId : undefined}
                    status={file.status}
                    previousPath={file.old_path}
                  />
                }
              />
            </div>
            <div id={contentId} hidden={collapsed}>
              {file.diff ? (
                <FileDiffViewer
                  filePath={path}
                  diff={file.diff}
                  status={file.status}
                  hideHeader
                  sessionId={target.source === "local" ? (sessionId ?? undefined) : undefined}
                  onOpenFile={target.source === "local" ? onOpenFile : undefined}
                  enableExpansion={target.source === "local"}
                  baseRef={target.source === "local" ? `${target.sha}^` : undefined}
                  repo={localRepo}
                  wordWrap={wordWrap}
                  expandUnchanged={canExpandUnchanged ? expandUnchanged : undefined}
                  onToggleExpandUnchanged={canExpandUnchanged ? onToggleExpandUnchanged : undefined}
                />
              ) : (
                <div className="px-3 py-2 text-xs text-muted-foreground">
                  {t("task:binaryOrEmptyDiff", { path })}
                </div>
              )}
            </div>
          </section>
        );
      })}
    </div>
  );
}

export function CommitDetailContent({
  target,
  fileEntries,
  commit,
  sessionId,
  initialWordWrap,
  onOpenFile,
  fileNavigation,
}: CommitDetailContentProps) {
  const {
    t,
    fileEntries: sortedFileEntries,
    activeTaskId,
    wordWrap,
    expandUnchanged,
    canExpandUnchanged,
    collapsedPaths,
    indexExpanded,
    activateFile,
    sectionsRef,
    repositoryLabel,
    repositoryLinkName,
    showRepository,
    emptyFiles,
    listId,
    toggleFile,
    toggleWordWrap,
    toggleExpandUnchanged,
    toggleIndex,
  } = useCommitDetailViewModel({
    target,
    fileEntries,
    commit,
    initialWordWrap,
    fileNavigation,
  });

  return (
    <div data-testid="commit-detail-content">
      {(commit || (showRepository && repositoryLabel)) && (
        <div className="p-3">
          <CommitHeader
            commit={commit}
            commitSha={target.sha}
            showRepository={showRepository}
            repositoryName={repositoryLabel}
          />
        </div>
      )}
      {!emptyFiles && (
        <CommitFileIndex
          entries={sortedFileEntries}
          expanded={indexExpanded}
          onToggle={toggleIndex}
          onActivate={activateFile}
        />
      )}
      {emptyFiles ? (
        <div className="px-3 py-8 text-center text-sm text-muted-foreground">
          {t("task:noFilesInThisCommit")}
        </div>
      ) : (
        <CommitFileSections
          target={target}
          entries={sortedFileEntries}
          collapsedPaths={collapsedPaths}
          onToggleFile={toggleFile}
          showRepository={showRepository}
          repositoryName={repositoryLabel}
          repositoryLinkName={repositoryLinkName}
          listId={listId}
          sectionsRef={sectionsRef}
          taskId={activeTaskId}
          sessionId={sessionId}
          canExpandUnchanged={canExpandUnchanged}
          wordWrap={wordWrap}
          onToggleWordWrap={toggleWordWrap}
          expandUnchanged={expandUnchanged}
          onToggleExpandUnchanged={toggleExpandUnchanged}
          onOpenFile={onOpenFile}
        />
      )}
    </div>
  );
}
