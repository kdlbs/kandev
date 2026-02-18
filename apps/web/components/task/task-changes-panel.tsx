'use client';

import { memo, useMemo, useCallback, createRef, useState, useEffect, useRef } from 'react';
import { PanelRoot, PanelBody, PanelHeaderBarSplit } from './panel-primitives';
import {
  IconSettings,
  IconTextWrap,
  IconLayoutColumns,
  IconLayoutRows,
  IconMessageForward,
  IconArrowsMaximize,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Checkbox } from '@kandev/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { useToast } from '@/components/toast-provider';
import { useAppStore } from '@/components/state-provider';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useCumulativeDiff } from '@/hooks/domains/session/use-cumulative-diff';
import { useGitOperations } from '@/hooks/use-git-operations';
import { useSessionFileReviews } from '@/hooks/use-session-file-reviews';
import { useDiffCommentsStore } from '@/lib/state/slices/diff-comments/diff-comments-slice';
import { getWebSocketClient } from '@/lib/ws/connection';
import { updateUserSettings } from '@/lib/api';
import { formatReviewCommentsAsMarkdown } from '@/components/task/chat/messages/review-comments-attachment';
import { ReviewDiffList } from '@/components/review/review-diff-list';
import type { ReviewFile } from '@/components/review/types';
import { hashDiff, normalizeDiffContent } from '@/components/review/types';
import { usePanelActions } from '@/hooks/use-panel-actions';
import type { SelectedDiff } from './task-layout';
import { useIsTaskArchived, ArchivedPanelPlaceholder } from './task-archived-context';

type TaskChangesPanelProps = {
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  /** Callback to open file in editor */
  onOpenFile?: (filePath: string) => void;
};

type UncommittedFile = { diff?: string; status?: string; additions?: number; deletions?: number; staged?: boolean };
type CumulativeFile = { diff?: string; status?: string; additions?: number; deletions?: number };

function addUncommittedFiles(fileMap: Map<string, ReviewFile>, files: Record<string, UncommittedFile>) {
  for (const [path, file] of Object.entries(files)) {
    const diff = file.diff ? normalizeDiffContent(file.diff) : '';
    if (diff) {
      fileMap.set(path, { path, diff, status: file.status ?? 'modified', additions: file.additions ?? 0, deletions: file.deletions ?? 0, staged: file.staged ?? false, source: 'uncommitted' });
    }
  }
}

function addCumulativeFiles(fileMap: Map<string, ReviewFile>, files: Record<string, CumulativeFile>) {
  for (const [path, file] of Object.entries(files)) {
    if (fileMap.has(path)) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : '';
    if (diff) {
      fileMap.set(path, { path, diff, status: file.status || 'modified', additions: file.additions ?? 0, deletions: file.deletions ?? 0, staged: false, source: 'committed' });
    }
  }
}

/** Merge uncommitted + committed files into a single sorted list */
function mergeReviewFiles(
  gitStatus: ReturnType<typeof useSessionGitStatus>,
  cumulativeDiff: { files?: Record<string, CumulativeFile> } | null,
): ReviewFile[] {
  const fileMap = new Map<string, ReviewFile>();
  if (gitStatus?.files) addUncommittedFiles(fileMap, gitStatus.files as Record<string, UncommittedFile>);
  if (cumulativeDiff?.files) addCumulativeFiles(fileMap, cumulativeDiff.files);
  return Array.from(fileMap.values()).sort((a, b) => a.path.localeCompare(b.path));
}

