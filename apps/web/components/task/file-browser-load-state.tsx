"use client";

import { PanelLoadingState } from "@/components/panel-loading-state";
import type { FileTreeNode } from "@/lib/types/backend";
import type { WorkspaceRestorationAttempt } from "@/lib/state/slices/session-runtime/workspace-restoration";
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
  workspaceRestoration?: WorkspaceRestorationAttempt | null;
  onRestoreWorkspace?: () => void;
  restoreWorkspaceDisabled?: boolean;
};

export function renderSessionOrLoadState({
  isSessionFailed,
  sessionError,
  loadState,
  isLoadingTree,
  tree,
  loadError,
  onRetry,
  workspaceRestoration,
  onRestoreWorkspace,
  restoreWorkspaceDisabled,
}: RenderSessionOrLoadStateInput) {
  if (workspaceRestoration && workspaceRestoration.status !== "ready") {
    return (
      <WorkspaceUnavailable
        restoration={workspaceRestoration}
        onRetry={onRestoreWorkspace}
        retryDisabled={restoreWorkspaceDisabled}
      />
    );
  }
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
