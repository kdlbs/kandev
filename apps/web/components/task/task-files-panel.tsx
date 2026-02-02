'use client';

import { memo, useMemo, useState, useCallback } from 'react';
import {
  IconArrowBackUp,
  IconExternalLink,
  IconPlus,
  IconCircleFilled,
  IconMinus,
  IconCheck,
  IconChevronRight,
  IconGitCommit,
  IconLoader2,
} from '@tabler/icons-react';

import { TabsContent } from '@kandev/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { SessionPanel, SessionPanelContent } from '@kandev/ui/pannel-session';
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
import { LineStat } from '@/components/diff-stat';
import { cn } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import { useOpenSessionInEditor } from '@/hooks/use-open-session-in-editor';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';
import { useGitOperations } from '@/hooks/use-git-operations';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { FileInfo } from '@/lib/state/store';
import { FileBrowser } from '@/components/task/file-browser';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';
import { useToast } from '@/components/toast-provider';
import type { OpenFileTab } from '@/lib/types/backend';

type TaskFilesPanelProps = {
  onSelectDiff: (path: string, content?: string) => void;
  onOpenFile: (file: OpenFileTab) => void;
};



const StatusIcon = ({ status }: { status: FileInfo['status'] }) => {
  switch (status) {
    case 'added':
    case 'untracked':
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-emerald-600">
          <IconPlus className="h-2 w-2 text-emerald-600" />
        </div>
      );
    case 'modified':
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-yellow-600">
          <IconCircleFilled className="h-1 w-1 text-yellow-600" />
        </div>
      );
    case 'deleted':
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-rose-600">
          <IconMinus className="h-2 w-2 text-rose-600" />
        </div>
      );
    case 'renamed':
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-purple-600">
          <IconCircleFilled className="h-1 w-1 text-purple-600" />
        </div>
      );
    default:
      return (
        <div className="flex items-center justify-center h-3 w-3 rounded border border-muted-foreground">
          <IconCircleFilled className="h-1 w-1 text-muted-foreground" />
        </div>
      );
  }
};