function useChangesData(selectedDiff: SelectedDiff | null, onClearSelected: () => void) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { diff: cumulativeDiff, loading: cumulativeLoading } = useCumulativeDiff(activeSessionId);
  const { reviews } = useSessionFileReviews(activeSessionId);
  const bySession = useDiffCommentsStore((s) => s.bySession);

  const allFiles = useMemo<ReviewFile[]>(() => mergeReviewFiles(gitStatus, cumulativeDiff), [gitStatus, cumulativeDiff]);

  const { reviewedFiles, staleFiles } = useMemo(() => {
    const reviewed = new Set<string>();
    const stale = new Set<string>();
    for (const file of allFiles) {
      const reviewState = reviews.get(file.path);
      if (!reviewState?.reviewed) continue;
      const currentHash = hashDiff(file.diff);
      if (reviewState.diffHash && reviewState.diffHash !== currentHash) { stale.add(file.path); } else { reviewed.add(file.path); }
    }
    return { reviewedFiles: reviewed, staleFiles: stale };
  }, [allFiles, reviews]);

  const totalCommentCount = useMemo(() => {
    if (!activeSessionId) return 0;
    const sessionComments = bySession[activeSessionId];
    if (!sessionComments) return 0;
    return Object.values(sessionComments).reduce((sum, c) => sum + (c.length > 0 ? c.length : 0), 0);
  }, [bySession, activeSessionId]);

  const fileRefs = useMemo(() => {
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const file of allFiles) refs.set(file.path, createRef<HTMLDivElement>());
    return refs;
  }, [allFiles]);

  const scrolledRef = useRef<string | null>(null);
  useEffect(() => {
    if (!selectedDiff?.path || scrolledRef.current === selectedDiff.path) return;
    scrolledRef.current = selectedDiff.path;
    const ref = fileRefs.get(selectedDiff.path);
    if (ref?.current) requestAnimationFrame(() => { ref.current?.scrollIntoView({ behavior: 'smooth', block: 'start' }); });
    onClearSelected();
  }, [selectedDiff, fileRefs, onClearSelected]);

  useEffect(() => { if (!selectedDiff) scrolledRef.current = null; }, [selectedDiff]);

  return { activeSessionId, allFiles, reviewedFiles, staleFiles, totalCommentCount, fileRefs, cumulativeLoading };
}

function useChangesActions(activeSessionId: string | null | undefined, allFiles: ReviewFile[]) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const autoMarkOnScroll = useAppStore((s) => s.userSettings.reviewAutoMarkOnScroll);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const userSettings = useAppStore((state) => state.userSettings);
  const { discard } = useGitOperations(activeSessionId ?? null);
  const { markReviewed, markUnreviewed } = useSessionFileReviews(activeSessionId ?? null);
  const getPendingComments = useDiffCommentsStore((s) => s.getPendingComments);
  const markCommentsSent = useDiffCommentsStore((s) => s.markCommentsSent);
  const { toast } = useToast();

  const [splitView, setSplitView] = useState(() => typeof window !== 'undefined' && localStorage.getItem('diff-view-mode') === 'split');
  const [wordWrap, setWordWrap] = useState(false);

  const handleToggleSplitView = useCallback((split: boolean) => {
    setSplitView(split);
    const mode = split ? 'split' : 'unified';
    localStorage.setItem('diff-view-mode', mode);
    window.dispatchEvent(new CustomEvent('diff-view-mode-change', { detail: mode }));
  }, []);

  const handleToggleReviewed = useCallback((path: string, reviewed: boolean) => {
    if (reviewed) { const file = allFiles.find((f) => f.path === path); markReviewed(path, file ? hashDiff(file.diff) : ''); } else { markUnreviewed(path); }
  }, [allFiles, markReviewed, markUnreviewed]);

  const handleDiscard = useCallback(async (path: string) => {
    try {
      const result = await discard([path]);
      if (result.success) { toast({ title: 'Changes discarded', description: path, variant: 'success' }); } else { toast({ title: 'Discard failed', description: result.error || 'An error occurred', variant: 'error' }); }
    } catch (e) { toast({ title: 'Discard failed', description: e instanceof Error ? e.message : 'An error occurred', variant: 'error' }); }
  }, [discard, toast]);

  const handleToggleAutoMark = useCallback((checked: boolean) => {
    const next = { ...userSettings, reviewAutoMarkOnScroll: checked };
    setUserSettings(next);
    const client = getWebSocketClient();
    const payload = { review_auto_mark_on_scroll: checked };
    if (client) { client.request('user.settings.update', payload).catch(() => { updateUserSettings(payload, { cache: 'no-store' }).catch(() => {}); }); } else { updateUserSettings(payload, { cache: 'no-store' }).catch(() => {}); }
  }, [userSettings, setUserSettings]);

  const handleFixComments = useCallback(() => {
    if (!activeSessionId || !activeTaskId) return;
    const comments = getPendingComments();
    if (comments.length === 0) return;
    const markdown = formatReviewCommentsAsMarkdown(comments);
    if (!markdown) return;
    const client = getWebSocketClient();
    if (client) { client.request('message.add', { task_id: activeTaskId, session_id: activeSessionId, content: markdown }).catch(() => { toast({ title: 'Failed to send comments', variant: 'error' }); }); }
    markCommentsSent(comments.map((c) => c.id));
  }, [activeSessionId, activeTaskId, getPendingComments, markCommentsSent, toast]);

  return { splitView, wordWrap, setWordWrap, autoMarkOnScroll, handleToggleSplitView, handleToggleReviewed, handleDiscard, handleToggleAutoMark, handleFixComments };
}

