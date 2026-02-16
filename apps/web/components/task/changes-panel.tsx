'use client';

import { memo, useMemo, useState, useCallback, useEffect } from 'react';
import { PanelRoot, PanelBody, PanelHeaderBarSplit } from './panel-primitives';
import {
  IconArrowBackUp,
  IconPlus,
  IconMinus,
  IconCheck,
  IconGitCommit,
  IconGitPullRequest,
  IconLoader2,
  IconPencil,
  IconCloudDownload,
  IconCloudUpload,
  IconEye,
  IconChevronDown,
  IconGitBranch,
  IconGitCherryPick,
  IconGitMerge,
  IconArrowRight,
} from '@tabler/icons-react';

import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@kandev/ui/hover-card';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from '@kandev/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@kandev/ui/alert-dialog';
import { Checkbox } from '@kandev/ui/checkbox';
import { Label } from '@kandev/ui/label';
import { Textarea } from '@kandev/ui/textarea';
import { LineStat } from '@/components/diff-stat';
import { cn } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';
import { useGitOperations } from '@/hooks/use-git-operations';
import { useSessionFileReviews } from '@/hooks/use-session-file-reviews';
import { useCumulativeDiff } from '@/hooks/domains/session/use-cumulative-diff';
import { hashDiff, normalizeDiffContent } from '@/components/review/types';
import type { FileInfo } from '@/lib/state/store';
import { useToast } from '@/components/toast-provider';
import { useIsTaskArchived, ArchivedPanelPlaceholder } from './task-archived-context';
import { FileStatusIcon } from './file-status-icon';

const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf('/');
  if (lastSlash === -1) return { folder: '', file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

type ChangedFile = {
  path: string;
  status: FileInfo['status'];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
};

type ChangesPanelProps = {
  onOpenFile: (path: string) => void;
  onOpenCommitDetail?: (sha: string) => void;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
};

// --- Timeline section dot colors ---
const DOT_COLORS = {
  unstaged: 'bg-yellow-500',
  staged: 'bg-emerald-500',
  commits: 'bg-blue-500',
  action: 'bg-muted-foreground/25',
} as const;

// --- Timeline visual components ---

function TimelineDot({ color }: { color: string }) {
  return (
    <div
      className={cn(
        'relative z-10 size-1.5 rounded-full shrink-0 mt-[5px]',
        color,
      )}
    />
  );
}

function TimelineSection({
  dotColor,
  label,
  count,
  action,
  isLast,
  children,
}: {
  dotColor: string;
  label?: string;
  count?: number;
  action?: React.ReactNode;
  isLast?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="relative flex gap-2.5">
      {/* Vertical line + dot */}
      <div className="flex flex-col items-center">
        <TimelineDot color={dotColor} />
        {!isLast && (
          <div className="w-px flex-1 bg-border/60" />
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0 pb-3">
        {/* Header */}
        {label && (
          <div className="flex items-center justify-between gap-2 -mt-0.5 mb-1">
            <span className="text-[11px] font-medium uppercase tracking-wider text-foreground/70">
              {label}
              {typeof count === 'number' && count > 0 && (
                <span className="ml-1 text-muted-foreground/50 font-normal">({count})</span>
              )}
            </span>
            {action}
          </div>
        )}

        {/* Children (file list, buttons, etc.) */}
        {children}
      </div>
    </div>
  );
}

// --- File row ---

function FileRow({
  file,
  isPending,
  onOpenFile,
  onStage,
  onUnstage,
  onDiscard,
  onEditFile,
}: {
  file: ChangedFile;
  isPending: boolean;
  onOpenFile: (path: string) => void;
  onStage: (path: string) => void;
  onUnstage: (path: string) => void;
  onDiscard: (path: string) => void;
  onEditFile: (path: string) => void;
}) {
  const { folder, file: name } = splitPath(file.path);

  return (
    <li
      className="group flex items-center justify-between gap-2 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
      onClick={() => onOpenFile(file.path)}
    >
      <div className="flex items-center gap-2 min-w-0">
        {isPending ? (
          <div className="flex-shrink-0 flex items-center justify-center size-4">
            <IconLoader2 className="h-3 w-3 animate-spin text-muted-foreground" />
          </div>
        ) : file.staged ? (
          <button
            type="button"
            title="Unstage file"
            className="group/unstage flex-shrink-0 flex items-center justify-center size-4 rounded bg-emerald-500/20 text-emerald-600 hover:bg-rose-500/20 hover:text-rose-600 cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onUnstage(file.path);
            }}
          >
            <IconCheck className="h-3 w-3 group-hover/unstage:hidden" />
            <IconMinus className="h-2.5 w-2.5 hidden group-hover/unstage:block" />
          </button>
        ) : (
          <button
            type="button"
            title="Stage file"
            className="flex-shrink-0 flex items-center justify-center size-4 rounded border border-dashed border-muted-foreground/50 text-muted-foreground hover:border-emerald-500 hover:text-emerald-500 hover:bg-emerald-500/10 cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onStage(file.path);
            }}
          >
            <IconPlus className="h-2.5 w-2.5" />
          </button>
        )}
        <button type="button" className="min-w-0 text-left cursor-pointer" title={file.path}>
          <p className="flex text-foreground text-xs min-w-0">
            {folder && <span className="text-foreground/60 truncate shrink">{folder}/</span>}
            <span className="font-medium text-foreground whitespace-nowrap shrink-0">{name}</span>
          </p>
        </button>
      </div>
      <div className="flex items-center gap-2">
        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground cursor-pointer"
                onClick={(e) => {
                  e.stopPropagation();
                  onDiscard(file.path);
                }}
              >
                <IconArrowBackUp className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent>Discard changes</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground cursor-pointer"
                onClick={(e) => {
                  e.stopPropagation();
                  onEditFile(file.path);
                }}
              >
                <IconPencil className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent>Edit</TooltipContent>
          </Tooltip>
        </div>
        <LineStat added={file.plus} removed={file.minus} />
        <FileStatusIcon status={file.status} />
      </div>
    </li>
  );
}

