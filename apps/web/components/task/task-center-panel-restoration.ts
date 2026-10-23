"use client";

import { useRef, useEffect, useLayoutEffect, useState, useCallback, useContext } from "react";
import { FileDraftSessionContext } from "./file-draft-session-context";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { useSaveDeleteActions, type PendingFileSave } from "@/hooks/use-file-save-delete";
import type { OpenFileTab } from "@/lib/types/backend";
import { getWebSocketClient } from "@/lib/ws/connection";
import {
  getOpenFileTabs,
  getActiveTabForSession,
  setActiveTabForSession,
  type StoredFileTab,
} from "@/lib/local-storage";
import { calculateHash, generateUnifiedDiff } from "@/lib/utils/file-diff";
import { requestFileContent, updateFileContent, deleteFile } from "@/lib/ws/workspace-files";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import {
  getFileTabKey,
  installFileEditorTab,
  type FileEditorTab,
} from "./task-center-panel-file-tabs";
import { lspClientManager } from "@/lib/lsp/lsp-client-manager";
import { getFilePreviewKind, isMarkdownFile } from "@/lib/utils/file-types";
import { resolveStoredMarkdownFileMode } from "./markdown-file-mode";

export type FileTabRestorationOptions = {
  activeSessionId: string | null;
  leftTab: string;
  setLeftTab: (tab: string) => void;
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
};

export type FileSaveDeleteOptions = {
  activeSessionId: string | null;
  openFileTabs: FileEditorTab[];
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
  handleCloseFileTab: (fileKey: string, instanceId?: symbol) => void;
};

export function toPrimaryTab(savedTab: string) {
  return savedTab === "chat" || savedTab === "changes" ? savedTab : "chat";
}

export async function loadSavedFileTabs(sessionId: string, savedTabs: StoredFileTab[]) {
  const client = getWebSocketClient();
  if (!client) return null;
  const loadedTabs: OpenFileTab[] = [];
  for (const savedTab of savedTabs) {
    try {
      const response = await requestFileContent(client, sessionId, savedTab.path, savedTab.repo);
      const hash = await calculateHash(response.content);
      loadedTabs.push(
        installFileEditorTab({
          path: savedTab.path,
          name: savedTab.name,
          content: response.content,
          originalContent: response.content,
          originalHash: hash,
          isDirty: false,
          isBinary: response.is_binary,
          resolvedPath: response.resolved_path,
          repo: savedTab.repo,
          renderedPreview:
            getFilePreviewKind(savedTab.path, response.is_binary) === "markdown"
              ? savedTab.renderedPreview
              : undefined,
          ...(isMarkdownFile(savedTab.path)
            ? { markdownMode: resolveStoredMarkdownFileMode(savedTab) }
            : {}),
        }),
      );
    } catch {
      /* skip failed tabs */
    }
  }
  return loadedTabs;
}

type RestoreLoadedTabsParams = {
  loadedTabs: OpenFileTab[];
  savedActiveTab: string;
  restoredTabsRef: React.MutableRefObject<string | null>;
  restorationInProgressRef: React.MutableRefObject<boolean>;
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
  setLeftTab: (tab: string) => void;
};

function restoreLoadedTabs({
  loadedTabs,
  savedActiveTab,
  restorationInProgressRef,
  setOpenFileTabs,
  setLeftTab,
}: RestoreLoadedTabsParams) {
  if (loadedTabs.length > 0) {
    setOpenFileTabs(loadedTabs);
    if (savedActiveTab.startsWith("file:")) {
      const savedFileKey = savedActiveTab.slice("file:".length);
      // Existing sessions may have stored the old path-only tab value. Prefer
      // the repo-aware key, while keeping that persisted state restorable.
      const activeTab = loadedTabs.find(
        (tab) => getFileTabKey(tab) === savedFileKey || tab.path === savedFileKey,
      );
      if (activeTab) {
        setTimeout(() => {
          setLeftTab(`file:${getFileTabKey(activeTab)}`);
          restorationInProgressRef.current = false;
        }, 0);
      } else {
        setLeftTab("chat");
        restorationInProgressRef.current = false;
      }
    } else {
      setLeftTab(savedActiveTab);
      restorationInProgressRef.current = false;
    }
  } else {
    setLeftTab(toPrimaryTab(savedActiveTab));
    restorationInProgressRef.current = false;
  }
}