type ChangesTopBarProps = {
  autoMarkOnScroll: boolean;
  splitView: boolean;
  wordWrap: boolean;
  totalCommentCount: number;
  reviewedCount: number;
  totalCount: number;
  progressPercent: number;
  setWordWrap: (v: boolean) => void;
  handleToggleSplitView: (v: boolean) => void;
  handleToggleAutoMark: (v: boolean) => void;
  handleFixComments: () => void;
};

function ChangesTopBar({ autoMarkOnScroll, splitView, wordWrap, totalCommentCount, reviewedCount, totalCount, progressPercent, setWordWrap, handleToggleSplitView, handleToggleAutoMark, handleFixComments }: ChangesTopBarProps) {
  return (
    <PanelHeaderBarSplit
      left={
        <>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="ghost" className="px-1.5 h-5 cursor-pointer"><IconSettings className="h-3.5 w-3.5" /></Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-64">
              <DropdownMenuItem className="cursor-pointer gap-2" onSelect={(e) => { e.preventDefault(); handleToggleAutoMark(!autoMarkOnScroll); }}>
                <Checkbox checked={autoMarkOnScroll} className="pointer-events-none" />
                <span className="text-sm flex-1">Auto-mark reviewed on scroll</span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          {totalCount > 0 && (
            <div className="flex items-center gap-2 min-w-0">
              <div className="w-20 h-1 rounded-full bg-muted overflow-hidden">
                <div className="h-full bg-indigo-500 rounded-full transition-all duration-300" style={{ width: `${progressPercent}%` }} />
              </div>
              <span className="text-[11px] text-muted-foreground whitespace-nowrap">{reviewedCount}/{totalCount} Reviewed</span>
            </div>
          )}
        </>
      }
      right={
        <>
          <Tooltip><TooltipTrigger asChild><Button size="sm" variant="ghost" className="px-1.5 h-5 cursor-pointer" onClick={() => window.dispatchEvent(new CustomEvent('open-review-dialog'))}><IconArrowsMaximize className="h-3.5 w-3.5" /></Button></TooltipTrigger><TooltipContent>Expand review</TooltipContent></Tooltip>
          <Tooltip><TooltipTrigger asChild><Button size="sm" variant="ghost" className={`px-1.5 h-5 cursor-pointer ${wordWrap ? 'bg-muted' : ''}`} onClick={() => setWordWrap(!wordWrap)}><IconTextWrap className="h-3.5 w-3.5" /></Button></TooltipTrigger><TooltipContent>Toggle word wrap</TooltipContent></Tooltip>
          <Tooltip><TooltipTrigger asChild><Button size="sm" variant="ghost" className="px-1.5 h-5 cursor-pointer" onClick={() => handleToggleSplitView(!splitView)}>{splitView ? <IconLayoutRows className="h-3.5 w-3.5" /> : <IconLayoutColumns className="h-3.5 w-3.5" />}</Button></TooltipTrigger><TooltipContent>{splitView ? 'Unified view' : 'Split view'}</TooltipContent></Tooltip>
          {totalCommentCount > 0 && (
            <Button size="sm" variant="outline" className="h-5 text-xs cursor-pointer" onClick={handleFixComments}>
              <IconMessageForward className="h-3.5 w-3.5" />Fix
              <span className="ml-0.5 rounded-full bg-blue-500/30 px-1 py-0 text-[10px] font-medium text-blue-600 dark:text-blue-400">{totalCommentCount}</span>
            </Button>
          )}
        </>
      }
    />
  );
}