// --- Main component ---

const ChangesPanel = memo(function ChangesPanel({ onOpenFile, onOpenCommitDetail, onOpenDiffAll, onOpenReview }: ChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);
  const [fileToDiscard, setFileToDiscard] = useState<string | null>(null);
  const [pendingStageFiles, setPendingStageFiles] = useState<Set<string>>(new Set());

  // Commit dialog state
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [commitMessage, setCommitMessage] = useState('');

  // PR dialog state
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [prTitle, setPrTitle] = useState('');
  const [prBody, setPrBody] = useState('');
  const [prDraft, setPrDraft] = useState(true);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const taskTitle = useAppStore((state) => {
    if (!state.tasks.activeTaskId) return undefined;
    return state.kanban.tasks.find((t: { id: string }) => t.id === state.tasks.activeTaskId)?.title;
  });
  const baseBranch = useAppStore((state) => {
    if (!activeSessionId) return undefined;
    return state.taskSessions.items[activeSessionId]?.base_branch;
  });
  const displayBranch = useAppStore((state) => {
    if (!activeSessionId) return null;
    const gs = state.gitStatus.bySessionId[activeSessionId];
    return gs?.branch ?? null;
  });

  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  const gitOps = useGitOperations(activeSessionId ?? null);
  const { toast } = useToast();
  const { reviews } = useSessionFileReviews(activeSessionId);
  const { diff: cumulativeDiff } = useCumulativeDiff(activeSessionId);

  const aheadCount = gitStatus?.ahead ?? 0;
  const behindCount = gitStatus?.behind ?? 0;

  // Clean base branch name for display
  const baseBranchDisplay = useMemo(() => {
    if (!baseBranch) return 'main';
    return baseBranch.replace(/^origin\//, '');
  }, [baseBranch]);

  // Convert git status files to array
  const changedFiles = useMemo(() => {
    if (!gitStatus?.files || Object.keys(gitStatus.files).length === 0) {
      return [];
    }
    return (Object.values(gitStatus.files) as FileInfo[]).map((file: FileInfo) => ({
      path: file.path,
      status: file.status,
      staged: file.staged,
      plus: file.additions,
      minus: file.deletions,
      oldPath: file.old_path,
    }));
  }, [gitStatus]);

  // Split into unstaged and staged
  const unstagedFiles = useMemo(() => changedFiles.filter((f) => !f.staged), [changedFiles]);
  const stagedFiles = useMemo(() => changedFiles.filter((f) => f.staged), [changedFiles]);

  // Review progress
  const { reviewedCount, totalFileCount } = useMemo(() => {
    const paths = new Set<string>();

    if (gitStatus?.files) {
      for (const [path, file] of Object.entries(gitStatus.files)) {
        const diff = file.diff ? normalizeDiffContent(file.diff) : '';
        if (diff) paths.add(path);
      }
    }

    if (cumulativeDiff?.files) {
      for (const [path, file] of Object.entries(cumulativeDiff.files)) {
        if (paths.has(path)) continue;
        const diff = file.diff ? normalizeDiffContent(file.diff) : '';
        if (diff) paths.add(path);
      }
    }

    let reviewed = 0;
    for (const path of paths) {
      const state = reviews.get(path);
      if (!state?.reviewed) continue;

      let diffContent = '';
      const uncommittedFile = gitStatus?.files?.[path];
      if (uncommittedFile?.diff) {
        diffContent = normalizeDiffContent(uncommittedFile.diff);
      } else if (cumulativeDiff?.files?.[path]?.diff) {
        diffContent = normalizeDiffContent(cumulativeDiff.files[path].diff!);
      }

      if (diffContent && state.diffHash && state.diffHash !== hashDiff(diffContent)) {
        continue;
      }
      reviewed++;
    }

    return { reviewedCount: reviewed, totalFileCount: paths.size };
  }, [gitStatus, cumulativeDiff, reviews]);

  const reviewProgressPercent = totalFileCount > 0 ? (reviewedCount / totalFileCount) * 100 : 0;

  // Clear pending spinners when git status updates
  useEffect(() => {
    if (pendingStageFiles.size > 0) {
      setPendingStageFiles(new Set());
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [changedFiles]);

  // --- Handlers ---

  const handleGitOperation = useCallback(
    async (
      operation: () => Promise<{ success: boolean; output: string; error?: string }>,
      operationName: string,
    ) => {
      try {
        const result = await operation();
        if (result.success) {
          toast({
            title: `${operationName} successful`,
            description: result.output.slice(0, 200) || `${operationName} completed`,
            variant: 'success',
          });
        } else {
          toast({
            title: `${operationName} failed`,
            description: result.error || 'An error occurred',
            variant: 'error',
          });
        }
      } catch (e) {
        toast({
          title: `${operationName} failed`,
          description: e instanceof Error ? e.message : 'An unexpected error occurred',
          variant: 'error',
        });
      }
    },
    [toast],
  );

  const handlePull = useCallback(() => {
    handleGitOperation(() => gitOps.pull(), 'Pull');
  }, [handleGitOperation, gitOps]);

  const handleRebase = useCallback(() => {
    const targetBranch = baseBranch?.replace(/^origin\//, '') || 'main';
    handleGitOperation(() => gitOps.rebase(targetBranch), 'Rebase');
  }, [handleGitOperation, gitOps, baseBranch]);

  const handlePush = useCallback(() => {
    handleGitOperation(() => gitOps.push(), 'Push');
  }, [handleGitOperation, gitOps]);

  const handleStageAll = useCallback(async () => {
    try {
      await gitOps.stage();
    } catch {
      // Toast handled by gitOps
    }
  }, [gitOps]);

  const handleStage = useCallback(async (path: string) => {
    setPendingStageFiles((prev) => new Set(prev).add(path));
    try {
      await gitOps.stage([path]);
    } catch {
      setPendingStageFiles((prev) => {
        const next = new Set(prev);
        next.delete(path);
        return next;
      });
    }
  }, [gitOps]);

  const handleUnstage = useCallback(async (path: string) => {
    setPendingStageFiles((prev) => new Set(prev).add(path));
    try {
      await gitOps.unstage([path]);
    } catch {
      setPendingStageFiles((prev) => {
        const next = new Set(prev);
        next.delete(path);
        return next;
      });
    }
  }, [gitOps]);

  const handleDiscardClick = useCallback((filePath: string) => {
    setFileToDiscard(filePath);
    setShowDiscardDialog(true);
  }, []);

  const handleDiscardConfirm = useCallback(async () => {
    if (!fileToDiscard) return;
    try {
      const result = await gitOps.discard([fileToDiscard]);
      if (!result.success) {
        toast({
          title: 'Failed to discard changes',
          description: result.error || 'An unknown error occurred',
          variant: 'error',
        });
      }
    } catch (error) {
      toast({
        title: 'Failed to discard changes',
        description: error instanceof Error ? error.message : 'An unknown error occurred',
        variant: 'error',
      });
    } finally {
      setShowDiscardDialog(false);
      setFileToDiscard(null);
    }
  }, [fileToDiscard, gitOps, toast]);

  // Commit dialog
  const handleOpenCommitDialog = useCallback(() => {
    setCommitMessage('');
    setCommitDialogOpen(true);
  }, []);

  const handleCommit = useCallback(async () => {
    if (!commitMessage.trim()) return;
    setCommitDialogOpen(false);
    await handleGitOperation(
      () => gitOps.commit(commitMessage.trim(), false),
      'Commit',
    );
    setCommitMessage('');
  }, [commitMessage, handleGitOperation, gitOps]);

  // PR dialog
  const handleOpenPRDialog = useCallback(() => {
    setPrTitle(taskTitle || '');
    setPrBody('');
    setPrDialogOpen(true);
  }, [taskTitle]);

  const handleCreatePR = useCallback(async () => {
    if (!prTitle.trim()) return;
    setPrDialogOpen(false);
    try {
      const result = await gitOps.createPR(prTitle.trim(), prBody.trim(), baseBranch, prDraft);
      if (result.success) {
        toast({
          title: prDraft ? 'Draft PR created' : 'PR created',
          description: result.pr_url || 'PR created successfully',
          variant: 'success',
        });
        if (result.pr_url) window.open(result.pr_url, '_blank');
      } else {
        toast({
          title: 'Create PR failed',
          description: result.error || 'An error occurred',
          variant: 'error',
        });
      }
    } catch (e) {
      toast({
        title: 'Create PR failed',
        description: e instanceof Error ? e.message : 'An error occurred',
        variant: 'error',
      });
    }
    setPrTitle('');
    setPrBody('');
  }, [prTitle, prBody, baseBranch, prDraft, gitOps, toast]);

  // Determine which sections to show
  const hasUnstaged = unstagedFiles.length > 0;
  const hasStaged = stagedFiles.length > 0;
  const hasCommits = commits.length > 0;
  const hasChanges = changedFiles.length > 0;
  const hasAnything = hasUnstaged || hasStaged || hasCommits;

  // Compute stats for commit dialog
  const { stagedFileCount, stagedAdditions, stagedDeletions } = useMemo(() => {
    let additions = 0;
    let deletions = 0;
    const count = stagedFiles.length;
    if (gitStatus?.files && count > 0) {
      for (const file of Object.values(gitStatus.files) as FileInfo[]) {
        if (file.staged) {
          additions += file.additions || 0;
          deletions += file.deletions || 0;
        }
      }
    }
    return { stagedFileCount: count, stagedAdditions: additions, stagedDeletions: deletions };
  }, [gitStatus, stagedFiles]);

  // Determine how many timeline sections exist after commits for "isLast" logic
  const showActionButtons = hasCommits;
  const lastTimelineSection = hasCommits ? 'action' : hasStaged ? 'staged' : 'unstaged';

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      {/* Toolbar header */}
      <PanelHeaderBarSplit
        left={
          (hasChanges || hasCommits) ? (
            <>
              <Button
                size="sm"
                variant="ghost"
                className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
                onClick={onOpenDiffAll}
              >
                <IconGitMerge className="h-3 w-3" />
                Diff
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
                onClick={onOpenReview}
              >
                <IconEye className="h-3 w-3" />
                Review
              </Button>
            </>
          ) : undefined
        }
        right={
          <>
            {displayBranch && (
              <HoverCard openDelay={200} closeDelay={100}>
                <HoverCardTrigger asChild>
                  <button
                    type="button"
                    className="flex items-center justify-center size-5 rounded hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors cursor-default"
                  >
                    <IconGitBranch className="h-3.5 w-3.5" />
                  </button>
                </HoverCardTrigger>
                <HoverCardContent side="bottom" align="end" className="w-auto p-3">
                  <div className="flex flex-col gap-2.5 text-xs">
                    <div className="flex items-center justify-between gap-6">
                      <span className="text-muted-foreground/60">Your code lives in:</span>
                      <span className="text-muted-foreground/60">and will be merged into:</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="flex items-center gap-1.5 text-foreground font-medium">
                        <IconGitBranch className="h-3.5 w-3.5 text-muted-foreground" />
                        {displayBranch}
                      </span>
                      <div className="flex-1 border-t border-muted-foreground/20 min-w-8" />
                      <IconArrowRight className="h-3 w-3 text-muted-foreground/40" />
                      <span className="text-foreground font-medium">
                        {baseBranchDisplay}
                      </span>
                    </div>
                  </div>
                </HoverCardContent>
              </HoverCard>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
                  disabled={gitOps.isLoading}
                >
                  <IconCloudDownload className="h-3 w-3" />
                  Pull
                  {behindCount > 0 && (
                    <span className="text-yellow-500 text-[10px]">{behindCount}</span>
                  )}
                  <IconChevronDown className="h-2.5 w-2.5 text-muted-foreground" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-44">
                <DropdownMenuItem onClick={handlePull} className="cursor-pointer text-xs gap-2">
                  <IconCloudDownload className="h-3.5 w-3.5 text-muted-foreground" />
                  Pull
                  {behindCount > 0 && (
                    <span className="ml-auto text-muted-foreground text-[10px]">{behindCount} behind</span>
                  )}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={handleRebase} className="cursor-pointer text-xs gap-2">
                  <IconGitCherryPick className="h-3.5 w-3.5 text-muted-foreground" />
                  Rebase
                  {behindCount > 0 && (
                    <span className="ml-auto text-muted-foreground text-[10px]">{behindCount} behind</span>
                  )}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </>
        }
      />

      <PanelBody className="flex flex-col">
        {/* Scrollable content */}
        <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden">
          {/* Empty state */}
          {!hasAnything && (
            <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
              Your changed files will appear here
            </div>
          )}

          {/* Timeline */}
          {hasAnything && (
            <div className="flex flex-col">
              {/* UNSTAGED section */}
              {hasUnstaged && (
                <TimelineSection
                  dotColor={DOT_COLORS.unstaged}
                  label="Unstaged"
                  count={unstagedFiles.length}
                  isLast={lastTimelineSection === 'unstaged'}
                  action={
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-5 text-[10px] px-1.5 cursor-pointer"
                      onClick={handleStageAll}
                    >
                      Stage all
                    </Button>
                  }
                >
                  <ul className="space-y-0.5">
                    {unstagedFiles.map((file) => (
                      <FileRow
                        key={file.path}
                        file={file}
                        isPending={pendingStageFiles.has(file.path)}
                        onOpenFile={onOpenFile}
                        onStage={handleStage}
                        onUnstage={handleUnstage}
                        onDiscard={handleDiscardClick}
                        onEditFile={onOpenFile}
                      />
                    ))}
                  </ul>
                </TimelineSection>
              )}

              {/* STAGED section — only show when there are staged files */}
              {hasStaged && (
                <TimelineSection
                  dotColor={DOT_COLORS.staged}
                  label="Staged"
                  count={stagedFiles.length}
                  isLast={lastTimelineSection === 'staged'}
                  action={
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-5 text-[10px] px-1.5 cursor-pointer"
                      onClick={handleOpenCommitDialog}
                    >
                      Commit
                    </Button>
                  }
                >
                  <ul className="space-y-0.5">
                    {stagedFiles.map((file) => (
                      <FileRow
                        key={file.path}
                        file={file}
                        isPending={pendingStageFiles.has(file.path)}
                        onOpenFile={onOpenFile}
                        onStage={handleStage}
                        onUnstage={handleUnstage}
                        onDiscard={handleDiscardClick}
                        onEditFile={onOpenFile}
                      />
                    ))}
                  </ul>
                </TimelineSection>
              )}

              {/* COMMITS section */}
              {hasCommits && (
                <TimelineSection
                  dotColor={DOT_COLORS.commits}
                  label="Commits"
                  count={commits.length}
                  isLast={!showActionButtons}
                >
                  <ul className="space-y-0.5">
                    {commits.map((commit) => (
                      <li
                        key={commit.commit_sha}
                        className="flex items-center gap-2 text-xs rounded-md px-1 py-1 -mx-1 hover:bg-muted/60 cursor-pointer"
                        onClick={() => onOpenCommitDetail?.(commit.commit_sha)}
                      >
                        <IconGitCommit className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
                        <code className="font-mono text-muted-foreground text-[11px]">
                          {commit.commit_sha.slice(0, 7)}
                        </code>
                        <span className="flex-1 min-w-0 truncate text-foreground">
                          {commit.commit_message}
                        </span>
                        <span className="shrink-0 text-[11px]">
                          <span className="text-emerald-500">+{commit.insertions}</span>
                          {' '}
                          <span className="text-rose-500">-{commit.deletions}</span>
                        </span>
                      </li>
                    ))}
                  </ul>
                </TimelineSection>
              )}

              {/* Action buttons (Create PR / Push) — connected to timeline */}
              {showActionButtons && (
                <TimelineSection
                  dotColor={DOT_COLORS.action}
                  isLast
                >
                  <div className="flex items-center gap-2 -mt-0.5">
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
                      onClick={handleOpenPRDialog}
                    >
                      <IconGitPullRequest className="h-3 w-3" />
                      Create PR
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
                      onClick={handlePush}
                      disabled={gitOps.isLoading}
                    >
                      <IconCloudUpload className="h-3 w-3" />
                      Push
                      {aheadCount > 0 && (
                        <span className="text-muted-foreground">{aheadCount} ahead</span>
                      )}
                    </Button>
                  </div>
                </TimelineSection>
              )}
            </div>
          )}
        </div>

        {/* Review progress — pinned to bottom */}
        {totalFileCount > 0 && (
          <Tooltip>
            <TooltipTrigger asChild>
              <div
                className="shrink-0 flex items-center gap-2 pt-2 border-t border-border/40 cursor-pointer transition-colors"
                onClick={onOpenReview}
              >
                <div className="flex-1 h-0.5 rounded-full bg-muted-foreground/10 overflow-hidden">
                  <div
                    className="h-full bg-muted-foreground/25 rounded-full transition-all duration-300"
                    style={{ width: `${reviewProgressPercent}%` }}
                  />
                </div>
                <span className="text-[10px] text-muted-foreground/40 whitespace-nowrap">
                  {reviewedCount}/{totalFileCount} reviewed
                </span>
              </div>
            </TooltipTrigger>
            <TooltipContent>
              {reviewedCount} of {totalFileCount} files reviewed
            </TooltipContent>
          </Tooltip>
        )}
      </PanelBody>

      {/* Discard confirmation dialog */}
      <AlertDialog open={showDiscardDialog} onOpenChange={setShowDiscardDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard changes?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently discard all changes to{' '}
              <span className="font-semibold">{fileToDiscard}</span>. This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleDiscardConfirm} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              Discard
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Commit Dialog */}
      <Dialog open={commitDialogOpen} onOpenChange={setCommitDialogOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <IconGitCommit className="h-5 w-5" />
              Commit Changes
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="text-sm text-muted-foreground">
              {stagedFileCount > 0 ? (
                <span>
                  <span className="font-medium text-foreground">{stagedFileCount}</span>{' '}
                  staged file{stagedFileCount !== 1 ? 's' : ''}
                  {(stagedAdditions > 0 || stagedDeletions > 0) && (
                    <span className="ml-2">
                      (<span className="text-green-600">+{stagedAdditions}</span>
                      {' / '}
                      <span className="text-red-600">-{stagedDeletions}</span>)
                    </span>
                  )}
                </span>
              ) : (
                <span>No staged files to commit</span>
              )}
            </div>
            <Textarea
              placeholder="Enter commit message..."
              value={commitMessage}
              onChange={(e) => setCommitMessage(e.target.value)}
              rows={4}
              className="resize-none"
              autoFocus
            />
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                Cancel
              </Button>
            </DialogClose>
            <Button
              onClick={handleCommit}
              disabled={!commitMessage.trim() || gitOps.isLoading}
            >
              {gitOps.isLoading ? (
                <>
                  <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                  Committing...
                </>
              ) : (
                <>
                  <IconCheck className="h-4 w-4 mr-2" />
                  Commit
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create PR Dialog */}
      <Dialog open={prDialogOpen} onOpenChange={setPrDialogOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <IconGitPullRequest className="h-5 w-5" />
              Create Pull Request
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {displayBranch && (
              <div className="text-sm text-muted-foreground">
                {baseBranch ? (
                  <span>
                    Creating PR from{' '}
                    <span className="font-medium text-foreground">{displayBranch}</span>{' '}
                    &rarr;{' '}
                    <span className="font-medium text-foreground">{baseBranch}</span>
                  </span>
                ) : (
                  <span>
                    Creating PR from{' '}
                    <span className="font-medium text-foreground">{displayBranch}</span>
                  </span>
                )}
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="changes-pr-title" className="text-sm">
                Title
              </Label>
              <input
                id="changes-pr-title"
                type="text"
                placeholder="Pull request title..."
                value={prTitle}
                onChange={(e) => setPrTitle(e.target.value)}
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="changes-pr-body" className="text-sm">
                Description
              </Label>
              <Textarea
                id="changes-pr-body"
                placeholder="Describe your changes..."
                value={prBody}
                onChange={(e) => setPrBody(e.target.value)}
                rows={6}
                className="resize-none"
              />
            </div>
            <div className="flex items-center space-x-2">
              <Checkbox
                id="changes-pr-draft"
                checked={prDraft}
                onCheckedChange={(checked) => setPrDraft(checked === true)}
              />
              <Label htmlFor="changes-pr-draft" className="text-sm cursor-pointer">
                Create as draft
              </Label>
            </div>
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                Cancel
              </Button>
            </DialogClose>
            <Button
              onClick={handleCreatePR}
              disabled={!prTitle.trim() || gitOps.isLoading}
            >
              {gitOps.isLoading ? (
                <>
                  <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                  Creating...
                </>
              ) : (
                <>
                  <IconGitPullRequest className="h-4 w-4 mr-2" />
                  Create PR
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PanelRoot>
  );
});

export { ChangesPanel };
