"use client";

import { IconLoader2 } from "@tabler/icons-react";
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
    return <div className="p-4 text-sm text-muted-foreground">Loading files...</div>;
  }
  if (loadState === "waiting") {
    return (
      <div className="p-4 text-sm text-muted-foreground flex items-center gap-2">
        <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
        Preparing workspace...
      </div>
    );
  }
  if (loadState === "manual") {
    return (
      <div className="p-4 text-sm text-muted-foreground space-y-2">
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
