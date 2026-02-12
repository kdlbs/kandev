'use client';

import { memo, useMemo, useCallback, createRef, useState, useEffect, useRef } from 'react';
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
import { ReviewDialog } from '@/components/review/review-dialog';
import type { ReviewFile } from '@/components/review/types';
import { hashDiff, normalizeDiffContent } from '@/components/review/types';
import type { DiffComment } from '@/lib/diff/types';
import type { SelectedDiff } from './task-layout';

type TaskChangesPanelProps = {
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  /** Callback to open file in editor */
  onOpenFile?: (filePath: string) => void;
};

const TaskChangesPanel = memo(function TaskChangesPanel({
  selectedDiff,
  onClearSelected,
}: TaskChangesPanelProps) {
  const [splitView, setSplitView] = useState(() => {
    if (typeof window === 'undefined') return false;
    return localStorage.getItem('diff-view-mode') === 'split';
  });
  const [wordWrap, setWordWrap] = useState(false);
  const [reviewDialogOpen, setReviewDialogOpen] = useState(false);

  const { toast } = useToast();
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const autoMarkOnScroll = useAppStore((s) => s.userSettings.reviewAutoMarkOnScroll);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const userSettings = useAppStore((state) => state.userSettings);
  const taskTitle = useAppStore((state) => {
    if (!state.tasks.activeTaskId) return undefined;
    return state.kanban.tasks.find((t) => t.id === state.tasks.activeTaskId)?.title;
  });
  const baseBranch = useAppStore((state) => {
    if (!activeSessionId) return undefined;
    return state.taskSessions.items[activeSessionId]?.base_branch;
  });

  const gitStatus = useSessionGitStatus(activeSessionId);
  const { diff: cumulativeDiff, loading: cumulativeLoading } = useCumulativeDiff(activeSessionId);
  const { discard } = useGitOperations(activeSessionId);

  const { reviews, markReviewed, markUnreviewed } = useSessionFileReviews(activeSessionId);

  const bySession = useDiffCommentsStore((s) => s.bySession);
  const getPendingComments = useDiffCommentsStore((s) => s.getPendingComments);
  const markCommentsSent = useDiffCommentsStore((s) => s.markCommentsSent);

  // Merge uncommitted + committed files into a single sorted list
  const allFiles = useMemo<ReviewFile[]>(() => {
    const fileMap = new Map<string, ReviewFile>();

    // Uncommitted changes from git status
    if (gitStatus?.files) {
      for (const [path, file] of Object.entries(gitStatus.files)) {
        const diff = file.diff ? normalizeDiffContent(file.diff) : '';
        if (diff) {
          fileMap.set(path, {
            path,
            diff,
            status: file.status,
            additions: file.additions ?? 0,
            deletions: file.deletions ?? 0,
            staged: file.staged,
            source: 'uncommitted',
          });
        }
      }
    }

    // Committed changes from cumulative diff
    if (cumulativeDiff?.files) {
      for (const [path, file] of Object.entries(cumulativeDiff.files)) {
        if (fileMap.has(path)) continue; // Uncommitted takes priority
        const diff = file.diff ? normalizeDiffContent(file.diff) : '';
        if (diff) {
          fileMap.set(path, {
            path,
            diff,
            status: file.status || 'modified',
            additions: file.additions ?? 0,
            deletions: file.deletions ?? 0,
            staged: false,
            source: 'committed',
          });
        }
      }
    }

    return Array.from(fileMap.values()).sort((a, b) => a.path.localeCompare(b.path));
  }, [gitStatus, cumulativeDiff]);

  // Reviewed / stale sets
  const { reviewedFiles, staleFiles } = useMemo(() => {
    const reviewed = new Set<string>();
    const stale = new Set<string>();

    for (const file of allFiles) {
      const reviewState = reviews.get(file.path);
      if (!reviewState?.reviewed) continue;

      const currentHash = hashDiff(file.diff);
      if (reviewState.diffHash && reviewState.diffHash !== currentHash) {
        stale.add(file.path);
      } else {
        reviewed.add(file.path);
      }
    }

    return { reviewedFiles: reviewed, staleFiles: stale };
  }, [allFiles, reviews]);

  // Comment counts
  const commentCountByFile = useMemo(() => {
    const counts: Record<string, number> = {};
    if (!activeSessionId) return counts;
    const sessionComments = bySession[activeSessionId];
    if (!sessionComments) return counts;

    for (const [filePath, comments] of Object.entries(sessionComments)) {
      if (comments.length > 0) {
        counts[filePath] = comments.length;
      }
    }
    return counts;
  }, [bySession, activeSessionId]);

  const totalCommentCount = useMemo(() => {
    return Object.values(commentCountByFile).reduce((sum, c) => sum + c, 0);
  }, [commentCountByFile]);

  // File refs for scrollIntoView
  const fileRefs = useMemo(() => {
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const file of allFiles) {
      refs.set(file.path, createRef<HTMLDivElement>());
    }
    return refs;
  }, [allFiles]);

  // Scroll to selected file when selectedDiff changes
  const scrolledRef = useRef<string | null>(null);
  useEffect(() => {
    if (!selectedDiff?.path || scrolledRef.current === selectedDiff.path) return;
    scrolledRef.current = selectedDiff.path;

    const ref = fileRefs.get(selectedDiff.path);
    if (ref?.current) {
      // Small delay to let lazy-rendered content mount
      requestAnimationFrame(() => {
        ref.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      });
    }
    onClearSelected();
  }, [selectedDiff, fileRefs, onClearSelected]);

  // Reset scroll tracking when selectedDiff is cleared
  useEffect(() => {
    if (!selectedDiff) {
      scrolledRef.current = null;
    }
  }, [selectedDiff]);

  const handleToggleSplitView = useCallback((split: boolean) => {
    setSplitView(split);
    const mode = split ? 'split' : 'unified';
    localStorage.setItem('diff-view-mode', mode);
    window.dispatchEvent(new CustomEvent('diff-view-mode-change', { detail: mode }));
  }, []);

  const handleToggleReviewed = useCallback(
    (path: string, reviewed: boolean) => {
      if (reviewed) {
        const file = allFiles.find((f) => f.path === path);
        const diffHash = file ? hashDiff(file.diff) : '';
        markReviewed(path, diffHash);
      } else {
        markUnreviewed(path);
      }
    },
    [allFiles, markReviewed, markUnreviewed]
  );

  const handleDiscard = useCallback(
    async (path: string) => {
      try {
        const result = await discard([path]);
        if (result.success) {
          toast({ title: 'Changes discarded', description: path, variant: 'success' });
        } else {
          toast({ title: 'Discard failed', description: result.error || 'An error occurred', variant: 'error' });
        }
      } catch (e) {
        toast({ title: 'Discard failed', description: e instanceof Error ? e.message : 'An error occurred', variant: 'error' });
      }
    },
    [discard, toast]
  );

  const handleToggleAutoMark = useCallback(
    (checked: boolean) => {
      const next = { ...userSettings, reviewAutoMarkOnScroll: checked };
      setUserSettings(next);

      const client = getWebSocketClient();
      const payload = { review_auto_mark_on_scroll: checked };
      if (client) {
        client.request('user.settings.update', payload).catch(() => {
          updateUserSettings(payload, { cache: 'no-store' }).catch(() => { });
        });
      } else {
        updateUserSettings(payload, { cache: 'no-store' }).catch(() => { });
      }
    },
    [userSettings, setUserSettings]
  );

  const handleFixComments = useCallback(() => {
    if (!activeSessionId || !activeTaskId) return;
    const comments = getPendingComments();
    if (comments.length === 0) return;

    const markdown = formatReviewCommentsAsMarkdown(comments);
    if (!markdown) return;

    const client = getWebSocketClient();
    if (client) {
      client.request('message.add', {
        task_id: activeTaskId,
        session_id: activeSessionId,
        content: markdown,
      }).catch(() => {
        toast({ title: 'Failed to send comments', variant: 'error' });
      });
    }

    markCommentsSent(comments.map((c) => c.id));
  }, [activeSessionId, activeTaskId, getPendingComments, markCommentsSent, toast]);

  const handleDialogSendComments = useCallback(
    (comments: DiffComment[]) => {
      if (!activeTaskId || !activeSessionId || comments.length === 0) return;
      const client = getWebSocketClient();
      if (!client) return;
      const markdown = formatReviewCommentsAsMarkdown(comments);
      client.request('message.add', {
        task_id: activeTaskId,
        session_id: activeSessionId,
        content: markdown,
      }, 10000).catch(() => {
        toast({ title: 'Failed to send comments', variant: 'error' });
      });
      setReviewDialogOpen(false);
    },
    [activeTaskId, activeSessionId, toast]
  );

  const reviewedCount = reviewedFiles.size;
  const totalCount = allFiles.length;
  const progressPercent = totalCount > 0 ? (reviewedCount / totalCount) * 100 : 0;

  return (
    <div className="flex flex-col h-full">
      {/* Top bar */}
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border bg-card/50 min-h-[36px]">
        {/* Settings cog */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm" variant="ghost" className="px-1.5 h-7 cursor-pointer">
              <IconSettings className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-64">
            <DropdownMenuItem
              className="cursor-pointer gap-2"
              onSelect={(e) => {
                e.preventDefault();
                handleToggleAutoMark(!autoMarkOnScroll);
              }}
            >
              <Checkbox
                checked={autoMarkOnScroll}
                className="pointer-events-none"
              />
              <span className="text-sm flex-1">Auto-mark reviewed on scroll</span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {/* Progress bar */}
        {totalCount > 0 && (
          <div className="flex items-center gap-2 min-w-0">
            <div className="w-20 h-1 rounded-full bg-muted overflow-hidden">
              <div
                className="h-full bg-indigo-500 rounded-full transition-all duration-300"
                style={{ width: `${progressPercent}%` }}
              />
            </div>
            <span className="text-[11px] text-muted-foreground whitespace-nowrap">
              {reviewedCount}/{totalCount} Reviewed
            </span>
          </div>
        )}

        <div className="flex-1" />

        {/* Expand to review dialog */}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              className="px-1.5 h-7 cursor-pointer"
              onClick={() => setReviewDialogOpen(true)}
            >
              <IconArrowsMaximize className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Expand review</TooltipContent>
        </Tooltip>

        {/* Word wrap */}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              className={`px-1.5 h-7 cursor-pointer ${wordWrap ? 'bg-muted' : ''}`}
              onClick={() => setWordWrap(!wordWrap)}
            >
              <IconTextWrap className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Toggle word wrap</TooltipContent>
        </Tooltip>

        {/* Split/Unified */}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              className="px-1.5 h-7 cursor-pointer"
              onClick={() => handleToggleSplitView(!splitView)}
            >
              {splitView ? (
                <IconLayoutRows className="h-3.5 w-3.5" />
              ) : (
                <IconLayoutColumns className="h-3.5 w-3.5" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{splitView ? 'Unified view' : 'Split view'}</TooltipContent>
        </Tooltip>

        {/* Fix Comments */}
        {totalCommentCount > 0 && (
          <Button size="sm" variant="outline" className="h-7 text-xs cursor-pointer" onClick={handleFixComments}>
            <IconMessageForward className="h-3.5 w-3.5" />
            Fix
            <span className="ml-0.5 rounded-full bg-blue-500/30 px-1 py-0 text-[10px] font-medium text-blue-600 dark:text-blue-400">
              {totalCommentCount}
            </span>
          </Button>
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-h-0 overflow-hidden">
        {cumulativeLoading && allFiles.length === 0 ? (
          <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
            Loading changes...
          </div>
        ) : allFiles.length === 0 ? (
          <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
            No changes
          </div>
        ) : activeSessionId ? (
          <ReviewDiffList
            files={allFiles}
            reviewedFiles={reviewedFiles}
            staleFiles={staleFiles}
            sessionId={activeSessionId}
            autoMarkOnScroll={autoMarkOnScroll}
            wordWrap={wordWrap}
            onToggleReviewed={handleToggleReviewed}
            onDiscard={handleDiscard}
            fileRefs={fileRefs}
          />
        ) : null}
      </div>

      {/* Review Dialog */}
      {activeSessionId && (
        <ReviewDialog
          open={reviewDialogOpen}
          onOpenChange={setReviewDialogOpen}
          sessionId={activeSessionId}
          baseBranch={baseBranch}
          taskTitle={taskTitle}
          onSendComments={handleDialogSendComments}
          gitStatusFiles={gitStatus?.files ?? null}
          cumulativeDiff={cumulativeDiff}
        />
      )}
    </div>
  );
});

export { TaskChangesPanel };
