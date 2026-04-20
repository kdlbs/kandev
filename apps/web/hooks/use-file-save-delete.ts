"use client";

import { useCallback } from "react";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateFileContent, deleteFile } from "@/lib/ws/workspace-files";
import { generateUnifiedDiff, calculateHash } from "@/lib/utils/file-diff";
import type { useToast } from "@/components/toast-provider";
import { PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";

/** Read openFiles from the store without subscribing to changes. */
function getOpenFiles() {
  return useDockviewStore.getState().openFiles;
}

/** Update dockview panel dirty state after a successful save. */
export function updatePanelAfterSave(path: string, name: string) {
  const dockApi = useDockviewStore.getState().api;
  const panel =
    dockApi?.getPanel(`file:${path}`) ??
    (() => {
      const preview = dockApi?.getPanel(PREVIEW_FILE_EDITOR_ID);
      return (preview?.params as Record<string, unknown> | undefined)?.previewItemId === path
        ? preview
        : undefined;
    })();
  if (panel) {
    panel.api.updateParameters({ ...(panel.params ?? {}), isDirty: false });
    panel.setTitle(name);
  }
}

export type SaveDeleteParams = {
  activeSessionIdRef: React.MutableRefObject<string | null>;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
  setSavingFiles: React.Dispatch<React.SetStateAction<Set<string>>>;
  toast: ReturnType<typeof useToast>["toast"];
};

async function performSaveFile(path: string, params: SaveDeleteParams) {
  const file = getOpenFiles().get(path);
  if (!file || !file.isDirty) return;
  const client = getWebSocketClient();
  const currentSessionId = params.activeSessionIdRef.current;
  if (!client || !currentSessionId) return;
  params.setSavingFiles((prev) => new Set(prev).add(path));
  try {
    const diff = generateUnifiedDiff(file.originalContent, file.content, file.path);
    const response = await updateFileContent(client, currentSessionId, {
      path,
      diff,
      originalHash: file.originalHash,
      desiredContent: file.content,
    });
    if (response.success && response.new_hash) {
      // Re-read current state: user may have typed more while the save was
      // in flight. Only mark clean if content still matches what was saved.
      const current = getOpenFiles().get(path);
      const stillClean = current?.content === file.content;
      params.updateFileState(path, {
        originalContent: file.content,
        originalHash: response.new_hash,
        isDirty: !stillClean,
        hasRemoteUpdate: false,
        remoteContent: undefined,
        remoteOriginalHash: undefined,
      });
      if (stillClean) updatePanelAfterSave(path, file.name);
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
      next.delete(path);
      return next;
    });
  }
}

export function useSaveDeleteActions(params: SaveDeleteParams) {
  const { activeSessionIdRef, updateFileState, toast } = params;

  const saveFile = useCallback((path: string) => performSaveFile(path, params), [params]);

  const deleteFileAction = useCallback(
    async (path: string) => {
      const client = getWebSocketClient();
      const currentSessionId = activeSessionIdRef.current;
      if (!client || !currentSessionId) return;
      try {
        const response = await deleteFile(client, currentSessionId, path);
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
      const dockApi = useDockviewStore.getState().api;
      const pinned = dockApi?.getPanel(`file:${path}`);
      if (pinned) {
        dockApi?.removePanel(pinned);
      } else {
        const preview = dockApi?.getPanel(PREVIEW_FILE_EDITOR_ID);
        if (preview && (preview.params as Record<string, unknown>)?.previewItemId === path) {
          dockApi?.removePanel(preview);
        }
      }
    },
    [activeSessionIdRef, toast],
  );

  const applyRemoteUpdate = useCallback(
    async (path: string) => {
      const file = getOpenFiles().get(path);
      if (!file || !file.hasRemoteUpdate || file.remoteContent === undefined) return;
      const remoteHash = file.remoteOriginalHash ?? (await calculateHash(file.remoteContent));
      updateFileState(path, {
        content: file.remoteContent,
        originalContent: file.remoteContent,
        originalHash: remoteHash,
        isDirty: false,
        hasRemoteUpdate: false,
        remoteContent: undefined,
        remoteOriginalHash: undefined,
      });
      updatePanelAfterSave(path, file.name);
    },
    [updateFileState],
  );

  return { saveFile, deleteFileAction, applyRemoteUpdate };
}
