"use client";

import { useCallback } from "react";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateFileContent, deleteFile } from "@/lib/ws/workspace-files";
import { generateUnifiedDiff, calculateHash } from "@/lib/utils/file-diff";
import type { useToast } from "@/components/toast-provider";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";
import { lspClientManager } from "@/lib/lsp/lsp-client-manager";
import { t } from "@/lib/i18n";

/** Read openFiles from the store without subscribing to changes. */
function getOpenFiles() {
  return useDockviewStore.getState().openFiles;
}

/** Update dockview panel dirty state after a successful save. */
export function updatePanelAfterSave(path: string, name: string, repo?: string) {
  const dockApi = useDockviewStore.getState().api;
  const itemId = buildRepoScopedItemId(path, repo);
  const panel =
    dockApi?.getPanel(`file:${itemId}`) ??
    (() => {
      const preview = dockApi?.getPanel(PREVIEW_FILE_EDITOR_ID);
      return (preview?.params as Record<string, unknown> | undefined)?.previewItemId === itemId
        ? preview
        : undefined;
    })();
  if (panel) {
    panel.api.updateParameters({ ...(panel.params ?? {}), isDirty: false });
    panel.setTitle(name);
  }
}

/** Find the current pinned or preview panel for a repo-scoped file key. */
function getFileEditorPanel(fileKey: string) {
  const dockApi = useDockviewStore.getState().api;
  const pinned = dockApi?.getPanel(`file:${fileKey}`);
  if (pinned) return pinned;
  const preview = dockApi?.getPanel(PREVIEW_FILE_EDITOR_ID);
  return preview?.params?.previewItemId === fileKey ? preview : undefined;
}

export type PendingFileSave = {
  sessionId: string;
  visit: symbol;
  instanceId: symbol;
  api: ReturnType<typeof useDockviewStore.getState>["api"];
};

export type SaveDeleteParams = {
  activeSessionIdRef: React.MutableRefObject<string | null>;
  activeEditorVisitRef: React.MutableRefObject<symbol | null>;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
  setSavingFiles: React.Dispatch<React.SetStateAction<Map<string, PendingFileSave>>>;
  toast: ReturnType<typeof useToast>["toast"];
};

function captureOwner(fileKey: string, params: SaveDeleteParams) {
  const sessionId = params.activeSessionIdRef.current;
  const visit = params.activeEditorVisitRef.current;
  if (!sessionId || !visit) return null;
  const { api, openFiles } = useDockviewStore.getState();
  const file = openFiles.get(fileKey);
  if (file && !file.instanceId) return null;
  const panel = getFileEditorPanel(fileKey);
  const isCurrent = () => {
    if (
      params.activeSessionIdRef.current !== sessionId ||
      params.activeEditorVisitRef.current !== visit
    )
      return false;
    const state = useDockviewStore.getState();
    if (state.api !== api) return false;
    const current = state.openFiles.get(fileKey);
    if (file) return !!file.instanceId && current?.instanceId === file.instanceId;
    return !current && getFileEditorPanel(fileKey) === panel;
  };
  return { sessionId, visit, file, panel, api, isCurrent };
}

function publishSavedFile(
  fileKey: string,
  file: FileEditorState,
  newHash: string,
  sessionId: string,
  params: SaveDeleteParams,
) {
  // Typing preserves the buffer lifetime while the saved snapshot stays fixed.
  const current = getOpenFiles().get(fileKey)!;
  lspClientManager.saveDocument(sessionId, file.path, file.repo, file.content, current.content);
  const stillClean = current.content === file.content;
  params.updateFileState(fileKey, {
    originalContent: file.content,
    originalHash: newHash,
    isDirty: !stillClean,
    hasRemoteUpdate: false,
    remoteContent: undefined,
    remoteOriginalHash: undefined,
  });
  if (stillClean) updatePanelAfterSave(file.path, file.name, file.repo);
}

