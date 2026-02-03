'use client';

import { memo, useMemo, useState } from 'react';
import { IconArrowBackUp } from '@tabler/icons-react';
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
import { useToast } from '@/components/toast-provider';
import { useAppStore } from '@/components/state-provider';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useCumulativeDiff } from '@/hooks/domains/session/use-cumulative-diff';
import { useGitOperations } from '@/hooks/use-git-operations';
import { FileDiffViewer } from '@/components/diff';
import type { FileInfo } from '@/lib/state/slices';
import type { SelectedDiff } from './task-layout';
import { FileStatusIcon } from './file-status-icon';

type TaskChangesPanelProps = {
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  /** Callback to open file in editor */
  onOpenFile?: (filePath: string) => void;
};

/**
 * Normalize diff content by handling edge cases from the backend.
 *
 * Handles:
 * 1. Multiple diffs concatenated with "--- Staged changes ---" separator
 * 2. Diffs missing headers
 */
function normalizeDiffContent(diffContent: string): string {
  if (!diffContent || typeof diffContent !== 'string') return '';

  let trimmed = diffContent.trim();
  if (!trimmed) return '';

  // Handle concatenated diffs with "--- Staged changes ---" separator
  const stagedSeparator = '--- Staged changes ---';
  if (trimmed.includes(stagedSeparator)) {
    const parts = trimmed.split(stagedSeparator);
    trimmed = (parts[1] || parts[0]).trim();
  }

  return trimmed;
}

const TaskChangesPanel = memo(function TaskChangesPanel({
  selectedDiff,
  onClearSelected,
  onOpenFile,
}: TaskChangesPanelProps) {
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);

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
    // Create a Set of uncommitted file paths for efficient lookup
    const uncommittedPaths = new Set(uncommittedFiles.map(f => f.path));

    return Object.entries(cumulativeDiff.files)
      .filter(([path, file]) => file.diff && !uncommittedPaths.has(path)) // Exclude uncommitted files
      .map(([path, file]) => ({
        path,
        diff: file.diff!,
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
      }));
  }, [cumulativeDiff, uncommittedFiles]);

  const selectedFile = selectedDiffPath && gitStatus?.files ? gitStatus.files[selectedDiffPath] : null;
  // Prioritize current git status for uncommitted files, use provided content only for historical diffs
  // This ensures that when a file is modified by the agent, we show the latest diff
  const selectedDiffContent = selectedFile?.diff ?? selectedDiff?.content ?? '';
  const isSingleDiffSelected = Boolean(selectedDiffPath && (hasProvidedContent || selectedFile));
  // Discard only works for uncommitted changes (not committed/historical diffs)
  const canDiscard = Boolean(selectedDiffPath && selectedFile && !hasProvidedContent);

  const hasUncommitted = uncommittedFiles.length > 0;
  const hasCommitted = committedFiles.length > 0;
  const hasAnyChanges = hasUncommitted || hasCommitted;

  const handleDiscardClick = () => {
    setShowDiscardDialog(true);
  };

  const handleDiscardConfirm = async () => {
    if (!selectedDiffPath) {
      return;
    }

    try {
      const result = await discard([selectedDiffPath]);
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
      {/* Header */}
      <div className="flex items-center justify-between gap-3">
        {selectedDiffPath && (
          <>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs cursor-pointer ml-auto"
                  onClick={onClearSelected}
                >
                  Clear
                </Button>
              </TooltipTrigger>
              <TooltipContent>Show all changes</TooltipContent>
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
          </>
        )}
      </div>

      {/* Diff content */}
      <SessionPanelContent>
        {isSingleDiffSelected && selectedDiffContent ? (
          // Render single file diff (from commit, snapshot, or uncommitted)
          // Use timestamp from git status to ensure re-render when file changes
          <div key={`selected-${selectedDiffPath}-${gitStatus?.timestamp ?? ''}-${selectedDiffContent.length}`} className="space-y-4">
            <FileDiffViewer
              filePath={selectedDiffPath!}
              diff={normalizeDiffContent(selectedDiffContent)}
              status="M"
              enableComments={!!activeSessionId}
              sessionId={activeSessionId ?? undefined}
              onOpenFile={onOpenFile}
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
                  const normalizedDiff = normalizeDiffContent(file.diff);
                  const diffKey = `uncommitted-${file.path}-${normalizedDiff.length}`;
                  return (
                    <div key={diffKey} className="space-y-2">
                      <FileDiffViewer
                        filePath={file.path}
                        diff={normalizedDiff}
                        status={file.status}
                        enableComments={!!activeSessionId}
                        sessionId={activeSessionId ?? undefined}
                        onOpenFile={onOpenFile}
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
                  const normalizedDiff = normalizeDiffContent(file.diff);
                  const diffKey = `committed-${file.path}-${normalizedDiff.length}`;
                  return (
                    <div key={diffKey} className="space-y-2">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <FileStatusIcon status="modified" />
                          <Badge variant="secondary" className="rounded-full text-xs">
                            {file.path}
                          </Badge>
                        </div>
                        <span className="text-xs text-muted-foreground">
                          <span className="text-emerald-500">+{file.additions}</span>
                          {' / '}
                          <span className="text-rose-500">-{file.deletions}</span>
                        </span>
                      </div>
                      <FileDiffViewer
                        filePath={file.path}
                        diff={normalizedDiff}
                        status="M"
                        enableComments={!!activeSessionId}
                        sessionId={activeSessionId ?? undefined}
                        onOpenFile={onOpenFile}
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
