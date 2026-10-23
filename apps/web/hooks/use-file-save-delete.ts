"use client";

import { useCallback } from "react";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateFileContent, deleteFile, type FileUpdateResponse } from "@/lib/ws/workspace-files";
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
  retainedOwner?:
    | import("@/components/task/file-draft-session-context").FileDraftSessionOwner
    | null;
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
  const retained = params.retainedOwner;
  const retainedVisit = retained?.visitRef.current;
  const epoch = useDockviewStore.getState().fileSessionEpoch;
  const isCanonicalCurrent = () => {
    if (!retained) return isCurrent();
    const state = useDockviewStore.getState();
    return (
      retained.sessionIdRef.current === sessionId &&
      !!retainedVisit &&
      retained.visitRef.current === retainedVisit &&
      state.fileSessionEpoch === epoch &&
      !!file?.instanceId &&
      state.openFiles.get(fileKey)?.instanceId === file.instanceId
    );
  };
  return { sessionId, visit, file, panel, api, isCurrent, isCanonicalCurrent };
}

function buildSavedFileState(file: FileEditorState, current: FileEditorState, newHash: string) {
  const observation = current.workspaceSnapshot;
  // Changed workspace publications install a fresh snapshot object.
  const hasNewObservation =
    observation && observation !== file.workspaceSnapshot && observation.content !== file.content;
  const accepted = current.acceptedWorkspaceSnapshot;
  const acceptedObservation =
    accepted &&
    accepted !== file.acceptedWorkspaceSnapshot &&
    accepted.content !== file.content &&
    current.originalContent === accepted.content &&
    current.originalHash === accepted.hash &&
    (hasNewObservation || accepted === observation);
  const originalContent = acceptedObservation ? current.originalContent : file.content;
  const originalHash = acceptedObservation ? current.originalHash : newHash;
  return {
    originalContent,
    originalHash,
    isDirty: current.content !== originalContent,
    ...(!hasNewObservation && {
      hasRemoteUpdate: false,
      remoteContent: undefined,
      remoteOriginalHash: undefined,
    }),
  };
}

function publishSavedFile(
  fileKey: string,
  file: FileEditorState,
  response: FileUpdateResponse & { new_hash: string },
  params: SaveDeleteParams,
  owner: NonNullable<ReturnType<typeof captureOwner>>,
) {
  const current = getOpenFiles().get(fileKey)!;
  lspClientManager.saveDocument(
    owner.sessionId,
    file.path,
    file.repo,
    file.content,
    current.content,
  );
  const updates = buildSavedFileState(file, current, response.new_hash);
  params.updateFileState(fileKey, updates);
  if (!updates.isDirty && owner.isCurrent()) updatePanelAfterSave(file.path, file.name, file.repo);
  if (owner.isCurrent() && response.resolution === "overwritten") {
    params.toast({
      title: t("editors:fileSavedOverwritten"),
      description: t("editors:fileSavedOverwrittenDescription"),
      variant: "default",
    });
  }
}

function isSavableFile(
  file: FileEditorState | undefined,
): file is FileEditorState & { instanceId: symbol } {
  return !!file?.isDirty && !!file.instanceId;
}

async function performSaveFile(path: string, repo: string | undefined, params: SaveDeleteParams) {
  const fileKey = buildRepoScopedItemId(path, repo);
  const owner = captureOwner(fileKey, params);
  const file = owner?.file;
  if (!owner || !isSavableFile(file)) return;
  const client = getWebSocketClient();
  if (!client) return;
  const marker = {
    sessionId: owner.sessionId,
    visit: owner.visit,
    instanceId: file.instanceId,
    api: owner.api,
  };
  params.setSavingFiles((prev) => (owner.isCurrent() ? new Map(prev).set(fileKey, marker) : prev));
  const operation = Symbol();
  params.updateFileState(fileKey, { saveOperation: operation });
  const ownsSave = () =>
    owner.isCanonicalCurrent() && getOpenFiles().get(fileKey)?.saveOperation === operation;
  try {
    const diff = generateUnifiedDiff(file.originalContent, file.content, file.path);
    const response = await updateFileContent(client, owner.sessionId, {
      path: file.path,
      diff,
      originalHash: file.originalHash,
      desiredContent: file.content,
      repo: file.repo,
    });
    if (!ownsSave()) return;
    if (response.success && response.new_hash) {
      publishSavedFile(fileKey, file, { ...response, new_hash: response.new_hash }, params, owner);
    } else if (owner.isCurrent()) {
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
      const current = getOpenFiles().get(fileKey)!;
      // The buffer lifetime can survive a newer snapshot during hash preparation.
      if (
        !current.hasRemoteUpdate ||
        current.remoteContent !== file.remoteContent ||
        current.remoteOriginalHash !== file.remoteOriginalHash ||
        current.workspaceSnapshot !== file.workspaceSnapshot
      )
        return;
      const acceptedWorkspaceSnapshot = file.workspaceSnapshot ?? {
        content: file.remoteContent,
        hash: remoteHash,
      };
      updateFileState(fileKey, {
        workspaceSnapshot: acceptedWorkspaceSnapshot,
        acceptedWorkspaceSnapshot,
        content: file.remoteContent,
        originalContent: file.remoteContent,
        originalHash: remoteHash,
        isDirty: false,
        hasRemoteUpdate: false,
        remoteContent: undefined,
        remoteOriginalHash: undefined,
      });
      if (owner.isCurrent()) updatePanelAfterSave(file.path, file.name, file.repo);
    },
    [params, updateFileState],
  );

  return { saveFile, deleteFileAction, applyRemoteUpdate };
}
