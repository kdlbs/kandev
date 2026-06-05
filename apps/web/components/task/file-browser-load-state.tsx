"use client";

import { PanelLoadingState } from "@/components/panel-loading-state";
import type { FileTreeNode } from "@/lib/types/backend";

type RenderSessionOrLoadStateInput = {
  isSessionFailed: boolean;
  sessionError: string | null | undefined;
  loadState: string;
  isLoadingTree: boolean;
  tree: FileTreeNode | null;
  loadError: string | null;
  onRetry: () => void;
};

export function renderSessionOrLoadState({
  isSessionFailed,
  sessionError,
  loadState,
  isLoadingTree,
  tree,
  loadError,
  onRetry,
}: RenderSessionOrLoadStateInput) {
  if (isSessionFailed) {
    return (
      <div className="p-4 text-sm text-destructive/80 space-y-2">
        <div>Session failed</div>
        {sessionError && <div className="text-xs text-muted-foreground">{sessionError}</div>}
      </div>
    );
  }
  if ((loadState === "loading" || isLoadingTree) && !tree) {
    return <PanelLoadingState label="Loading files..." />;
  }
  if (loadState === "waiting") {
    return <PanelLoadingState testId="file-tree-waiting" label="Preparing workspace..." />;
  }
  if (loadState === "manual") {
    return (
      <div data-testid="file-tree-manual" className="p-4 text-sm text-muted-foreground space-y-2">
        <div>{loadError ?? "Workspace is still starting."}</div>
        <button
          type="button"
          className="text-xs text-foreground underline cursor-pointer"
          onClick={onRetry}
        >
          Retry
        </button>
      </div>
    );
  }
  return null;
}