export function useFileTabRestoration({
  activeSessionId,
  leftTab,
  setLeftTab,
  setOpenFileTabs,
}: FileTabRestorationOptions) {
  const retainedOwner = useContext(FileDraftSessionContext);
  const restoredTabsRef = useRef<string | null>(null);
  const restorationInProgressRef = useRef<boolean>(false);
  const prevSessionRef = useRef<string | null>(null);

  useEffect(() => {
    return () => {
      if (prevSessionRef.current && leftTab)
        setActiveTabForSession(prevSessionRef.current, leftTab);
    };
  }, [leftTab]);

  useEffect(() => {
    if (!activeSessionId) return;
    if (restoredTabsRef.current !== activeSessionId) {
      if (prevSessionRef.current && prevSessionRef.current !== activeSessionId)
        setActiveTabForSession(prevSessionRef.current, leftTab);
      restoredTabsRef.current = activeSessionId;
      prevSessionRef.current = activeSessionId;
      restorationInProgressRef.current = false;
      if (!retainedOwner) setOpenFileTabs([]);
    } else if (restorationInProgressRef.current || restoredTabsRef.current === activeSessionId) {
      return;
    }
    const store = useDockviewStore.getState();
    const epoch = store.fileSessionEpoch;
    const selectedKey = store.mobileFileKeys.get(activeSessionId);
    const selected = selectedKey ? store.openFiles.get(selectedKey) : undefined;
    const savedTabs = getOpenFileTabs(activeSessionId).filter(
      (tab) => !retainedOwner || !store.openFiles.has(buildRepoScopedItemId(tab.path, tab.repo)),
    );
    const savedActiveTab =
      retainedOwner && selected
        ? `file:${getFileTabKey(selected)}`
        : getActiveTabForSession(activeSessionId, "chat");
    if (savedTabs.length === 0) {
      setLeftTab(retainedOwner && selected ? savedActiveTab : toPrimaryTab(savedActiveTab));
      return;
    }
    restorationInProgressRef.current = true;
    const loadTabs = async (retryCount = 0): Promise<void> => {
      const loadedTabs = await loadSavedFileTabs(activeSessionId, savedTabs);
      if (!loadedTabs) {
        if (retryCount < 5) {
          setTimeout(() => loadTabs(retryCount + 1), 200);
          return;
        }
        restorationInProgressRef.current = false;
        return;
      }
      if (
        restoredTabsRef.current !== activeSessionId ||
        (retainedOwner && useDockviewStore.getState().fileSessionEpoch !== epoch)
      ) {
        restorationInProgressRef.current = false;
        return;
      }
      restoreLoadedTabs({
        loadedTabs,
        savedActiveTab,
        restoredTabsRef,
        restorationInProgressRef,
        setOpenFileTabs,
        setLeftTab,
      });
    };
    void loadTabs();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- leftTab is intentionally excluded to prevent re-running on tab changes
  }, [activeSessionId]);

  useEffect(() => {
    if (!activeSessionId || restorationInProgressRef.current) return;
    setActiveTabForSession(activeSessionId, leftTab);
  }, [activeSessionId, leftTab]);

  return { restorationInProgressRef };
}

function updateTabsAfterSave(
  tabs: FileEditorTab[],
  fileKey: string,
  persistedContent: string,
  originalHash: string,
): OpenFileTab[] {
  return tabs.map((tab) =>
    getFileTabKey(tab) === fileKey
      ? {
          ...tab,
          originalContent: persistedContent,
          originalHash,
          isDirty: tab.content !== persistedContent,
        }
      : tab,
  );
}

type MutationOwner = { session: string; visit: symbol; fileKey: string; instanceId: symbol };

function useFileMutationOwner(activeSessionId: string | null, tabs: FileEditorTab[]) {
  const visitRef = useRef<{ session: string; id: symbol } | null>(null);
  const tabsRef = useRef(tabs);
  tabsRef.current = tabs;
  const [pendingSaves, setPendingSaves] = useState(new Map<string, MutationOwner>());
  useLayoutEffect(() => {
    visitRef.current = activeSessionId ? { session: activeSessionId, id: Symbol() } : null;
    setPendingSaves(new Map());
    return () => {
      visitRef.current = null;
    };
  }, [activeSessionId]);
  const capture = (fileKey: string): MutationOwner | null => {
    const visit = visitRef.current;
    const tab = tabsRef.current.find((item) => getFileTabKey(item) === fileKey);
    if (!visit || !tab?.instanceId) return null;
    return { session: visit.session, visit: visit.id, fileKey, instanceId: tab.instanceId };
  };
  const isCurrent = (owner: MutationOwner, currentTabs = tabsRef.current) =>
    visitRef.current?.id === owner.visit &&
    visitRef.current.session === owner.session &&
    currentTabs.some(
      (tab) => getFileTabKey(tab) === owner.fileKey && tab.instanceId === owner.instanceId,
    );
  const savingFiles = new Set(
    [...pendingSaves]
      .filter(([, owner]) => owner.session === activeSessionId && isCurrent(owner))
      .map(([key]) => key),
  );
  return { capture, isCurrent, tabsRef, savingFiles, setPendingSaves };
}

