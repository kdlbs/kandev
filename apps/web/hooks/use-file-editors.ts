"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  isFileRenameRetargeting,
  isRenameReserved,
  waitForRenameRelease,
} from "./file-rename-continuity";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import {
  getOpenFileTabs,
  setOpenFileTabs as saveOpenFileTabs,
  getActiveTabForSession,
  setActiveTabForSession,
} from "@/lib/local-storage";
import { calculateHash } from "@/lib/utils/file-diff";
import { getFilePreviewKind } from "@/lib/utils/file-types";
import { normalizeWorkspaceFilePath } from "@/lib/workspace-file-path";
import { useToast } from "@/components/toast-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useOpenFileAction, useMarkdownPreviewAction } from "./file-editor-open-actions";
import { useSaveDeleteActions, type PendingFileSave } from "./use-file-save-delete";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";
import { useOpenFileWorkspaceSync } from "./file-editors-sync";
import {
  isFileEditorPanelAlreadyRestored,
  isRestoreWriteCurrent,
  type FileEditorRequestToken,
} from "./file-editor-state";
import { scrollEditorIfMounted, setPendingCursorPosition } from "./file-editor-cursor";
export {
  consumePendingCursorPosition,
  scrollEditorIfMounted,
  setPendingCursorPosition,
} from "./file-editor-cursor";

// Module-level guard: ensures restoration only runs once across all hook instances
let _restoredSessionId: string | null = null;
let _restorationInProgress = false;

export function useOpenFileAtLine(
  onOpenFile: ((path: string) => void) | undefined,
  startLine: number | undefined,
  worktreePath: string | null | undefined,
  sessionId?: string,
) {
  return useCallback(
    (path: string) => {
      const openPath = normalizeWorkspaceFilePath(path, worktreePath);
      if (startLine && startLine > 0) {
        setPendingCursorPosition(openPath, startLine, 1, undefined, sessionId);
        onOpenFile?.(openPath);
        scrollEditorIfMounted(openPath, worktreePath ?? null, startLine, 1, { sessionId });
        return;
      }
      onOpenFile?.(openPath);
    },
    [onOpenFile, startLine, worktreePath, sessionId],
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
  return Array.from(openFiles.values()).flatMap(({ path, name, repo, renderedPreview }) => {
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
        ...(getFilePreviewKind(path) === "markdown" && renderedPreview ? { renderedPreview } : {}),
        pinned: persistAsPinned,
      },
    ];
  });
}