async function performSaveFile(path: string, repo: string | undefined, params: SaveDeleteParams) {
  const fileKey = buildRepoScopedItemId(path, repo);
  const owner = captureOwner(fileKey, params);
  const file = owner?.file;
  if (!owner || !file?.isDirty || !file.instanceId) return;
  const client = getWebSocketClient();
  if (!client) return;
  const marker = {
    sessionId: owner.sessionId,
    visit: owner.visit,
    instanceId: file.instanceId,
    api: owner.api,
  };
  params.setSavingFiles((prev) => (owner.isCurrent() ? new Map(prev).set(fileKey, marker) : prev));
  try {
    const diff = generateUnifiedDiff(file.originalContent, file.content, file.path);
    const response = await updateFileContent(client, owner.sessionId, {
      path: file.path,
      diff,
      originalHash: file.originalHash,
      desiredContent: file.content,
      repo: file.repo,
    });
    if (!owner.isCurrent()) return;
    if (response.success && response.new_hash) {
      publishSavedFile(fileKey, file, response.new_hash, owner.sessionId, params);
      if (response.resolution === "overwritten") {
        params.toast({
          title: t("editors:fileSavedOverwritten"),
          description: t("editors:fileSavedOverwrittenDescription"),
          variant: "default",
        });
      }
    } else {
      params.toast({
        title: t("editors:saveFailed"),
        description: response.error || t("editors:failedToSaveFile"),
        variant: "error",
      });
    }
  } catch (error) {
    if (!owner.isCurrent()) return;
    params.toast({
      title: t("editors:saveFailed"),
      description: error instanceof Error ? error.message : t("editors:errorWhileSavingFile"),
      variant: "error",
    });
  } finally {
    params.setSavingFiles((prev) => {
      if (prev.get(fileKey) !== marker) return prev;
      const next = new Map(prev);
      next.delete(fileKey);
      return next;
    });
  }
}

export function useSaveDeleteActions(params: SaveDeleteParams) {
  const { updateFileState, toast } = params;

  const saveFile = useCallback(
    (path: string, repo?: string) => performSaveFile(path, repo, params),
    [params],
  );

  const deleteFileAction = useCallback(
    async (path: string, repo?: string) => {
      const client = getWebSocketClient();
      const fileKey = buildRepoScopedItemId(path, repo);
      const owner = captureOwner(fileKey, params);
      if (!client || !owner) return;
      try {
        const fileRepo = owner.file?.repo ?? repo;
        const response = await deleteFile(client, owner.sessionId, path, fileRepo);
        if (!owner.isCurrent()) return;
        if (!response.success) {
          toast({
            title: t("editors:deleteFailed"),
            description: response.error || t("editors:failedToDeleteFile"),
            variant: "error",
          });
          return;
        }
      } catch (error) {
        if (!owner.isCurrent()) return;
        toast({
          title: t("editors:deleteFailed"),
          description: error instanceof Error ? error.message : t("editors:errorWhileDeletingFile"),
          variant: "error",
        });
        return;
      }
      // Close the panel only after the remote delete succeeds.
      const panel = owner.panel && getFileEditorPanel(fileKey);
      if (panel) owner.api?.removePanel(panel);
    },
    [params, toast],
  );

  const applyRemoteUpdate = useCallback(
    async (path: string, repo?: string) => {
      const fileKey = buildRepoScopedItemId(path, repo);
      const owner = captureOwner(fileKey, params);
      const file = owner?.file;
      if (!owner || !file?.hasRemoteUpdate || file.remoteContent === undefined) return;
      const remoteHash = file.remoteOriginalHash ?? (await calculateHash(file.remoteContent));
      if (!owner.isCurrent()) return;
      updateFileState(fileKey, {
        content: file.remoteContent,
        originalContent: file.remoteContent,
        originalHash: remoteHash,
        isDirty: false,
        hasRemoteUpdate: false,
        remoteContent: undefined,
        remoteOriginalHash: undefined,
      });
      updatePanelAfterSave(file.path, file.name, file.repo);
    },
    [params, updateFileState],
  );

  return { saveFile, deleteFileAction, applyRemoteUpdate };
}
