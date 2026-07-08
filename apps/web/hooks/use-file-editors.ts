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
import { useSaveDeleteActions } from "./use-file-save-delete";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";
import { useOpenFileWorkspaceSync } from "./file-editors-sync";
import { getMonacoInstance } from "@/components/editors/monaco/monaco-init";
import {
  fetchFileEditorState,
  getPreviewItemIdToRemoveOnReplace,
  isFileEditorPanelAlreadyRestored,
  isRestoreWriteCurrent,
} from "./file-editor-state";

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

/**
 * Scroll an already-mounted Monaco editor for `path` to the given position.
 * Returns true if an editor was found and scrolled. The editor's model URI is
 * built as `${worktreePath}/${path}` (see use-monaco-editor-lsp), so the same
 * `path` used for setPendingCursorPosition resolves the right editor here.
 *
 * Used for the already-open case: when a file link is clicked but its tab is
 * already open, no editor mounts, so handleEditorDidMount never consumes the
 * pending position — this scrolls the live editor instead (and consumes the
 * pending entry so a later remount doesn't double-apply it).
 */
export function scrollEditorIfMounted(
  path: string,
  worktreePath: string | null,
  line: number,
  column: number,
): boolean {
  const monaco = getMonacoInstance();
  if (!monaco) return false;

  const monacoPath = worktreePath ? `${worktreePath}/${path}` : path;
  for (const editor of monaco.editor.getEditors()) {
    const model = editor.getModel();
    if (!model) continue;
    const modelPath = model.uri.path;
    if (modelPath === `/${monacoPath}` || modelPath === monacoPath) {
      consumePendingCursorPosition(path);
      editor.setPosition({ lineNumber: line, column });
      editor.revealLineInCenter(line);
      editor.focus();
      return true;
    }
  }
  return false;
}

export function useOpenFileAtLine(
  onOpenFile: ((path: string) => void) | undefined,
  startLine: number | undefined,
  worktreePath: string | null | undefined,
) {
  return useCallback(
    (path: string) => {
      if (startLine && startLine > 0) {
        setPendingCursorPosition(path, startLine, 1);
        onOpenFile?.(path);
        scrollEditorIfMounted(path, worktreePath ?? null, startLine, 1);
        return;
      }
      onOpenFile?.(path);
    },
    [onOpenFile, startLine, worktreePath],
  );
}

/** Read openFiles from the store without subscribing to changes. */
function getOpenFiles() {
  return useDockviewStore.getState().openFiles;
}

/**
 * Apply an editor buffer change, auto-promoting a preview tab to pinned on the
 * first edit so the user's unsaved changes aren't discarded when another file
 * is opened. Promote BEFORE updating state so the openFiles subscription sees
 * the promoted flag when it fires from updateFileState.
 */
function applyFileChange(
  path: string,
  repo: string | undefined,
  newContent: string,
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void,
  promotePreviewToPinned: (type: "file-editor") => void,
) {
  const fileKey = buildRepoScopedItemId(path, repo);
  const file = getOpenFiles().get(fileKey);
  if (!file) return;
  const nextIsDirty = newContent !== file.originalContent;
  if (nextIsDirty && !file.isDirty) {
    const preview = useDockviewStore.getState().api?.getPanel(PREVIEW_FILE_EDITOR_ID);
    if ((preview?.params as Record<string, unknown> | undefined)?.previewItemId === fileKey) {
      promotePreviewToPinned("file-editor");
    }
  }
  updateFileState(fileKey, { content: newContent, isDirty: nextIsDirty });
}

/** Build the sessionStorage tab records from live openFiles + dockview state. */
function buildPersistedTabs(
  api: ReturnType<typeof useDockviewStore.getState>["api"],
  openFiles: Map<string, FileEditorState>,
) {
  const preview = api?.getPanel(PREVIEW_FILE_EDITOR_ID);
  const previewParams = preview?.params as Record<string, unknown> | undefined;
  const previewItemId = (previewParams?.previewItemId ?? null) as string | null;
  const isPromoted = previewParams?.promoted === true;
  return Array.from(openFiles.values()).flatMap(({ path, name, repo, markdownPreview }) => {
    const itemId = buildRepoScopedItemId(path, repo);
    const isPinned = !!api?.getPanel(`file:${itemId}`);
    const isPreview = !isPinned && itemId === previewItemId;
    if (!isPinned && !isPreview) return [];
    // Promoted previews persist as pinned so edits survive refresh
    const persistAsPinned = isPinned || (isPreview && isPromoted);
    return [
      {
        path,
        name,
        ...(repo ? { repo } : {}),
        ...(markdownPreview ? { markdownPreview } : {}),
        pinned: persistAsPinned,
      },
    ];
  });
}

