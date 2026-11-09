import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { getWebSocketClient } from "@/lib/ws/connection";
import { t } from "@/lib/i18n";
import { isRenameReserved, trackRenameOpen, treePath } from "./file-rename-continuity";
import {
  fetchFileEditorState,
  getPreviewItemIdToRemoveOnReplace,
  type FileEditorRequestToken,
} from "./file-editor-state";
import type { FileEditorActionsParams } from "./use-file-editors";

// i18n-exempt: translation key
const FAILED_TO_OPEN_FILE = "task:failedToOpenFile";

function getOpenFiles() {
  return useDockviewStore.getState().openFiles;
}

function findOpenFile(path: string, repo?: string) {
  const files = getOpenFiles();
  const exact = files.get(buildRepoScopedItemId(path, repo));
  if (exact) return exact;
  const requested = treePath({ path, repo });
  return requested ? [...files.values()].find((file) => treePath(file) === requested) : undefined;
}

function addFileEditorPanelWithPreviewCleanup(
  filePath: string,
  name: string,
  repo: string | undefined,
  addFileEditorPanel: FileEditorActionsParams["addFileEditorPanel"],
  removeFileState: FileEditorActionsParams["removeFileState"],
) {
  const itemId = buildRepoScopedItemId(filePath, repo);
  const previewItemIdToRemove = getPreviewItemIdToRemoveOnReplace(
    useDockviewStore.getState().api,
    itemId,
  );
  addFileEditorPanel(filePath, name, { repo });
  if (previewItemIdToRemove) removeFileState(previewItemIdToRemove);
}

function markActiveFileRequest(
  ref: React.MutableRefObject<FileEditorRequestToken | null>,
  fileKey: string,
): FileEditorRequestToken {
  const requestToken = {
    fileKey,
    generation: (ref.current?.generation ?? 0) + 1,
  };
  ref.current = requestToken;
  return requestToken;
}

type OpenFileActionParams = Pick<
  FileEditorActionsParams,
  "activeSessionIdRef" | "setFileState" | "removeFileState" | "addFileEditorPanel" | "toast"
> & {
  activeFileRequestRef: React.MutableRefObject<FileEditorRequestToken | null>;
};

export function useOpenFileAction({
  activeSessionIdRef,
  activeFileRequestRef,
  setFileState,
  removeFileState,
  addFileEditorPanel,
  toast,
}: OpenFileActionParams) {
  const store = useAppStoreApi();
  return useCallback(
    async (filePath: string, repo?: string) => {
      const client = getWebSocketClient();
      const currentSessionId = activeSessionIdRef.current;
      if (!client || !currentSessionId) return;
      if (isRenameReserved(store, currentSessionId, filePath, repo)) {
        toast({
          title: t(FAILED_TO_OPEN_FILE),
          variant: "error",
        });
        return;
      }
      const fileKey = buildRepoScopedItemId(filePath, repo);
      const existing = findOpenFile(filePath, repo);
      if (existing) {
        addFileEditorPanelWithPreviewCleanup(
          existing.path,
          existing.name,
          existing.repo,
          addFileEditorPanel,
          removeFileState,
        );
        return;
      }
      const requestToken = markActiveFileRequest(activeFileRequestRef, fileKey);
      const pendingOpen = trackRenameOpen(store, currentSessionId, filePath, repo);
      try {
        const state = await fetchFileEditorState({
          client,
          sessionId: currentSessionId,
          filePath,
          repo,
          activeSessionIdRef,
          activeRequestRef: activeFileRequestRef,
          requestToken,
        });
        if (
          !state ||
          !pendingOpen.current() ||
          isRenameReserved(store, currentSessionId, filePath, repo)
        )
          return;
        // Create the panel BEFORE setting file state. The openFiles subscription
        // triggers tab persistence — it needs the dockview panel to already exist
        // so buildPersistedTabs can detect whether the file is preview or pinned.
        addFileEditorPanelWithPreviewCleanup(
          filePath,
          state.name,
          repo,
          addFileEditorPanel,
          removeFileState,
        );
        setFileState(fileKey, state);
      } catch (error) {
        toast({
          title: t(FAILED_TO_OPEN_FILE),
          description: error instanceof Error ? error.message : t("common:unknownError"),
          variant: "error",
        });
      } finally {
        pendingOpen.dispose();
      }
    },
    [
      store,
      activeSessionIdRef,
      activeFileRequestRef,
      addFileEditorPanel,
      removeFileState,
      setFileState,
      toast,
    ],
  );
}

type MarkdownPreviewActionParams = Pick<
  FileEditorActionsParams,
  | "activeSessionIdRef"
  | "setFileState"
  | "updateFileState"
  | "removeFileState"
  | "addFileEditorPanel"
  | "toast"
> & {
  activeFileRequestRef: React.MutableRefObject<FileEditorRequestToken | null>;
};

export function useMarkdownPreviewAction({
  activeSessionIdRef,
  activeFileRequestRef,
  setFileState,
  updateFileState,
  removeFileState,
  addFileEditorPanel,
  toast,
}: MarkdownPreviewActionParams) {
  const store = useAppStoreApi();
  return useCallback(
    async (filePath: string, repo?: string) => {
      const client = getWebSocketClient();
      const currentSessionId = activeSessionIdRef.current;
      if (!client || !currentSessionId) return;
      if (isRenameReserved(store, currentSessionId, filePath, repo)) {
        toast({
          title: t(FAILED_TO_OPEN_FILE),
          variant: "error",
        });
        return;
      }
      const fileKey = buildRepoScopedItemId(filePath, repo);
      const existing = findOpenFile(filePath, repo);
      if (existing) {
        updateFileState(buildRepoScopedItemId(existing.path, existing.repo), {
          renderedPreview: true,
        });
        addFileEditorPanelWithPreviewCleanup(
          existing.path,
          existing.name,
          existing.repo,
          addFileEditorPanel,
          removeFileState,
        );
        return;
      }
      const requestToken = markActiveFileRequest(activeFileRequestRef, fileKey);
      const pendingOpen = trackRenameOpen(store, currentSessionId, filePath, repo);
      try {
        const state = await fetchFileEditorState({
          client,
          sessionId: currentSessionId,
          filePath,
          repo,
          activeSessionIdRef,
          activeRequestRef: activeFileRequestRef,
          requestToken,
        });
        if (
          !state ||
          !pendingOpen.current() ||
          isRenameReserved(store, currentSessionId, filePath, repo)
        )
          return;
        addFileEditorPanelWithPreviewCleanup(
          filePath,
          state.name,
          repo,
          addFileEditorPanel,
          removeFileState,
        );
        setFileState(fileKey, { ...state, renderedPreview: true });
      } catch (error) {
        toast({
          title: t(FAILED_TO_OPEN_FILE),
          description: error instanceof Error ? error.message : t("common:unknownError"),
          variant: "error",
        });
      } finally {
        pendingOpen.dispose();
      }
    },
    [
      store,
      activeSessionIdRef,
      activeFileRequestRef,
      setFileState,
      updateFileState,
      addFileEditorPanel,
      removeFileState,
      toast,
    ],
  );
}