function useRetainedFileSave(activeSessionId: string | null) {
  const retainedOwner = useContext(FileDraftSessionContext);
  const { toast } = useToast();
  const [pending, setPending] = useState(new Map<string, PendingFileSave>());
  const sessionIdRef = useRef(activeSessionId);
  sessionIdRef.current = activeSessionId;
  const visitRef = useRef<symbol | null>(null);
  const files = useDockviewStore((state) => state.openFiles);
  const actions = useSaveDeleteActions({
    retainedOwner,
    activeSessionIdRef: retainedOwner?.sessionIdRef ?? sessionIdRef,
    activeEditorVisitRef: retainedOwner?.visitRef ?? visitRef,
    updateFileState: useDockviewStore.getState().updateFileState,
    setSavingFiles: setPending,
    toast,
  });
  const savingFiles = new Set(
    Array.from(pending).flatMap(([key, owner]) => {
      const file = files.get(key);
      return file?.instanceId === owner.instanceId && owner.sessionId === activeSessionId
        ? [getFileTabKey(file)]
        : [];
    }),
  );
  return retainedOwner ? { handleFileSave: actions.saveFile, savingFiles } : null;
}

function clearPendingSave(
  ownership: ReturnType<typeof useFileMutationOwner>,
  fileKey: string,
  owner: MutationOwner,
) {
  ownership.setPendingSaves((previous) => {
    if (previous.get(fileKey) !== owner) return previous;
    const next = new Map(previous);
    next.delete(fileKey);
    return next;
  });
}

export function useFileSaveDelete({
  activeSessionId,
  openFileTabs,
  setOpenFileTabs,
  handleCloseFileTab,
}: FileSaveDeleteOptions) {
  const { toast } = useToast();
  const ownership = useFileMutationOwner(activeSessionId, openFileTabs);
  const retainedSave = useRetainedFileSave(activeSessionId);
  const closeRef = useRef(handleCloseFileTab);
  closeRef.current = handleCloseFileTab;

  const handleFileSave = useCallback(
    async (path: string, repo?: string) => {
      const fileKey = getFileTabKey({ path, repo });
      const tab = ownership.tabsRef.current.find((item) => getFileTabKey(item) === fileKey);
      if (!tab || !tab.isDirty) return;
      const client = getWebSocketClient();
      const owner = ownership.capture(fileKey);
      if (!client || !owner) return;
      ownership.setPendingSaves((prev) => new Map(prev).set(fileKey, owner));
      try {
        const diff = generateUnifiedDiff(tab.originalContent, tab.content, tab.path);
        const response = await updateFileContent(client, owner.session, {
          path,
          diff,
          originalHash: tab.originalHash,
          desiredContent: tab.content,
          repo: tab.repo,
        });
        if (!ownership.isCurrent(owner)) return;
        if (response.success && response.new_hash) {
          const current = ownership.tabsRef.current.find((item) => getFileTabKey(item) === fileKey);
          lspClientManager.saveDocument(
            owner.session,
            path,
            tab.repo,
            tab.content,
            current?.content ?? tab.content,
          );
          setOpenFileTabs((prev) =>
            ownership.isCurrent(owner, prev)
              ? updateTabsAfterSave(prev, fileKey, tab.content, response.new_hash!)
              : prev,
          );
        } else {
          toast({
            title: t("editors:saveFailed"),
            description: response.error || t("editors:failedToSaveFile"),
            variant: "error",
          });
        }
      } catch (error) {
        if (!ownership.isCurrent(owner)) return;
        toast({
          title: t("editors:saveFailed"),
          description: error instanceof Error ? error.message : t("editors:errorWhileSavingFile"),
          variant: "error",
        });
      } finally {
        clearPendingSave(ownership, fileKey, owner);
      }
    },
    [ownership, toast, setOpenFileTabs],
  );

  const handleFileDelete = useCallback(
    async (path: string, repo?: string) => {
      const client = getWebSocketClient();
      const owner = ownership.capture(getFileTabKey({ path, repo }));
      if (!client || !owner) return;
      try {
        const response = await deleteFile(client, owner.session, path, repo);
        if (!ownership.isCurrent(owner)) return;
        if (response.success) {
          closeRef.current(owner.fileKey, owner.instanceId);
        } else {
          toast({
            title: t("editors:deleteFailed"),
            description: response.error || t("editors:failedToDeleteFile"),
            variant: "error",
          });
        }
      } catch (error) {
        if (!ownership.isCurrent(owner)) return;
        toast({
          title: t("editors:deleteFailed"),
          description: error instanceof Error ? error.message : t("editors:errorWhileDeletingFile"),
          variant: "error",
        });
      }
    },
    [ownership, toast],
  );

  return {
    handleFileSave: retainedSave?.handleFileSave ?? handleFileSave,
    handleFileDelete,
    savingFiles: retainedSave?.savingFiles ?? ownership.savingFiles,
  };
}