type RestoreTabsParams = {
  activeSessionId: string;
  activeSessionIdRef: React.MutableRefObject<string | null>;
  savedTabs: Array<{
    path: string;
    name: string;
    repo?: string;
    markdownPreview?: boolean;
    pinned?: boolean;
  }>;
  savedActiveTab: string;
  setFileState: (path: string, state: FileEditorState) => void;
  addFileEditorPanel: (
    path: string,
    name: string,
    opts?: { quiet?: boolean; pin?: boolean; repo?: string },
  ) => void;
};

async function loadAndRestoreTabs(params: RestoreTabsParams, retryCount = 0): Promise<void> {
  const {
    activeSessionId,
    activeSessionIdRef,
    savedTabs,
    savedActiveTab,
    setFileState,
    addFileEditorPanel,
  } = params;
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
    const itemId = buildRepoScopedItemId(savedTab.path, savedTab.repo);
    if (isFileEditorPanelAlreadyRestored(dockApi, savedTab.path, savedTab.repo)) continue;
    addFileEditorPanel(savedTab.path, savedTab.name, {
      quiet: true,
      pin: savedTab.pinned,
      repo: savedTab.repo,
    });
    // Seed a placeholder file state synchronously, carrying the restored
    // `markdownPreview` flag. This makes `openFiles.has(path)` true the moment
    // FileEditorPanel mounts, which suppresses its own `useFileLoader` fetch.
    // Without this seed, useFileLoader races the per-tab fetch below: both call
    // setFileState (a wholesale replace), and useFileLoader's state has no
    // markdownPreview — so when it wins the race (common under CPU load) the
    // restored preview flag is clobbered and the tab reopens in code view.
    setFileState(itemId, {
      path: savedTab.path,
      repo: savedTab.repo,
      name: savedTab.name,
      content: "",
      originalContent: "",
      originalHash: "",
      isDirty: false,
      markdownPreview: savedTab.markdownPreview,
    });
  }
  for (const savedTab of savedTabs) {
    try {
      const itemId = buildRepoScopedItemId(savedTab.path, savedTab.repo);
      const response = await requestFileContent(
        client,
        activeSessionId,
        savedTab.path,
        savedTab.repo,
      );
      const hash = await calculateHash(response.content);
      if (!isRestoreWriteCurrent(_restoredSessionId, activeSessionId, activeSessionIdRef)) {
        if (_restoredSessionId === activeSessionId) _restorationInProgress = false;
        return;
      }
      setFileState(itemId, {
        path: savedTab.path,
        repo: savedTab.repo,
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
    opts?: { quiet?: boolean; pin?: boolean; repo?: string },
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
      activeSessionIdRef,
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
        const itemId = (event.params?.previewItemId as string | undefined) ?? null;
        if (!itemId) return;
        const pinnedStillOpen = !!api.getPanel(`file:${itemId}`);
        if (!pinnedStillOpen) removeFileState(itemId);
      }
    });
    return () => disposable.dispose();
  }, [api, removeFileState]);
}

type FileEditorActionsParams = {
  activeSessionIdRef: React.MutableRefObject<string | null>;
  setFileState: (path: string, state: FileEditorState) => void;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
  removeFileState: (path: string) => void;
  addFileEditorPanel: (
    path: string,
    name: string,
    opts?: { quiet?: boolean; pin?: boolean; repo?: string },
  ) => void;
  promotePreviewToPinned: (type: "file-editor") => void;
  setSavingFiles: React.Dispatch<React.SetStateAction<Set<string>>>;
  toast: ReturnType<typeof useToast>["toast"];
};

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

type OpenFileActionParams = Pick<
  FileEditorActionsParams,
  "activeSessionIdRef" | "setFileState" | "removeFileState" | "addFileEditorPanel" | "toast"