const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf('/');
  if (lastSlash === -1) return { folder: '', file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

const TaskFilesPanel = memo(function TaskFilesPanel({ onSelectDiff, onOpenFile }: TaskFilesPanelProps) {
  const [topTab, setTopTab] = useState<'diff' | 'files'>('diff');
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);
  const [fileToDiscard, setFileToDiscard] = useState<string | null>(null);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  const openEditor = useOpenSessionInEditor(activeSessionId ?? null);
  const gitOps = useGitOperations(activeSessionId ?? null);
  const { toast } = useToast();

  // State for commit diffs
  const [expandedCommit, setExpandedCommit] = useState<string | null>(null);
  const [commitDiffs, setCommitDiffs] = useState<Record<string, Record<string, FileInfo>>>({});
  const [loadingCommitSha, setLoadingCommitSha] = useState<string | null>(null);

  // Fetch commit diff
  const fetchCommitDiff = useCallback(async (commitSha: string) => {
    if (!activeSessionId || commitDiffs[commitSha]) return;

    setLoadingCommitSha(commitSha);
    try {
      const ws = getWebSocketClient();
      if (!ws) return;
      const response = await ws.request<{ success: boolean; files: Record<string, FileInfo> }>(
        'session.commit_diff',
        { session_id: activeSessionId, commit_sha: commitSha },
        10000
      );
      if (response?.success && response.files) {
        setCommitDiffs((prev) => ({ ...prev, [commitSha]: response.files }));
      }
    } catch (err) {
      toast({
        title: 'Failed to load commit diff',
        description: err instanceof Error ? err.message : 'An unexpected error occurred',
        variant: 'error',
      });
    } finally {
      setLoadingCommitSha(null);
    }
  }, [activeSessionId, commitDiffs, toast]);

  // Toggle commit expansion
  const toggleCommit = useCallback((commitSha: string) => {
    if (expandedCommit === commitSha) {
      setExpandedCommit(null);
    } else {
      setExpandedCommit(commitSha);
      void fetchCommitDiff(commitSha);
    }
  }, [expandedCommit, fetchCommitDiff]);

  // Handle discard changes
  const handleDiscardClick = useCallback((filePath: string) => {
    setFileToDiscard(filePath);
    setShowDiscardDialog(true);
  }, []);

  const handleDiscardConfirm = useCallback(async () => {
    if (!fileToDiscard) return;

    try {
      const result = await gitOps.discard([fileToDiscard]);
      if (result.success) {
        toast({
          title: 'Changes discarded',
          description: `Successfully discarded changes to ${fileToDiscard}`,
          variant: 'success',
        });
      } else {
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

  // Convert git status files to array for display
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

  const tabs: SessionTab[] = [
    {
      id: 'diff',
      label: `Diff files${changedFiles.length > 0 ? ` (${changedFiles.length})` : ''}`,
    },
    {
      id: 'files',
      label: 'All files',
    },
  ];

  return (
    <SessionPanel borderSide="left">
      <SessionTabs
        tabs={tabs}
        activeTab={topTab}
        onTabChange={(value) => setTopTab(value as 'diff' | 'files')}
        className="flex-1 min-h-0"
      >
        <TabsContent value="diff" className="flex-1 min-h-0">
          <SessionPanelContent>
            <div className="space-y-4">
              {/* Uncommitted changes section */}
              {changedFiles.length > 0 && (
                <div>
                  <div className="text-xs font-medium text-muted-foreground mb-2">
                    Uncommitted ({changedFiles.length})
                  </div>
                  <ul className="space-y-1">
                    {changedFiles.map((file) => {
                      const { folder, file: name } = splitPath(file.path);
                      return (
                        <li
                          key={file.path}
                          className="group flex items-center justify-between gap-2 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
                          onClick={() => onSelectDiff(file.path)}
                        >
                          <div className="flex items-center gap-2 min-w-0">
                            {file.staged ? (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <button
                                    type="button"
                                    className="group/unstage flex items-center justify-center h-4 w-4 rounded bg-emerald-500/20 text-emerald-600 hover:bg-rose-500/20 hover:text-rose-600"
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      void gitOps.unstage([file.path]);
                                    }}
                                  >
                                    <IconCheck className="h-3 w-3 group-hover/unstage:hidden" />
                                    <IconMinus className="h-2.5 w-2.5 hidden group-hover/unstage:block" />
                                  </button>
                                </TooltipTrigger>
                                <TooltipContent>Click to unstage</TooltipContent>
                              </Tooltip>
                            ) : (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <button
                                    type="button"
                                    className="flex items-center justify-center h-4 w-4 rounded border border-dashed border-muted-foreground/50 text-muted-foreground hover:border-emerald-500 hover:text-emerald-500 hover:bg-emerald-500/10"
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      void gitOps.stage([file.path]);
                                    }}
                                  >
                                    <IconPlus className="h-2.5 w-2.5" />
                                  </button>
                                </TooltipTrigger>
                                <TooltipContent>Click to stage</TooltipContent>
                              </Tooltip>
                            )}
                            <button type="button" className="min-w-0 text-left cursor-pointer">
                              <p className="truncate text-foreground text-xs">
                                <span className="text-foreground/60">{folder}/</span>
                                <span className="font-medium text-foreground">{name}</span>
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
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      handleDiscardClick(file.path);
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
                                    className="text-muted-foreground hover:text-foreground"
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      if (!activeSessionId) return;
                                      void openEditor.open({ filePath: file.path });
                                    }}
                                  >
                                    <IconExternalLink className="h-3.5 w-3.5" />
                                  </button>
                                </TooltipTrigger>
                                <TooltipContent>Open in editor</TooltipContent>
                              </Tooltip>
                            </div>
                            <LineStat added={file.plus} removed={file.minus} />
                            <StatusIcon status={file.status} />
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              )}

              {/* Commits section */}
              {commits.length > 0 && (
                <div>
                  <div className="text-xs font-medium text-muted-foreground mb-2">
                    Commits ({commits.length})
                  </div>
                  <ul className="space-y-1">
                    {commits.map((commit) => {
                      const isExpanded = expandedCommit === commit.commit_sha;
                      const files = commitDiffs[commit.commit_sha];
                      const isLoading = loadingCommitSha === commit.commit_sha;
                      return (
                        <li key={commit.commit_sha}>
                          <button
                            type="button"
                            className="w-full flex items-center gap-2 text-left rounded-md px-1 py-1 -mx-1 hover:bg-muted/60 cursor-pointer"
                            onClick={() => toggleCommit(commit.commit_sha)}
                          >
                            <IconChevronRight
                              className={cn(
                                'h-3 w-3 text-muted-foreground transition-transform shrink-0',
                                isExpanded && 'rotate-90'
                              )}
                            />
                            <IconGitCommit className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
                            <div className="flex-1 min-w-0">
                              <p className="text-xs truncate">
                                <code className="font-mono text-muted-foreground">{commit.commit_sha.slice(0, 7)}</code>
                                {' '}
                                <span className="text-foreground">{commit.commit_message}</span>
                              </p>
                            </div>
                            <span className="text-xs shrink-0">
                              <span className="text-emerald-500">+{commit.insertions}</span>
                              {'/'}
                              <span className="text-rose-500">-{commit.deletions}</span>
                            </span>
                            {isLoading && (
                              <IconLoader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                            )}
                          </button>
                          {isExpanded && files && (
                            <ul className="ml-6 mt-1 space-y-0.5">
                              {Object.entries(files).map(([path, file]) => {
                                const { folder, file: fileName } = splitPath(path);
                                return (
                                  <li
                                    key={path}
                                    className="flex items-center gap-2 text-xs rounded px-1 py-0.5 hover:bg-muted/50 cursor-pointer"
                                    onClick={() => onSelectDiff(path, file.diff)}
                                  >
                                    <StatusIcon status={file.status} />
                                    <span className="truncate flex-1">
                                      <span className="text-foreground/60">{folder}/</span>
                                      <span className="text-foreground">{fileName}</span>
                                    </span>
                                    <span className="shrink-0 text-xs">
                                      <span className="text-emerald-500">+{file.additions ?? 0}</span>
                                      {'/'}
                                      <span className="text-rose-500">-{file.deletions ?? 0}</span>
                                    </span>
                                  </li>
                                );
                              })}
                            </ul>
                          )}
                        </li>
                      );
                    })}
                  </ul>
                </div>
              )}

              {/* Empty state */}
              {changedFiles.length === 0 && commits.length === 0 && (
                <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
                  No changes detected
                </div>
              )}
            </div>
          </SessionPanelContent>
        </TabsContent>
        <TabsContent value="files" className="mt-3 flex-1 min-h-0">
          <SessionPanelContent>
            {activeSessionId ? (
              <FileBrowser sessionId={activeSessionId} onOpenFile={onOpenFile} />
            ) : (
              <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
                No task selected
              </div>
            )}
          </SessionPanelContent>
        </TabsContent>
      </SessionTabs>

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
    </SessionPanel>
  );
});

export { TaskFilesPanel };
