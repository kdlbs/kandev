"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import {
  getOpenFileTabs,
  setOpenFileTabs as saveOpenFileTabs,
  getActiveTabForSession,
  setActiveTabForSession,
} from "@/lib/local-storage";
import { calculateHash } from "@/lib/utils/file-diff";
import { useToast } from "@/components/toast-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import type { FileContentResponse } from "@/lib/types/backend";
import { useSaveDeleteActions } from "./use-file-save-delete";
import { PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";
import { useOpenFileWorkspaceSync } from "./file-editors-sync";

// Module-level guard: ensures restoration only runs once across all hook instances
let _restoredSessionId: string | null = null;
let _restorationInProgress = false;

// Pending cursor positions: set before opening a file, consumed by the editor on mount.
// Used by LSP Go-to-Definition to jump to the correct line/column.
const _pendingCursorPositions = new Map<string, { line: number; column: number }>();

export function setPendingCursorPosition(path: string, line: number, column: number) {
  _pendingCursorPositions.set(path, { line, column });
}

export function consumePendingCursorPosition(
  path: string,
): { line: number; column: number } | undefined {
  const pos = _pendingCursorPositions.get(path);
  if (pos) _pendingCursorPositions.delete(path);
  return pos;
}

/** Read openFiles from the store without subscribing to changes. */
function getOpenFiles() {
  return useDockviewStore.getState().openFiles;
}

/** Build a FileEditorState from a file content response. */
async function buildFileEditorState(
  filePath: string,
  response: FileContentResponse,
): Promise<FileEditorState> {
  const fileName = filePath.split("/").pop() || filePath;
  const hash = await calculateHash(response.content);
  return {
    path: filePath,
    name: fileName,
    content: response.content,
    originalContent: response.content,
    originalHash: hash,
    isDirty: false,
    isBinary: response.is_binary,
    resolvedPath: response.resolved_path,
  };
}

/** Build the sessionStorage tab records from live openFiles + dockview state. */
function buildPersistedTabs(
  api: ReturnType<typeof useDockviewStore.getState>["api"],
  openFiles: Map<string, FileEditorState>,
) {
  const preview = api?.getPanel(PREVIEW_FILE_EDITOR_ID);
  const previewParams = preview?.params as Record<string, unknown> | undefined;
  const previewPath = (previewParams?.previewItemId ?? null) as string | null;
  const isPromoted = previewParams?.promoted === true;
  return Array.from(openFiles.values()).flatMap(({ path, name, markdownPreview }) => {
    const isPinned = !!api?.getPanel(`file:${path}`);
    const isPreview = !isPinned && path === previewPath;
    if (!isPinned && !isPreview) return [];
    // Promoted previews persist as pinned so edits survive refresh
    const persistAsPinned = isPinned || (isPreview && isPromoted);
    return [
      { path, name, ...(markdownPreview ? { markdownPreview } : {}), pinned: persistAsPinned },
    ];
  });
}

type RestoreTabsParams = {
  activeSessionId: string;
  savedTabs: Array<{ path: string; name: string; markdownPreview?: boolean; pinned?: boolean }>;
  savedActiveTab: string;
  setFileState: (path: string, state: FileEditorState) => void;
  addFileEditorPanel: (
    path: string,
    name: string,
    opts?: { quiet?: boolean; pin?: boolean },
  ) => void;
};

/**
 * Skip persisted tabs the dockview snapshot already restored (preview slot or
 * existing pinned panel). Re-adding with `pin: true` here would create a
 * duplicate `file:<path>` alongside the existing `preview:file-editor` —
 * see the round-trip task switch where a promoted preview was persisted as
 * pinned in localStorage and then re-opened on top of the restored preview.
 */
function isAlreadyRestored(
  dockApi: ReturnType<typeof useDockviewStore.getState>["api"],
  path: string,
): boolean {
  if (!dockApi) return false;
  if (dockApi.getPanel(`file:${path}`)) return true;
  const previewParams = dockApi.getPanel(PREVIEW_FILE_EDITOR_ID)?.params as
    | Record<string, unknown>
    | undefined;
  return previewParams?.previewItemId === path;
}

async function loadAndRestoreTabs(params: RestoreTabsParams, retryCount = 0): Promise<void> {
  const { activeSessionId, savedTabs, savedActiveTab, setFileState, addFileEditorPanel } = params;
  const client = getWebSocketClient();
  if (!client) {
    if (retryCount < 5) {
      setTimeout(() => loadAndRestoreTabs(params, retryCount + 1), 200);
      return;
    }
    _restorationInProgress = false;
    return;
  }
  if (_restoredSessionId !== activeSessionId) {
    _restorationInProgress = false;
    return;
  }
  // Create all panels immediately so tabs are visible right away.
  // Content is fetched afterwards; if it fails, `useFileLoader` in
  // FileEditorPanel retries when the executor becomes available.
  const dockApi = useDockviewStore.getState().api;
  for (const savedTab of savedTabs) {
    if (isAlreadyRestored(dockApi, savedTab.path)) continue;
    addFileEditorPanel(savedTab.path, savedTab.name, {
      quiet: true,
      pin: savedTab.pinned,
    });
  }
  for (const savedTab of savedTabs) {
    try {
      const response = await requestFileContent(client, activeSessionId, savedTab.path);
      const hash = await calculateHash(response.content);
      setFileState(savedTab.path, {
        path: savedTab.path,
        name: savedTab.name,
        content: response.content,
        originalContent: response.content,
        originalHash: hash,
        isDirty: false,
        isBinary: response.is_binary,
        markdownPreview: savedTab.markdownPreview,
      });
    } catch {
      /* useFileLoader will retry when executor is ready */
    }
  }
  const targetPanel = dockApi?.getPanel(savedActiveTab);
  if (targetPanel) targetPanel.api.setActive();
  _restorationInProgress = false;
}

type FileEditorEffectsParams = {
  activeSessionId: string | null;
  activeSessionIdRef: React.MutableRefObject<string | null>;
  setFileState: (path: string, state: FileEditorState) => void;
  addFileEditorPanel: (
    path: string,
    name: string,
    opts?: { quiet?: boolean; pin?: boolean },
  ) => void;
  clearFileStates: () => void;
  removeFileState: (path: string) => void;
  api: ReturnType<typeof useDockviewStore.getState>["api"];
};

function useFileEditorEffects({
  activeSessionId,
  activeSessionIdRef,
  setFileState,
  addFileEditorPanel,
  clearFileStates,
  removeFileState,
  api,
}: FileEditorEffectsParams) {
  useEffect(() => {
    if (!activeSessionId || _restoredSessionId === activeSessionId) return;
    _restoredSessionId = activeSessionId;
    // Set the flag BEFORE clearing so the openFiles subscription doesn't
    // overwrite saved tabs with an empty list during the clear.
    _restorationInProgress = true;
    clearFileStates();
    const savedTabs = getOpenFileTabs(activeSessionId);
    const savedActiveTab = getActiveTabForSession(activeSessionId, "chat");
    if (savedTabs.length === 0) {
      _restorationInProgress = false;
      return;
    }
    void loadAndRestoreTabs({
      activeSessionId,
      savedTabs,
      savedActiveTab,
      setFileState,
      addFileEditorPanel,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSessionId]);

  useEffect(() => {
    const unsub = useDockviewStore.subscribe((state, prevState) => {
      if (state.openFiles === prevState.openFiles) return;
      const sessionId = activeSessionIdRef.current;
      if (!sessionId || _restorationInProgress || state.isRestoringLayout) return;
      saveOpenFileTabs(sessionId, buildPersistedTabs(state.api, state.openFiles));
    });
    return unsub;
  }, [activeSessionIdRef]);

  useEffect(() => {
    if (!api || !activeSessionId) return;
    const disposable = api.onDidActivePanelChange((event) => {
      if (_restorationInProgress) return;
      if (event) setActiveTabForSession(activeSessionId, event.id);
    });
    return () => disposable.dispose();
  }, [api, activeSessionId]);

  useEffect(() => {
    if (!api) return;
    const disposable = api.onDidRemovePanel((event) => {
      if (event.id.startsWith("file:")) {
        removeFileState(event.id.replace("file:", ""));
        return;
      }
      // Preview panel closed: drop whichever file it was showing — but NOT if a
      // pinned panel for the same file already exists (e.g. the preview was
      // just promoted to pinned, which removes the preview before creating the
      // pinned panel; wiping the file state here would drop the user's dirty
      // buffer during auto-promote-on-edit).
      if (event.id === PREVIEW_FILE_EDITOR_ID) {
        const path = (event.params?.previewItemId as string | undefined) ?? null;
        if (!path) return;
        const pinnedStillOpen = !!api.getPanel(`file:${path}`);
        if (!pinnedStillOpen) removeFileState(path);
      }
    });
    return () => disposable.dispose();
  }, [api, removeFileState]);
}

type FileEditorActionsParams = {
  activeSessionIdRef: React.MutableRefObject<string | null>;
  setFileState: (path: string, state: FileEditorState) => void;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
  addFileEditorPanel: (
    path: string,
    name: string,
    opts?: { quiet?: boolean; pin?: boolean },
  ) => void;
  promotePreviewToPinned: (type: "file-editor") => void;
  setSavingFiles: React.Dispatch<React.SetStateAction<Set<string>>>;
  toast: ReturnType<typeof useToast>["toast"];
};

function useFileEditorActions({
  activeSessionIdRef,
  setFileState,
  updateFileState,
  addFileEditorPanel,
  promotePreviewToPinned,
  setSavingFiles,
  toast,
}: FileEditorActionsParams) {
  const openFile = useCallback(
    async (filePath: string) => {
      const client = getWebSocketClient();
      const currentSessionId = activeSessionIdRef.current;
      if (!client || !currentSessionId) return;
      const files = getOpenFiles();
      if (files.has(filePath)) {
        addFileEditorPanel(filePath, filePath.split("/").pop() || filePath);
        return;
      }
      try {
        const response: FileContentResponse = await requestFileContent(
          client,
          currentSessionId,
          filePath,
        );
        const state = await buildFileEditorState(filePath, response);
        // Create the panel BEFORE setting file state. The openFiles subscription
        // triggers tab persistence — it needs the dockview panel to already exist
        // so buildPersistedTabs can detect whether the file is preview or pinned.
        addFileEditorPanel(filePath, state.name);
        setFileState(filePath, state);
      } catch (error) {
        toast({
          title: "Failed to open file",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [activeSessionIdRef, addFileEditorPanel, setFileState, toast],
  );

  const handleFileChange = useCallback(
    (path: string, newContent: string) => {
      const file = getOpenFiles().get(path);
      if (!file) return;
      const nextIsDirty = newContent !== file.originalContent;
      // VSCode-style: editing a preview file auto-promotes its tab so the
      // user's unsaved changes aren't silently discarded when they open
      // another file. Promote BEFORE updating file state so the openFiles
      // subscription sees the promoted flag when it fires from updateFileState.
      if (nextIsDirty && !file.isDirty) {
        const preview = useDockviewStore.getState().api?.getPanel(PREVIEW_FILE_EDITOR_ID);
        if ((preview?.params as Record<string, unknown> | undefined)?.previewItemId === path) {
          promotePreviewToPinned("file-editor");
        }
      }
      updateFileState(path, { content: newContent, isDirty: nextIsDirty });
    },
    [updateFileState, promotePreviewToPinned],
  );

  const { saveFile, deleteFileAction, applyRemoteUpdate } = useSaveDeleteActions({
    activeSessionIdRef,
    updateFileState,
    setSavingFiles,
    toast,
  });

  const openFileInMarkdownPreview = useCallback(
    async (filePath: string) => {
      const client = getWebSocketClient();
      const currentSessionId = activeSessionIdRef.current;
      if (!client || !currentSessionId) return;
      const files = getOpenFiles();
      if (files.has(filePath)) {
        updateFileState(filePath, { markdownPreview: true });
        addFileEditorPanel(filePath, filePath.split("/").pop() || filePath);
        return;
      }
      try {
        const response: FileContentResponse = await requestFileContent(
          client,
          currentSessionId,
          filePath,
        );
        const state = await buildFileEditorState(filePath, response);
        addFileEditorPanel(filePath, state.name);
        setFileState(filePath, { ...state, markdownPreview: true });
      } catch (error) {
        toast({
          title: "Failed to open file",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [activeSessionIdRef, setFileState, updateFileState, addFileEditorPanel, toast],
  );

  return {
    openFile,
    openFileInMarkdownPreview,
    handleFileChange,
    saveFile,
    deleteFileAction,
    applyRemoteUpdate,
  };
}

export function useFileEditors() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { toast } = useToast();
  const [savingFiles, setSavingFiles] = useState<Set<string>>(new Set());

  const setFileState = useDockviewStore((s) => s.setFileState);
  const updateFileState = useDockviewStore((s) => s.updateFileState);
  const removeFileState = useDockviewStore((s) => s.removeFileState);
  const clearFileStates = useDockviewStore((s) => s.clearFileStates);
  const addFileEditorPanel = useDockviewStore((s) => s.addFileEditorPanel);
  const promotePreviewToPinned = useDockviewStore((s) => s.promotePreviewToPinned);
  const openFiles = useDockviewStore((s) => s.openFiles);
  const api = useDockviewStore((s) => s.api);
  const gitFileSignaturesRef = useRef<Map<string, string>>(new Map());

  const activeSessionIdRef = useRef(activeSessionId);
  useEffect(() => {
    activeSessionIdRef.current = activeSessionId;
  }, [activeSessionId]);

  useFileEditorEffects({
    activeSessionId,
    activeSessionIdRef,
    setFileState,
    addFileEditorPanel,
    clearFileStates,
    removeFileState,
    api,
  });
  useOpenFileWorkspaceSync({
    gitStatus,
    openFiles,
    updateFileState,
    activeSessionIdRef,
    gitFileSignaturesRef,
  });
  const {
    openFile,
    openFileInMarkdownPreview,
    handleFileChange,
    saveFile,
    deleteFileAction,
    applyRemoteUpdate,
  } = useFileEditorActions({
    activeSessionIdRef,
    setFileState,
    updateFileState,
    addFileEditorPanel,
    promotePreviewToPinned,
    setSavingFiles,
    toast,
  });

  return {
    savingFiles,
    openFile,
    openFileInMarkdownPreview,
    saveFile,
    deleteFile: deleteFileAction,
    handleFileChange,
    applyRemoteUpdate,
  };
}
