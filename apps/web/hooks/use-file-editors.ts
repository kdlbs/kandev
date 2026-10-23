"use client";

import { useCallback, useContext, useEffect, useLayoutEffect, useRef, useState } from "react";
import { FileDraftSessionContext } from "@/components/task/file-draft-session-context";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import {
  getOpenFileTabs,
  setOpenFileTabs as saveOpenFileTabs,
  getActiveTabForSession,
  setActiveTabForSession,
  type StoredFileTab,
} from "@/lib/local-storage";
import { calculateHash } from "@/lib/utils/file-diff";
import { getFilePreviewKind } from "@/lib/utils/file-types";
import { normalizeWorkspaceFilePath } from "@/lib/workspace-file-path";
import { useToast } from "@/components/toast-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSaveDeleteActions, type PendingFileSave } from "./use-file-save-delete";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";
import { useOpenFileWorkspaceSync } from "./file-editors-sync";
import { t } from "@/lib/i18n";
import {
  fetchFileEditorState,
  buildPersistedTabs,
  getPreviewItemIdToRemoveOnReplace,
  isFileEditorPanelAlreadyRestored,
  isRestoreWriteCurrent,
  type FileEditorRequestToken,
} from "./file-editor-state";
import { scrollEditorIfMounted, setPendingCursorPosition } from "./file-editor-cursor";
import {
  defaultMarkdownFileMode,
  resolveStoredMarkdownFileMode,
} from "@/components/task/markdown-file-mode";
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
  useDockviewStore.getState().selectFileDraft(fileKey);
}

type RestoreTabsParams = {
  activeSessionId: string;
  activeSessionIdRef: React.MutableRefObject<string | null>;
  savedTabs: StoredFileTab[];
  savedActiveTab: string;
  addFileEditorPanel: (
    path: string,
    name: string,
    opts?: { quiet?: boolean; pin?: boolean; repo?: string },
  ) => void;
};

async function restoreFileTabContent(
  savedTab: RestoreTabsParams["savedTabs"][number],
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
  { activeSessionId, activeSessionIdRef }: RestoreTabsParams,
): Promise<boolean> {
  try {
    const itemId = buildRepoScopedItemId(savedTab.path, savedTab.repo);
    const before = useDockviewStore.getState();
    const placeholder = before.openFiles.get(itemId);
    if (!placeholder || placeholder.originalHash || placeholder.isDirty) return true;
    const epoch = before.fileSessionEpoch;
    const response = await requestFileContent(
      client,
      activeSessionId,
      savedTab.path,
      savedTab.repo,
    );
    const hash = await calculateHash(response.content);
    if (!isRestoreWriteCurrent(_restoredSessionId, activeSessionId, activeSessionIdRef)) {
      if (_restoredSessionId === activeSessionId) _restorationInProgress = false;
      return false;
    }
    const current = useDockviewStore.getState();
    const live = current.openFiles.get(itemId);
    if (
      current.fileSessionEpoch !== epoch ||
      !live ||
      live.instanceId !== placeholder.instanceId ||
      live.isDirty ||
      live.originalHash
    )
      return true;
    current.updateFileState(itemId, {
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
      markdownMode: live.markdownMode ?? resolveStoredMarkdownFileMode(savedTab),
    });
  } catch {
    /* useFileLoader will retry when executor is ready */
  }
  return true;
}

async function loadAndRestoreTabs(params: RestoreTabsParams, retryCount = 0): Promise<void> {
  const { activeSessionId, savedTabs, savedActiveTab, addFileEditorPanel } = params;
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
    // Seed the stored mode before panels mount, while retaining live draft records.
    useDockviewStore.getState().ensureFileState(itemId, {
      path: savedTab.path,
      repo: savedTab.repo,
      name: savedTab.name,
      content: "",
      originalContent: "",
      originalHash: "",
      isDirty: false,
      markdownMode: resolveStoredMarkdownFileMode(savedTab),
      renderedPreview:
        getFilePreviewKind(savedTab.path) === "markdown" ? savedTab.renderedPreview : undefined,
    });
  }
  for (const savedTab of savedTabs) {
    if (!(await restoreFileTabContent(savedTab, client, params))) return;
  }
  const targetPanel = dockApi?.getPanel(savedActiveTab);
  if (targetPanel) targetPanel.api.setActive();
  _restorationInProgress = false;
}

