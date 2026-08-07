"use client";

import { PanelLoadingState } from "@/components/panel-loading-state";
import type { FileTreeNode } from "@/lib/types/backend";
import { WorkspaceUnavailable } from "./workspace-unavailable";
import { t } from "@/lib/i18n";

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
    return <WorkspaceUnavailable error={sessionError} />;
  }
  if ((loadState === "loading" || isLoadingTree) && !tree) {
    return <PanelLoadingState label={t("task:loadingFiles")} />;
  }
  if (loadState === "waiting") {
    return <PanelLoadingState testId="file-tree-waiting" label={t("task:preparingWorkspace")} />;
  }
  if (loadState === "manual") {
    return (
      <div data-testid="file-tree-manual" className="p-4 text-sm text-muted-foreground space-y-2">
        <div>{loadError ?? t("task:workspaceIsStillStarting")}</div>
        <button
          type="button"
          className="text-xs text-foreground underline cursor-pointer"
          onClick={onRetry}
        >
          {t("task:retry")}
        </button>
      </div>
    );
  }
  return null;
}
