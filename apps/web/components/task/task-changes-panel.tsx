'use client';

import { memo, useMemo, useState } from 'react';
import { DiffModeEnum, DiffView } from '@git-diff-view/react';
import {
  IconArrowBackUp,
  IconCopy,
  IconLayoutColumns,
  IconLayoutRows,
} from '@tabler/icons-react';
import { useTheme } from 'next-themes';
import { Badge } from '@kandev/ui/badge';
import { Button } from '@kandev/ui/button';
import { SessionPanelContent } from '@kandev/ui/pannel-session';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
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
import { cn } from '@/lib/utils';
import { useToast } from '@/components/toast-provider';
import { getLanguageFromPath } from '@/lib/languages';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { useAppStore } from '@/components/state-provider';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useCumulativeDiff } from '@/hooks/domains/session/use-cumulative-diff';
import { useGitOperations } from '@/hooks/use-git-operations';
import type { FileInfo } from '@/lib/state/slices';
import type { SelectedDiff } from './task-layout';

type TaskChangesPanelProps = {
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
};

const DEFAULT_DIFF_MODE: 'unified' | 'split' = 'unified';

/**
 * Normalize diff content for the @git-diff-view/react library.
 *
 * Handles two edge cases from the backend:
 * 1. Multiple diffs concatenated with "--- Staged changes ---" separator
 *    (we take the staged diff as it's what matters for the commit)
 * 2. Diffs missing the "diff --git" header line (for untracked files)
 *
 * The library expects each hunk to be a complete diff string with headers.
 */
function parseDiffIntoHunks(diffContent: string, filePath?: string): string[] {
  if (!diffContent || typeof diffContent !== 'string') return [];

  let trimmed = diffContent.trim();
  if (!trimmed) return [];

  // Handle concatenated diffs with "--- Staged changes ---" separator
  // Take the staged diff (after the separator) as it's the authoritative one
  const stagedSeparator = '--- Staged changes ---';
  if (trimmed.includes(stagedSeparator)) {
    const parts = trimmed.split(stagedSeparator);
    // Use the staged diff (second part) if available
    trimmed = (parts[1] || parts[0]).trim();
  }

  // If diff is missing the "diff --git" header, add it
  // This happens for untracked files where backend generates incomplete diff
  if (!trimmed.startsWith('diff --git') && trimmed.includes('@@')) {
    const path = filePath || 'file';
    const header = `diff --git a/${path} b/${path}\nnew file mode 100644\nindex 0000000..0000000\n`;
    trimmed = header + trimmed;
  }

  // Return as single hunk - the library handles parsing internally
  return [trimmed];
}

