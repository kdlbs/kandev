'use client';

import { memo, useMemo, useState, useCallback, useEffect, useRef } from 'react';
import {
  IconArrowBackUp,
  IconExternalLink,
  IconPlus,
  IconMinus,
  IconCheck,
  IconChevronRight,
  IconGitCommit,
  IconLoader2,
  IconColumns,
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
import { useSessionFileReviews } from '@/hooks/use-session-file-reviews';
import { useCumulativeDiff } from '@/hooks/domains/session/use-cumulative-diff';
import { hashDiff, normalizeDiffContent } from '@/components/review/types';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { FileInfo } from '@/lib/state/store';
import { FileBrowser } from '@/components/task/file-browser';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';
import { useToast } from '@/components/toast-provider';
import { useFileOperations } from '@/hooks/use-file-operations';
import type { OpenFileTab } from '@/lib/types/backend';
import {
  getFilesPanelTab,
  setFilesPanelTab,
  hasUserSelectedFilesPanelTab,
  setUserSelectedFilesPanelTab,
} from '@/lib/local-storage';

import { usePanelActions } from '@/hooks/use-panel-actions';
import { FileStatusIcon } from './file-status-icon';

type TaskFilesPanelProps = {
  onSelectDiff: (path: string, content?: string) => void;
  onOpenFile: (file: OpenFileTab) => void;
  activeFilePath?: string | null;
};



const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf('/');
  if (lastSlash === -1) return { folder: '', file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

const TaskFilesPanel = memo(function TaskFilesPanel({ onSelectDiff, onOpenFile, activeFilePath }: TaskFilesPanelProps) {
  const [topTab, setTopTab] = useState<'diff' | 'files'>('diff');
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);
  const [fileToDiscard, setFileToDiscard] = useState<string | null>(null);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  const openEditor = useOpenSessionInEditor(activeSessionId ?? null);
  const gitOps = useGitOperations(activeSessionId ?? null);
  const { toast } = useToast();
  const { createFile: baseCreateFile, deleteFile: hookDeleteFile } = useFileOperations(activeSessionId ?? null);
  const { reviews } = useSessionFileReviews(activeSessionId);
  const { diff: cumulativeDiff } = useCumulativeDiff(activeSessionId);

  // Track if we've initialized the tab for this session
  const hasInitializedTabRef = useRef<string | null>(null);
  // Track whether user explicitly clicked the "files" tab (vs auto-selected)
  const userClickedFilesTabRef = useRef(false);
  // Track previous changed files count for auto-switch detection
  const prevChangedCountRef = useRef(0);

  // State for commit diffs
  const [expandedCommit, setExpandedCommit] = useState<string | null>(null);
  const [commitDiffs, setCommitDiffs] = useState<Record<string, Record<string, FileInfo>>>({});
  const [loadingCommitSha, setLoadingCommitSha] = useState<string | null>(null);
  const [pendingStageFiles, setPendingStageFiles] = useState<Set<string>>(new Set());

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

  // Handle create file (wraps shared hook to also open the new file)
  const handleCreateFile = useCallback(async (path: string): Promise<boolean> => {
    const ok = await baseCreateFile(path);
    if (ok) {
      const name = path.split('/').pop() || path;
      const { calculateHash } = await import('@/lib/utils/file-diff');
      const hash = await calculateHash('');
      onOpenFile({ path, name, content: '', originalContent: '', originalHash: hash, isDirty: false });
    }
    return ok;
  }, [baseCreateFile, onOpenFile]);

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

  // Compute review progress across all files (uncommitted + committed)
  const { reviewedCount, totalFileCount } = useMemo(() => {
    const paths = new Set<string>();

    // Uncommitted
    if (gitStatus?.files) {
      for (const [path, file] of Object.entries(gitStatus.files)) {
        const diff = file.diff ? normalizeDiffContent(file.diff) : '';
        if (diff) paths.add(path);
      }
    }

    // Committed (skip duplicates)
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

      // Check staleness: find the diff content for this path
      let diffContent = '';
      const uncommittedFile = gitStatus?.files?.[path];
      if (uncommittedFile?.diff) {
        diffContent = normalizeDiffContent(uncommittedFile.diff);
      } else if (cumulativeDiff?.files?.[path]?.diff) {
        diffContent = normalizeDiffContent(cumulativeDiff.files[path].diff!);
      }

      if (diffContent && state.diffHash && state.diffHash !== hashDiff(diffContent)) {
        continue; // stale — don't count
      }
      reviewed++;
    }

    return { reviewedCount: reviewed, totalFileCount: paths.size };
  }, [gitStatus, cumulativeDiff, reviews]);

  const reviewProgressPercent = totalFileCount > 0 ? (reviewedCount / totalFileCount) * 100 : 0;

  // Clear pending stage/unstage spinners when git status updates
  useEffect(() => {
    if (pendingStageFiles.size > 0) {
      setPendingStageFiles(new Set());
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [changedFiles]);

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

  // Open file in editor panel
  const { openFile: panelOpenFile } = usePanelActions();
  const handleOpenFileInDocumentPanel = useCallback((path: string) => {
    if (!activeSessionId) return;
    // On desktop, this will open a file editor tab via dockview
    panelOpenFile(path);
  }, [activeSessionId, panelOpenFile]);

  // Smart tab selection: restore user preference or auto-select based on changes
  useEffect(() => {
    if (!activeSessionId) return;

    // Only initialize once per session
    if (hasInitializedTabRef.current === activeSessionId) return;
    hasInitializedTabRef.current = activeSessionId;

    // If user has previously selected a tab for this session, restore it
    if (hasUserSelectedFilesPanelTab(activeSessionId)) {
      const savedTab = getFilesPanelTab(activeSessionId, 'diff');
      setTopTab(savedTab);
      userClickedFilesTabRef.current = savedTab === 'files';
      return;
    }

    // Auto-select based on whether there are changes
    userClickedFilesTabRef.current = false;
    const hasChanges = changedFiles.length > 0 || commits.length > 0;
    const defaultTab = hasChanges ? 'diff' : 'files';
    setTopTab(defaultTab);
    prevChangedCountRef.current = changedFiles.length;
  }, [activeSessionId, changedFiles.length, commits.length]);

  // Handle tab change - save preference and mark as user-selected
  const handleTabChange = useCallback((tab: 'diff' | 'files') => {
    setTopTab(tab);
    userClickedFilesTabRef.current = tab === 'files';
    if (activeSessionId) {
      setFilesPanelTab(activeSessionId, tab);
      setUserSelectedFilesPanelTab(activeSessionId);
    }
  }, [activeSessionId]);

  // Auto-switch to "Diff files" tab when new workspace changes appear
  useEffect(() => {
    const prev = prevChangedCountRef.current;
    prevChangedCountRef.current = changedFiles.length;

    // Switch when change count changes (new or removed changes detected)
    if (changedFiles.length !== prev) {
      // Don't switch if user explicitly chose the "All files" tab
      if (topTab === 'files' && userClickedFilesTabRef.current) return;
      setTopTab('diff');
    }
  }, [changedFiles.length, topTab]);

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
        onTabChange={(value) => handleTabChange(value as 'diff' | 'files')}
        className="flex-1 min-h-0"
      >
        <TabsContent value="diff" className="flex-1 min-h-0">
          <SessionPanelContent className="flex flex-col">
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
                            {pendingStageFiles.has(file.path) ? (
                              <div className="flex-shrink-0 flex items-center justify-center size-4">
                                <IconLoader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                              </div>
                            ) : file.staged ? (
                              <button
                                type="button"
                                title="Unstage file"
                                className="group/unstage flex-shrink-0 flex items-center justify-center size-4 rounded bg-emerald-500/20 text-emerald-600 hover:bg-rose-500/20 hover:text-rose-600 cursor-pointer"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  void handleUnstage(file.path);
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
                                onClick={(event) => {
                                  event.stopPropagation();
                                  void handleStage(file.path);
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
                                    className="text-muted-foreground hover:text-foreground cursor-pointer"
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      handleOpenFileInDocumentPanel(file.path);
                                    }}
                                  >
                                    <IconColumns className="h-3.5 w-3.5" />
                                  </button>
                                </TooltipTrigger>
                                <TooltipContent>Open side-by-side</TooltipContent>
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
                                <TooltipContent>Open in external editor</TooltipContent>
                              </Tooltip>
                            </div>
                            <LineStat added={file.plus} removed={file.minus} />
                            <FileStatusIcon status={file.status} />
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
                                    <FileStatusIcon status={file.status} />
                                    <span className="flex flex-1 min-w-0" title={path}>
                                      {folder && <span className="text-foreground/60 truncate shrink">{folder}/</span>}
                                      <span className="text-foreground whitespace-nowrap shrink-0">{fileName}</span>
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
                  Your changed files will appear here
                </div>
              )}
            </div>
            {/* Review progress — pushed to bottom of panel */}
            {totalFileCount > 0 && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <div
                    className="mt-auto flex items-center gap-2 pt-2 cursor-pointer transition-colors"
                    onClick={() => window.dispatchEvent(new CustomEvent('switch-to-changes-tab'))}
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
          </SessionPanelContent>
        </TabsContent>
        <TabsContent value="files" className="flex-1 min-h-0">
          <SessionPanelContent>
            {activeSessionId ? (
              <FileBrowser
                sessionId={activeSessionId}
                onOpenFile={onOpenFile}
                onCreateFile={handleCreateFile}
                onDeleteFile={hookDeleteFile}
                activeFilePath={activeFilePath}
              />
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
