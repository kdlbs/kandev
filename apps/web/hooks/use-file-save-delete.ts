"use client";

import { useCallback } from "react";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateFileContent, deleteFile } from "@/lib/ws/workspace-files";
import { generateUnifiedDiff, calculateHash } from "@/lib/utils/file-diff";
import type { useToast } from "@/components/toast-provider";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";

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

/** Close the pinned (or preview) editor panel for a path after a remote delete. */
function closeFileEditorPanel(path: string, repo?: string) {
  const dockApi = useDockviewStore.getState().api;
  const itemId = buildRepoScopedItemId(path, repo);
  const pinned = dockApi?.getPanel(`file:${itemId}`);
  if (pinned) {
    dockApi?.removePanel(pinned);
    return;
  }
  const preview = dockApi?.getPanel(PREVIEW_FILE_EDITOR_ID);
  if (preview && (preview.params as Record<string, unknown>)?.previewItemId === itemId) {
    dockApi?.removePanel(preview);
  }
}

export type SaveDeleteParams = {
  activeSessionIdRef: React.MutableRefObject<string | null>;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
  setSavingFiles: React.Dispatch<React.SetStateAction<Set<string>>>;
  toast: ReturnType<typeof useToast>["toast"];
};

async function performSaveFile(path: string, repo: string | undefined, params: SaveDeleteParams) {
  const fileKey = buildRepoScopedItemId(path, repo);
  const file = getOpenFiles().get(fileKey);
  if (!file || !file.isDirty) return;
  const client = getWebSocketClient();
  const currentSessionId = params.activeSessionIdRef.current;
  if (!client || !currentSessionId) return;
  params.setSavingFiles((prev) => new Set(prev).add(fileKey));
  try {
    const diff = generateUnifiedDiff(file.originalContent, file.content, file.path);
    const response = await updateFileContent(client, currentSessionId, {
      path: file.path,
      diff,
      originalHash: file.originalHash,
      desiredContent: file.content,
      repo: file.repo,
    });
    if (response.success && response.new_hash) {
      // Re-read current state: user may have typed more while the save was
      // in flight. Only mark clean if content still matches what was saved.
      const current = getOpenFiles().get(fileKey);
      const stillClean = current?.content === file.content;
      params.updateFileState(fileKey, {
        originalContent: file.content,
        originalHash: response.new_hash,
        isDirty: !stillClean,
        hasRemoteUpdate: false,
        remoteContent: undefined,
        remoteOriginalHash: undefined,
      });
      if (stillClean) updatePanelAfterSave(file.path, file.name, file.repo);
      if (response.resolution === "overwritten") {
        params.toast({
          title: "File saved (overwritten)",
          description: "The file was modified externally. Your version was saved.",
          variant: "default",
        });
      }
    } else {
      params.toast({
        title: "Save failed",
        description: response.error || "Failed to save file",
        variant: "error",
      });
    }
  } catch (error) {
    params.toast({
      title: "Save failed",
      description:
        error instanceof Error ? error.message : "An error occurred while saving the file",
      variant: "error",
    });
  } finally {
    params.setSavingFiles((prev) => {
      const next = new Set(prev);
      next.delete(fileKey);
      return next;
    });
  }
}

export function useSaveDeleteActions(params: SaveDeleteParams) {
  const { activeSessionIdRef, updateFileState, toast } = params;

  const saveFile = useCallback(
    (path: string, repo?: string) => performSaveFile(path, repo, params),
    [params],
  );

  const deleteFileAction = useCallback(
    async (path: string, repo?: string) => {
      const client = getWebSocketClient();
      const currentSessionId = activeSessionIdRef.current;
      if (!client || !currentSessionId) return;
      try {
        const fileKey = buildRepoScopedItemId(path, repo);
        const fileRepo = getOpenFiles().get(fileKey)?.repo ?? repo;
        const response = await deleteFile(client, currentSessionId, path, fileRepo);
        if (!response.success) {
          toast({
            title: "Delete failed",
            description: response.error || "Failed to delete file",
            variant: "error",
          });
          return;
        }
      } catch (error) {
        toast({
          title: "Delete failed",
          description:
            error instanceof Error ? error.message : "An error occurred while deleting the file",
          variant: "error",
        });
        return;
      }
      // Close the panel only after the remote delete succeeds.
      closeFileEditorPanel(path, repo);
    },
    [activeSessionIdRef, toast],
  );

  const applyRemoteUpdate = useCallback(
    async (path: string, repo?: string) => {
      const fileKey = buildRepoScopedItemId(path, repo);
      const file = getOpenFiles().get(fileKey);
      if (!file || !file.hasRemoteUpdate || file.remoteContent === undefined) return;
      const remoteHash = file.remoteOriginalHash ?? (await calculateHash(file.remoteContent));
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
    [updateFileState],
  );

  return { saveFile, deleteFileAction, applyRemoteUpdate };
}