const TaskChangesPanel = memo(function TaskChangesPanel({
  selectedDiff,
  onClearSelected,
}: TaskChangesPanelProps) {
  const [diffViewMode, setDiffViewMode] = useState<'unified' | 'split'>(() =>
    getLocalStorage('task-diff-view-mode', DEFAULT_DIFF_MODE)
  );
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);

  const { resolvedTheme } = useTheme();
  const { toast } = useToast();
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { discard, isLoading: isDiscarding } = useGitOperations(activeSessionId);
  // Fetch cumulative diff (all commits combined) when no specific file is selected
  const { diff: cumulativeDiff, loading: cumulativeLoading } = useCumulativeDiff(
    selectedDiff ? null : activeSessionId
  );

  const selectedDiffPath = selectedDiff?.path ?? null;
  // Use provided diff content (from commit or snapshot) or look up from current git status
  const hasProvidedContent = Boolean(selectedDiff?.content);

  // Convert git status files to array for display (uncommitted changes)
  const uncommittedFiles = useMemo<FileInfo[]>(() => {
    if (!gitStatus?.files || Object.keys(gitStatus.files).length === 0) {
      return [];
    }
    return Object.values(gitStatus.files) as FileInfo[];
  }, [gitStatus]);

  // Convert cumulative diff files to array (committed changes)
  const committedFiles = useMemo<Array<{ path: string; diff: string; additions: number; deletions: number }>>(() => {
    if (!cumulativeDiff?.files || Object.keys(cumulativeDiff.files).length === 0) {
      return [];
    }
    return Object.entries(cumulativeDiff.files)
      .filter(([, file]) => file.diff) // Only include files with diffs
      .map(([path, file]) => ({
        path,
        diff: file.diff!,
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
      }));
  }, [cumulativeDiff]);

  const diffModeEnum = diffViewMode === 'split' ? DiffModeEnum.Split : DiffModeEnum.Unified;
  const diffTheme = resolvedTheme === 'dark' ? 'dark' : 'light';
  const selectedFile = selectedDiffPath && gitStatus?.files ? gitStatus.files[selectedDiffPath] : null;
  // Use provided content if available, otherwise fall back to git status
  const selectedDiffContent = selectedDiff?.content ?? selectedFile?.diff ?? '';
  const isSingleDiffSelected = Boolean(selectedDiffPath && (hasProvidedContent || selectedFile));
  // Discard only works for uncommitted changes (not committed/historical diffs)
  const canDiscard = Boolean(selectedDiffPath && selectedFile && !hasProvidedContent);

  const hasUncommitted = uncommittedFiles.length > 0;
  const hasCommitted = committedFiles.length > 0;
  const hasAnyChanges = hasUncommitted || hasCommitted;

  const handleDiscardClick = () => {
    console.log('Discard button clicked', { selectedDiffPath, canDiscard });
    setShowDiscardDialog(true);
  };

  const handleDiscardConfirm = async () => {
    if (!selectedDiffPath) {
      console.error('No selectedDiffPath');
      return;
    }

    console.log('Discarding changes for:', selectedDiffPath);
    try {
      const result = await discard([selectedDiffPath]);
      console.log('Discard result:', result);
      if (result.success) {
        toast({
          title: 'Changes discarded',
          description: `Successfully discarded changes to ${selectedDiffPath}`,
          variant: 'success',
        });
        onClearSelected();
      } else {
        toast({
          title: 'Failed to discard changes',
          description: result.error || 'An unknown error occurred',
          variant: 'error',
        });
      }
    } catch (error) {
      console.error('Discard error:', error);
      toast({
        title: 'Failed to discard changes',
        description: error instanceof Error ? error.message : 'An unknown error occurred',
        variant: 'error',
      });
    } finally {
      setShowDiscardDialog(false);
    }
  };

  return (
    <div className="flex flex-col gap-2 h-full">
      {/* Header toolbar */}
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Badge variant="secondary" className="rounded-full text-xs">
            {selectedDiffPath ?? 'All files'}
          </Badge>
        </div>
        <div className="flex items-center gap-1.5">
          {selectedDiffPath && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs cursor-pointer"
                  onClick={onClearSelected}
                >
                  Clear
                </Button>
              </TooltipTrigger>
              <TooltipContent>Show all uncommitted changes</TooltipContent>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs cursor-pointer"
                  disabled={!isSingleDiffSelected}
                  onClick={async () => {
                    if (!isSingleDiffSelected) return;
                    await navigator.clipboard.writeText(selectedDiffContent);
                  }}
                >
                  <IconCopy className="h-3.5 w-3.5" />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>Copy diff</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs cursor-pointer"
                  disabled={!canDiscard || isDiscarding}
                  onClick={handleDiscardClick}
                >
                  <IconArrowBackUp className="h-3.5 w-3.5" />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {canDiscard ? 'Discard changes' : hasProvidedContent ? 'Cannot discard committed changes' : 'Select an uncommitted file'}
            </TooltipContent>
          </Tooltip>
          <div className="inline-flex rounded-md border border-border overflow-hidden">
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className={cn(
                      'h-7 px-2 text-xs rounded-none cursor-pointer',
                      diffViewMode === 'unified' && 'bg-muted'
                    )}
                    onClick={() => {
                      setDiffViewMode('unified');
                      setLocalStorage('task-diff-view-mode', 'unified');
                    }}
                  >
                    <IconLayoutRows className="h-3.5 w-3.5" />
                  </Button>
                </span>
              </TooltipTrigger>
              <TooltipContent>Unified view</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className={cn(
                      'h-7 px-2 text-xs rounded-none cursor-pointer',
                      diffViewMode === 'split' && 'bg-muted'
                    )}
                    onClick={() => {
                      setDiffViewMode('split');
                      setLocalStorage('task-diff-view-mode', 'split');
                    }}
                  >
                    <IconLayoutColumns className="h-3.5 w-3.5" />
                  </Button>
                </span>
              </TooltipTrigger>
              <TooltipContent>Split view</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </div>

      {/* Diff content */}
      <SessionPanelContent>
        {isSingleDiffSelected && selectedDiffContent ? (
          // Render single file diff (from commit, snapshot, or uncommitted)
          // Key includes content length to force re-render when diff updates
          <div key={`selected-${selectedDiffPath}-${selectedDiffContent.length}`} className="space-y-4">
            <DiffView
              data={{
                hunks: parseDiffIntoHunks(selectedDiffContent, selectedDiffPath!),
                oldFile: { fileName: selectedDiffPath!, fileLang: getLanguageFromPath(selectedDiffPath!) },
                newFile: { fileName: selectedDiffPath!, fileLang: getLanguageFromPath(selectedDiffPath!) },
              }}
              diffViewMode={diffModeEnum}
              diffViewTheme={diffTheme}
            />
          </div>
        ) : cumulativeLoading ? (
          <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
            Loading changes...
          </div>
        ) : !hasAnyChanges ? (
          <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
            No changes
          </div>
        ) : (
          <div className="space-y-6">
            {/* Uncommitted changes section */}
            {hasUncommitted && (
              <div className="space-y-4">
                <div className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Uncommitted ({uncommittedFiles.length})
                </div>
                {uncommittedFiles.map((file) => {
                  if (!file.diff) return null;
                  const fileLang = getLanguageFromPath(file.path);
                  // Include diff length in key to force re-render when content changes
                  const diffKey = `uncommitted-${file.path}-${file.diff.length}`;
                  return (
                    <div key={diffKey} className="space-y-2">
                      <div className="flex items-center justify-between">
                        <Badge variant="secondary" className="rounded-full text-xs">
                          {file.path}
                        </Badge>
                        <span className="text-xs text-muted-foreground">
                          <span className="text-emerald-500">+{file.additions}</span>
                          {' / '}
                          <span className="text-rose-500">-{file.deletions}</span>
                        </span>
                      </div>
                      <DiffView
                        data={{
                          hunks: parseDiffIntoHunks(file.diff, file.path),
                          oldFile: { fileName: file.path, fileLang },
                          newFile: { fileName: file.path, fileLang },
                        }}
                        diffViewMode={diffModeEnum}
                        diffViewTheme={diffTheme}
                      />
                    </div>
                  );
                })}
              </div>
            )}

            {/* Committed changes section */}
            {hasCommitted && (
              <div className="space-y-4">
                <div className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Committed ({committedFiles.length})
                </div>
                {committedFiles.map((file) => {
                  if (!file.diff) return null;
                  const fileLang = getLanguageFromPath(file.path);
                  // Include diff length in key to force re-render when content changes
                  const diffKey = `committed-${file.path}-${file.diff.length}`;
                  return (
                    <div key={diffKey} className="space-y-2">
                      <div className="flex items-center justify-between">
                        <Badge variant="secondary" className="rounded-full text-xs">
                          {file.path}
                        </Badge>
                        <span className="text-xs text-muted-foreground">
                          <span className="text-emerald-500">+{file.additions}</span>
                          {' / '}
                          <span className="text-rose-500">-{file.deletions}</span>
                        </span>
                      </div>
                      <DiffView
                        data={{
                          hunks: parseDiffIntoHunks(file.diff, file.path),
                          oldFile: { fileName: file.path, fileLang },
                          newFile: { fileName: file.path, fileLang },
                        }}
                        diffViewMode={diffModeEnum}
                        diffViewTheme={diffTheme}
                      />
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </SessionPanelContent>

      {/* Discard confirmation dialog */}
      <AlertDialog open={showDiscardDialog} onOpenChange={setShowDiscardDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard changes?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently discard all changes to{' '}
              <span className="font-semibold">{selectedDiffPath}</span>. This action cannot be undone.
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
    </div>
  );
});

export { TaskChangesPanel };
