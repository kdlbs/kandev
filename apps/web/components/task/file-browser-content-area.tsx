"use client";

import { useTranslation } from "react-i18next";
import { renderSessionOrLoadState } from "./file-browser-load-state";
import {
  FileTreeView,
  SearchResultsList,
  type FileBrowserContentAreaProps,
} from "./file-browser-parts";
import { WorkspaceUnavailable } from "./workspace-unavailable";

export function FileBrowserContentArea(props: FileBrowserContentAreaProps) {
  const { t } = useTranslation();
  const workspaceBlocked =
    props.workspaceRestoration && props.workspaceRestoration.status !== "ready";
  if (workspaceBlocked && props.tree) {
    const notice = (
      <WorkspaceUnavailable
        restoration={props.workspaceRestoration}
        onRetry={props.onRestoreWorkspace}
        retryDisabled={props.restoreWorkspaceDisabled}
        compact
      />
    );
    if (props.isSearchActive && props.searchResults !== null) {
      return (
        <div className="flex min-h-0 flex-col">
          {notice}
          <SearchResultsList
            searchResults={props.searchResults}
            fileStatuses={props.fileStatuses}
            onOpenFile={props.onOpenFile}
            showTouchActions={props.showTouchActions}
            onAddToChatContext={props.onAddToChatContext}
          />
        </div>
      );
    }
    return (
      <div className="flex min-h-0 flex-col">
        {notice}
        <FileTreeView {...props} />
      </div>
    );
  }
  if (props.isSearchActive && props.searchResults !== null) {
    return (
      <SearchResultsList
        searchResults={props.searchResults}
        fileStatuses={props.fileStatuses}
        onOpenFile={props.onOpenFile}
        showTouchActions={props.showTouchActions}
        onAddToChatContext={props.onAddToChatContext}
      />
    );
  }
  const loadStateResult = renderSessionOrLoadState({
    isSessionFailed: props.isSessionFailed,
    sessionError: props.sessionError,
    loadState: props.loadState,
    isLoadingTree: props.isLoadingTree,
    tree: props.tree,
    loadError: props.loadError,
    onRetry: props.onRetry,
    workspaceRestoration: props.workspaceRestoration,
    onRestoreWorkspace: props.onRestoreWorkspace,
    restoreWorkspaceDisabled: props.restoreWorkspaceDisabled,
  });
  if (loadStateResult) return loadStateResult;
  if (props.tree) return <FileTreeView {...props} />;
  return <div className="p-4 text-sm text-muted-foreground">{t("task:noFilesFound")}</div>;
}