type FileEditorEffectsParams = {
  activeSessionId: string | null;
  activeSessionIdRef: React.MutableRefObject<string | null>;
  addFileEditorPanel: (
    path: string,
    name: string,
    opts?: { quiet?: boolean; pin?: boolean; repo?: string },
  ) => void;
  removeFileState: (path: string) => void;
  api: ReturnType<typeof useDockviewStore.getState>["api"];
};

function useFileEditorEffects({
  activeSessionId,
  activeSessionIdRef,
  addFileEditorPanel,
  removeFileState,
  api,
}: FileEditorEffectsParams) {
  useEffect(() => {
    if (!activeSessionId || _restoredSessionId === activeSessionId) return;
    _restoredSessionId = activeSessionId;
    // Projection activation must not persist a partial restoration.
    _restorationInProgress = true;
    useDockviewStore.getState().activateFileSession(activeSessionId);
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
      addFileEditorPanel,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSessionId]);

  useEffect(() => {
    const unsub = useDockviewStore.subscribe((state, prevState) => {
      if (state.openFiles === prevState.openFiles) return;
      const sessionId = activeSessionIdRef.current;
      if (
        !sessionId ||
        state.activeFileSessionId !== sessionId ||
        _restorationInProgress ||
        state.isRestoringLayout
      )
        return;
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
  retainedOwner?:
    | import("@/components/task/file-draft-session-context").FileDraftSessionOwner
    | null;
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
      const files = getOpenFiles();
      useDockviewStore.getState().selectFileDraft(fileKey);
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
      const requestToken = markActiveFileRequest(activeFileRequestRef, fileKey);
      const epoch = useDockviewStore.getState().fileSessionEpoch;
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
        if (!state || useDockviewStore.getState().fileSessionEpoch !== epoch) return;
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
        const markdownMode = defaultMarkdownFileMode(filePath);
        setFileState(fileKey, markdownMode ? { ...state, markdownMode } : state);
      } catch (error) {
        toast({
          title: t("task:failedToOpenFile"),
          description: error instanceof Error ? error.message : t("common:unknownError"),
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
  activeFileRequestRef: React.MutableRefObject<FileEditorRequestToken | null>;
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
      const files = getOpenFiles();
      useDockviewStore.getState().selectFileDraft(fileKey);
      if (files.has(fileKey)) {
        updateFileState(fileKey, { markdownMode: "preview" });
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
      const requestToken = markActiveFileRequest(activeFileRequestRef, fileKey);
      const epoch = useDockviewStore.getState().fileSessionEpoch;
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
        if (!state || useDockviewStore.getState().fileSessionEpoch !== epoch) return;
        addFileEditorPanelWithPreviewCleanup(
          filePath,
          state.name,
          repo,
          addFileEditorPanel,
          removeFileState,
        );
        setFileState(fileKey, { ...state, markdownMode: "preview" });
      } catch (error) {
        toast({
          title: t("task:failedToOpenFile"),
          description: error instanceof Error ? error.message : t("common:unknownError"),
          variant: "error",
        });
      }
    },
    [activeSessionIdRef, setFileState, updateFileState, addFileEditorPanel, removeFileState, toast],
  );
}

function useFileEditorActions({
  retainedOwner,
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
    retainedOwner,
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
  const retainedOwner = useContext(FileDraftSessionContext);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId, { detailActive });
  const { toast } = useToast();
  const [pendingSaves, setSavingFiles] = useState<Map<string, PendingFileSave>>(new Map());

  const setFileState = useDockviewStore((s) => s.setFileState);
  const updateFileState = useDockviewStore((s) => s.updateFileState);
  const removeFileState = useDockviewStore((s) => s.removeFileState);
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
    if (activeSessionId) {
      if (_restoredSessionId !== activeSessionId) _restorationInProgress = true;
      useDockviewStore.getState().activateFileSession(activeSessionId);
    }
    activeEditorVisitRef.current = visit;
    setSavingFiles(new Map());
    return () => {
      activeEditorVisitRef.current = null;
    };
  }, [activeSessionId]);

  useFileEditorEffects({
    activeSessionId,
    activeSessionIdRef,
    addFileEditorPanel,
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
    retainedOwner,
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