const TaskChangesPanel = memo(function TaskChangesPanel({
  selectedDiff,
  onClearSelected,
  onOpenFile: onOpenFileProp,
}: TaskChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const { openFile: panelOpenFile } = usePanelActions();
  const handleOpenFile = onOpenFileProp ?? panelOpenFile;

  const { activeSessionId, allFiles, reviewedFiles, staleFiles, totalCommentCount, fileRefs, cumulativeLoading } = useChangesData(selectedDiff, onClearSelected);
  const { splitView, wordWrap, setWordWrap, autoMarkOnScroll, handleToggleSplitView, handleToggleReviewed, handleDiscard, handleToggleAutoMark, handleFixComments } = useChangesActions(activeSessionId, allFiles);

  const reviewedCount = reviewedFiles.size;
  const totalCount = allFiles.length;
  const progressPercent = totalCount > 0 ? (reviewedCount / totalCount) * 100 : 0;

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      <ChangesTopBar
        autoMarkOnScroll={autoMarkOnScroll}
        splitView={splitView}
        wordWrap={wordWrap}
        totalCommentCount={totalCommentCount}
        reviewedCount={reviewedCount}
        totalCount={totalCount}
        progressPercent={progressPercent}
        setWordWrap={setWordWrap}
        handleToggleSplitView={handleToggleSplitView}
        handleToggleAutoMark={handleToggleAutoMark}
        handleFixComments={handleFixComments}
      />
      <PanelBody padding={false} scroll={false} className="overflow-hidden">
        <ChangesPanelContent
          isLoading={cumulativeLoading}
          allFiles={allFiles}
          activeSessionId={activeSessionId}
          reviewedFiles={reviewedFiles}
          staleFiles={staleFiles}
          autoMarkOnScroll={autoMarkOnScroll}
          wordWrap={wordWrap}
          onToggleReviewed={handleToggleReviewed}
          onDiscard={handleDiscard}
          onOpenFile={handleOpenFile}
          fileRefs={fileRefs}
        />
      </PanelBody>
    </PanelRoot>
  );
});

function ChangesPanelContent({
  isLoading,
  allFiles,
  activeSessionId,
  reviewedFiles,
  staleFiles,
  autoMarkOnScroll,
  wordWrap,
  onToggleReviewed,
  onDiscard,
  onOpenFile,
  fileRefs,
}: {
  isLoading: boolean;
  allFiles: ReviewFile[];
  activeSessionId: string | null | undefined;
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  autoMarkOnScroll: boolean;
  wordWrap: boolean;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  onDiscard: (path: string) => Promise<void>;
  onOpenFile: (path: string) => void;
  fileRefs: Map<string, React.RefObject<HTMLDivElement | null>>;
}) {
  if (isLoading && allFiles.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        Loading changes...
      </div>
    );
  }
  if (allFiles.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        No changes
      </div>
    );
  }
  if (!activeSessionId) return null;
  return (
    <ReviewDiffList
      files={allFiles}
      reviewedFiles={reviewedFiles}
      staleFiles={staleFiles}
      sessionId={activeSessionId}
      autoMarkOnScroll={autoMarkOnScroll}
      wordWrap={wordWrap}
      onToggleReviewed={onToggleReviewed}
      onDiscard={onDiscard}
      onOpenFile={onOpenFile}
      fileRefs={fileRefs}
    />
  );
}

export { TaskChangesPanel };