> & {
  activeFileRequestRef: React.MutableRefObject<string | null>;
};

function useOpenFileAction({
  activeSessionIdRef,
  activeFileRequestRef,
  setFileState,
  removeFileState,
  addFileEditorPanel,
  toast,
}: OpenFileActionParams) {
  return useCallback(
    async (filePath: string, repo?: string) => {
      const client = getWebSocketClient();
      const currentSessionId = activeSessionIdRef.current;
      if (!client || !currentSessionId) return;
      const fileKey = buildRepoScopedItemId(filePath, repo);
      activeFileRequestRef.current = fileKey;
      const files = getOpenFiles();
      if (files.has(fileKey)) {
        const existing = files.get(fileKey);
        const tabName = filePath.split("/").pop() || filePath;
        addFileEditorPanelWithPreviewCleanup(
          filePath,
          tabName,
          existing?.repo,
          addFileEditorPanel,
          removeFileState,
        );
        return;
      }
      try {
        const state = await fetchFileEditorState({
          client,
          sessionId: currentSessionId,
          filePath,
          repo,
          activeSessionIdRef,
          activeFileKeyRef: activeFileRequestRef,
          fileKey,
        });
        if (!state) return;
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
          title: "Failed to open file",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [
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
  activeFileRequestRef: React.MutableRefObject<string | null>;
};

function useMarkdownPreviewAction({
  activeSessionIdRef,
  activeFileRequestRef,
  setFileState,
  updateFileState,
  removeFileState,
  addFileEditorPanel,
  toast,
}: MarkdownPreviewActionParams) {
  return useCallback(
    async (filePath: string, repo?: string) => {
      const client = getWebSocketClient();
      const currentSessionId = activeSessionIdRef.current;
      if (!client || !currentSessionId) return;
      const fileKey = buildRepoScopedItemId(filePath, repo);
      activeFileRequestRef.current = fileKey;
      const files = getOpenFiles();
      if (files.has(fileKey)) {
        updateFileState(fileKey, { markdownPreview: true });
        const name = filePath.split("/").pop() || filePath;
        addFileEditorPanelWithPreviewCleanup(
          filePath,
          name,
          files.get(fileKey)?.repo,
          addFileEditorPanel,
          removeFileState,
        );
        return;
      }
      try {
        const state = await fetchFileEditorState({
          client,
          sessionId: currentSessionId,
          filePath,
          repo,
          activeSessionIdRef,
          activeFileKeyRef: activeFileRequestRef,
          fileKey,
        });
        if (!state) return;
        addFileEditorPanelWithPreviewCleanup(
          filePath,
          state.name,
          repo,
          addFileEditorPanel,
          removeFileState,
        );
        setFileState(fileKey, { ...state, markdownPreview: true });
      } catch (error) {
        toast({
          title: "Failed to open file",
          description: error instanceof Error ? error.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [activeSessionIdRef, setFileState, updateFileState, addFileEditorPanel, removeFileState, toast],
  );
}

function useFileEditorActions({
  activeSessionIdRef,
  setFileState,
  updateFileState,
  removeFileState,
  addFileEditorPanel,
  promotePreviewToPinned,
  setSavingFiles,
  toast,
}: FileEditorActionsParams) {
  const activeFileRequestRef = useRef<string | null>(null);
  const openFile = useOpenFileAction({
    activeSessionIdRef,
    activeFileRequestRef,
    setFileState,
    removeFileState,
    addFileEditorPanel,
    toast,
  });
  const openFileInMarkdownPreview = useMarkdownPreviewAction({
    activeSessionIdRef,
    activeFileRequestRef,
    setFileState,
    updateFileState,
    removeFileState,
    addFileEditorPanel,
    toast,
  });

  const handleFileChange = useCallback(
    (path: string, newContent: string, repo?: string) =>
      applyFileChange(path, repo, newContent, updateFileState, promotePreviewToPinned),
    [updateFileState, promotePreviewToPinned],
  );

  const { saveFile, deleteFileAction, applyRemoteUpdate } = useSaveDeleteActions({
    activeSessionIdRef,
    updateFileState,
    setSavingFiles,
    toast,
  });

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
    removeFileState,
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
