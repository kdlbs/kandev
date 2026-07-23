"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { TaskPR } from "@/lib/types/github";
import type { ReviewFile } from "./types";

type AutoCloseReviewDialogInput = {
  open: boolean;
  previousFileCount: number | null;
  fileCount: number;
  prDiffLoading: boolean;
};

export function shouldAutoCloseReviewDialog({
  open,
  previousFileCount,
  fileCount,
  prDiffLoading,
}: AutoCloseReviewDialogInput): boolean {
  return (
    open && !prDiffLoading && previousFileCount !== null && previousFileCount > 0 && fileCount === 0
  );
}

export function useReviewDialogAutoClose(opts: {
  open: boolean;
  fileCount: number;
  prDiffLoading: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const previousFileCountRef = useRef<number | null>(null);
  useEffect(() => {
    if (
      shouldAutoCloseReviewDialog({
        open: opts.open,
        previousFileCount: previousFileCountRef.current,
        fileCount: opts.fileCount,
        prDiffLoading: opts.prDiffLoading,
      })
    ) {
      opts.onOpenChange(false);
    }
    previousFileCountRef.current = opts.fileCount;
  }, [opts.open, opts.fileCount, opts.prDiffLoading, opts.onOpenChange]);
}

export type ReviewTransientState = {
  sourceKey: string;
  selectedFile: string | null;
  filter: string;
};

export function reviewDialogSourceKey(sessionId: string, selectedPRKey: string | null): string {
  return `${sessionId}\u0000${selectedPRKey ?? "review-without-pr"}`;
}

export function shouldBlockReviewForPR(files: ReviewFile[]): boolean {
  return !files.some((file) => file.source !== "pr");
}

export function resolveReviewTransientState(
  state: ReviewTransientState,
  sourceKey: string,
): ReviewTransientState {
  if (state.sourceKey === sourceKey) return state;
  return { sourceKey, selectedFile: null, filter: "" };
}

export function useReviewDialogTransientState(sourceKey: string) {
  const [state, setState] = useState<ReviewTransientState>(() => ({
    sourceKey,
    selectedFile: null,
    filter: "",
  }));
  const resolvedState = resolveReviewTransientState(state, sourceKey);
  useEffect(() => {
    setState((current) => resolveReviewTransientState(current, sourceKey));
  }, [sourceKey]);

  const setSelectedFile = useCallback(
    (value: string | null) =>
      setState((current) => ({
        ...resolveReviewTransientState(current, sourceKey),
        selectedFile: value,
      })),
    [sourceKey],
  );
  const setFilter = useCallback(
    (value: string) =>
      setState((current) => ({
        ...resolveReviewTransientState(current, sourceKey),
        filter: value,
      })),
    [sourceKey],
  );

  return {
    selectedFile: resolvedState.selectedFile,
    filter: resolvedState.filter,
    setSelectedFile,
    setFilter,
  };
}

export function usePRKeyedReviewFileSelection(
  selectFile: (path: string, setSelectedFile: (value: string | null) => void) => void,
  setSelectedFile: (value: string | null) => void,
) {
  return useCallback(
    (path: string) => selectFile(path, setSelectedFile),
    [selectFile, setSelectedFile],
  );
}

type ReviewPRDiffBoundaryProps = {
  selectedPR: TaskPR | null;
  loading: boolean;
  error: string | null;
  onRetry?: () => void;
  children: ReactNode;
};

export function ReviewPRDiffBoundary({
  selectedPR,
  loading,
  error,
  onRetry,
  children,
}: ReviewPRDiffBoundaryProps) {
  if (selectedPR && loading) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 px-6 text-center text-sm text-muted-foreground">
        <IconLoader2 className="h-5 w-5 animate-spin" />
        <span>
          Loading {selectedPR.repo} #{selectedPR.pr_number} changes…
        </span>
      </div>
    );
  }
  if (selectedPR && error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center text-sm text-muted-foreground">
        <span>{error}</span>
        {onRetry && (
          <Button className="min-h-11" variant="outline" size="sm" onClick={onRetry}>
            <IconRefresh className="h-4 w-4" />
            Retry
          </Button>
        )}
      </div>
    );
  }
  return children;
}