type RestoreTabsParams = {
  store: ReturnType<typeof useAppStoreApi>;
  activeSessionId: string;
  activeSessionIdRef: React.MutableRefObject<string | null>;
  activeEditorVisitRef: React.MutableRefObject<symbol | null>;
  visit: symbol | null;
  savedTabs: Array<{
    path: string;
    name: string;
    repo?: string;
    renderedPreview?: boolean;
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

async function loadRestoredTab(
  savedTab: RestoreTabsParams["savedTabs"][number],
  params: RestoreTabsParams,
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
): Promise<boolean> {
  const { activeSessionId, activeSessionIdRef, setFileState } = params;
  try {
    const itemId = buildRepoScopedItemId(savedTab.path, savedTab.repo);
    const response = await requestFileContent(
      client,
      activeSessionId,
      savedTab.path,
      savedTab.repo,
    );
    const hash = await calculateHash(response.content);
    if (!isRestoreWriteCurrent(_restoredSessionId, activeSessionId, activeSessionIdRef))
      return false;
    setFileState(itemId, {
      path: savedTab.path,
      repo: savedTab.repo,
      name: savedTab.name,
      content: response.content,
      originalContent: response.content,
      originalHash: hash,
      isDirty: false,
      isBinary: response.is_binary,
      resolvedPath: response.resolved_path,
      renderedPreview:
        getFilePreviewKind(savedTab.path, response.is_binary) === "markdown"
          ? savedTab.renderedPreview
          : undefined,
    });
  } catch {
    /* useFileLoader will retry when executor is ready */
  }
  return true;
}

function finishRestoredTabs(
  params: RestoreTabsParams,
  dockApi: ReturnType<typeof useDockviewStore.getState>["api"],
) {
  const { activeSessionId, activeSessionIdRef, activeEditorVisitRef, visit, savedActiveTab } =
    params;
  _restorationInProgress = false;
  if (
    !visit ||
    activeEditorVisitRef.current !== visit ||
    !isRestoreWriteCurrent(_restoredSessionId, activeSessionId, activeSessionIdRef) ||
    useDockviewStore.getState().api !== dockApi
  )
    return;
  const targetPanel = dockApi?.getPanel(savedActiveTab);
  if (targetPanel) targetPanel.api.setActive();
  const current = useDockviewStore.getState();
  if (current.api === dockApi && !current.isRestoringLayout) {
    saveOpenFileTabs(activeSessionId, buildPersistedTabs(current.api, current.openFiles));
  }
}

async function loadAndRestoreTabs(params: RestoreTabsParams, retryCount = 0): Promise<void> {
  const {
    activeSessionId,
    activeSessionIdRef,
    savedTabs,
    store,
    setFileState,
    addFileEditorPanel,
  } = params;
  while (savedTabs.some((tab) => isRenameReserved(store, activeSessionId, tab.path, tab.repo))) {
    await waitForRenameRelease(store, activeSessionId, savedTabs);
  }
  if (!isRestoreWriteCurrent(_restoredSessionId, activeSessionId, activeSessionIdRef)) {
    if (_restoredSessionId === activeSessionId) _restorationInProgress = false;
    return;
  }
  const client = getWebSocketClient();
  if (!client) {
    if (retryCount < 5) {
      setTimeout(() => loadAndRestoreTabs(params, retryCount + 1), 200);
      return;
    }
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
    // `renderedPreview` flag. This makes `openFiles.has(path)` true the moment
    // FileEditorPanel mounts, which suppresses its own `useFileLoader` fetch.
    // Without this seed, useFileLoader races the per-tab fetch below: both call
    // setFileState (a wholesale replace), and useFileLoader's state has no
    // renderedPreview, so when it wins the race (common under CPU load) the
    // restored preview flag is clobbered and the tab reopens in code view.
    setFileState(itemId, {
      path: savedTab.path,
      repo: savedTab.repo,
      name: savedTab.name,
      content: "",
      originalContent: "",
      originalHash: "",
      isDirty: false,
      renderedPreview:
        getFilePreviewKind(savedTab.path) === "markdown" ? savedTab.renderedPreview : undefined,
    });
  }
  for (const savedTab of savedTabs) {
    if (!(await loadRestoredTab(savedTab, params, client))) {
      if (_restoredSessionId === activeSessionId) _restorationInProgress = false;
      return;
    }
  }
  finishRestoredTabs(params, dockApi);
}

type FileEditorEffectsParams = {
  activeSessionId: string | null;
  activeSessionIdRef: React.MutableRefObject<string | null>;
  activeEditorVisitRef: React.MutableRefObject<symbol | null>;
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
  activeEditorVisitRef,
  setFileState,
  addFileEditorPanel,
  clearFileStates,
  removeFileState,
  api,
}: FileEditorEffectsParams) {
  const store = useAppStoreApi();
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
      store,
      activeSessionId,
      activeSessionIdRef,
      activeEditorVisitRef,
      visit: activeEditorVisitRef.current,
      savedTabs,
      savedActiveTab,
      setFileState,
      addFileEditorPanel,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSessionId]);

  useEffect(() => {
    const unsub = useDockviewStore.subscribe((state, prevState) => {
      if (state.openFiles === prevState.openFiles || isFileRenameRetargeting()) return;
      const sessionId = activeSessionIdRef.current;
      if (!sessionId || _restorationInProgress || state.isRestoringLayout) return;
      saveOpenFileTabs(sessionId, buildPersistedTabs(state.api, state.openFiles));
    });
    return unsub;
  }, [activeSessionIdRef]);

  useEffect(() => {
    if (!api || !activeSessionId) return;
    const disposable = api.onDidActivePanelChange((event) => {
      if (_restorationInProgress || isFileRenameRetargeting()) return;
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

export type FileEditorActionsParams = {
  activeSessionIdRef: React.MutableRefObject<string | null>;
  activeEditorVisitRef: React.MutableRefObject<symbol | null>;
  setFileState: (path: string, state: FileEditorState) => void;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
  removeFileState: (path: string) => void;
  addFileEditorPanel: (
    path: string,
    name: string,
    opts?: { quiet?: boolean; pin?: boolean; repo?: string },
  ) => void;
  promotePreviewToPinned: (type: "file-editor") => void;
  setSavingFiles: React.Dispatch<React.SetStateAction<Map<string, PendingFileSave>>>;
  toast: ReturnType<typeof useToast>["toast"];
};

function useFileEditorActions({
  activeSessionIdRef,
  activeEditorVisitRef,
  setFileState,
  updateFileState,
  removeFileState,
  addFileEditorPanel,
  promotePreviewToPinned,
  setSavingFiles,
  toast,
}: FileEditorActionsParams) {
  const activeFileRequestRef = useRef<FileEditorRequestToken | null>(null);
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
    activeEditorVisitRef,
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

export function useFileEditors(detailActive = true) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId, { detailActive });
  const { toast } = useToast();
  const [pendingSaves, setSavingFiles] = useState<Map<string, PendingFileSave>>(new Map());

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
  const activeEditorVisitRef = useRef<symbol | null>(null);
  useLayoutEffect(() => {
    const visit = Symbol();
    activeSessionIdRef.current = activeSessionId;
    activeEditorVisitRef.current = visit;
    setSavingFiles(new Map());
    return () => {
      activeEditorVisitRef.current = null;
    };
  }, [activeSessionId]);

  useFileEditorEffects({
    activeSessionId,
    activeSessionIdRef,
    activeEditorVisitRef,
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
    activeEditorVisitRef,
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
    activeEditorVisitRef,
    setFileState,
    updateFileState,
    removeFileState,
    addFileEditorPanel,
    promotePreviewToPinned,
    setSavingFiles,
    toast,
  });

  return {
    savingFiles: new Set(
      Array.from(pendingSaves)
        .filter(
          ([key, marker]) =>
            marker.sessionId === activeSessionId &&
            marker.api === api &&
            marker.instanceId === openFiles.get(key)?.instanceId,
        )
        .map(([key]) => key),
    ),
    openFile,
    openFileInMarkdownPreview,
    saveFile,
    deleteFile: deleteFileAction,
    handleFileChange,
    applyRemoteUpdate,
  };
}
