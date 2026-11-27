"use client";

import {
  shouldOpenFilePreview,
  previewFileState,
  canFindViewerForFileType,
} from "@/lib/plugins/file-viewer-selection";

import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import {
  isRenameReserved,
  trackRenameOpen,
  waitForRenameRelease,
} from "@/hooks/file-rename-continuity";
import { useRef, useEffect, useLayoutEffect, useState, useCallback, useMemo } from "react";
import { treePath } from "@/hooks/file-rename-continuity";
import {
  tabletSelectionOwner,
  subscribeTabletFileDeletion,
} from "@/lib/state/tablet-file-authority";
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
  mergeRestoredFileTabs,
  type FileEditorTab,
} from "./task-center-panel-file-tabs";
import { lspClientManager } from "@/lib/lsp/lsp-client-manager";
import { getFilePreviewKind } from "@/lib/utils/file-types";
import { matchesTabletFileDeletion, retireDeletedTabletFile } from "./tablet-file-deletion";

export type FileTabRestorationOptions = {
  store?: StoreApi<AppState>;
  activeSessionId: string | null;
  leftTab: string;
  setLeftTab: (tab: string) => void;
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
  openFileTabs?: FileEditorTab[];
  currentTabs: React.RefObject<OpenFileTab[]>;
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
  const loadedTabs: Array<OpenFileTab | undefined> = new Array(savedTabs.length);
  let nextIndex = 0;
  const worker = async () => {
    while (nextIndex < savedTabs.length) {
      const index = nextIndex++;
      const savedTab = savedTabs[index];
      try {
        if (shouldOpenFilePreview(savedTab.path)) {
          loadedTabs[index] = installFileEditorTab(previewFileState(savedTab.path, savedTab.repo));
          continue;
        }
        const response = await requestFileContent(client, sessionId, savedTab.path, savedTab.repo);
        const hash = await calculateHash(response.content);
        loadedTabs[index] = installFileEditorTab({
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
        });
      } catch {
        if (canFindViewerForFileType(savedTab.path)) {
          loadedTabs[index] = installFileEditorTab(previewFileState(savedTab.path, savedTab.repo));
        }
      }
    }
  };
  await Promise.all(Array.from({ length: Math.min(4, savedTabs.length) }, worker));
  return loadedTabs.filter((tab): tab is OpenFileTab => tab !== undefined);
}

type RestoreLoadedTabsParams = {
  loadedTabs: OpenFileTab[];
  savedActiveTab: string;
  isCurrent: () => boolean;
  onRestored: () => void;
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
  setLeftTab: (tab: string) => void;
  store?: StoreApi<AppState>;
  sessionId: string;
  visit: RestorationVisit;
};

function restoreLoadedTabs({
  loadedTabs,
  savedActiveTab,
  isCurrent,
  onRestored,
  setOpenFileTabs,
  setLeftTab,
  store,
  sessionId,
  visit,
}: RestoreLoadedTabsParams) {
  if (!isCurrent()) return;
  loadedTabs = loadedTabs.filter((file) => !isRetiredFile(visit, file));
  if (loadedTabs.length > 0) setOpenFileTabs((live) => mergeRestoredFileTabs(live, loadedTabs));
  const select = () => {
    if (!isCurrent()) return;
    const current = store?.getState().tabletSession.activeTabBySessionId[sessionId];
    const newer =
      current !== undefined &&
      (current !== visit.initial ||
        (store && tabletSelectionOwner(store, sessionId) !== visit.owner));
    if (newer) {
      onRestored();
      return;
    }
    let selected =
      current ?? (loadedTabs.length > 0 ? savedActiveTab : toPrimaryTab(savedActiveTab));
    if (selected.startsWith("file:")) {
      const key = selected.slice("file:".length);
      // Old path-only preferences resolve to the current repo-aware file key.
      const file = [...visit.live, ...loadedTabs].find(
        (tab) => getFileTabKey(tab) === key || tab.path === key,
      );
      if (file) selected = `file:${getFileTabKey(file)}`;
      else if (!newer) selected = "chat";
    }
    setLeftTab(selected);
    onRestored();
  };
  if (loadedTabs.length > 0 && savedActiveTab.startsWith("file:")) setTimeout(select, 0);
  else select();
}

type RestorationVisit = {
  initial: string;
  owner: symbol | undefined;
  live: FileEditorTab[];
  closed: Set<string>;
  deleted: Set<string>;
  retire: (file: OpenFileTab) => void;
};

function isRetiredFile(visit: RestorationVisit, file: OpenFileTab): boolean {
  const path = treePath(file);
  if (visit.closed.has(getFileTabKey(file)) || visit.closed.has(`path:${path}`)) return true;
  for (const deleted of visit.deleted) if (matchesTabletFileDeletion(file, deleted)) return true;
  return false;
}
function useRestorationVisit(
  store: FileTabRestorationOptions["store"],
  activeSessionId: string | null,
  openFileTabs: FileEditorTab[] | undefined,
) {
  const visit = useMemo<RestorationVisit>(() => {
    const closed = new Set<string>();
    return {
      initial: activeSessionId
        ? (store?.getState().tabletSession.activeTabBySessionId[activeSessionId] ??
          getActiveTabForSession(activeSessionId, "chat"))
        : "chat",
      owner: store && activeSessionId ? tabletSelectionOwner(store, activeSessionId) : undefined,
      live: [],
      closed,
      deleted: new Set(),
      retire(file) {
        closed.add(getFileTabKey(file));
        const path = treePath(file);
        if (path) closed.add(`path:${path}`);
      },
    };
  }, [store, activeSessionId]);
  useLayoutEffect(() => {
    if (!openFileTabs) return;
    visit.live = openFileTabs;
  }, [openFileTabs, visit]);
  return visit;
}
function useLegacyTabPreference(
  store: FileTabRestorationOptions["store"],
  activeSessionId: string | null,
  leftTab: string,
  restoring: React.MutableRefObject<boolean>,
) {
  useEffect(
    () => () => {
      if (
        activeSessionId &&
        store?.getState().tabletSession.activeTabBySessionId[activeSessionId] === undefined
      )
        setActiveTabForSession(activeSessionId, leftTab);
    },
    [activeSessionId, leftTab, store],
  );
  useEffect(() => {
    if (
      !activeSessionId ||
      restoring.current ||
      store?.getState().tabletSession.activeTabBySessionId[activeSessionId] !== undefined
    )
      return;
    setActiveTabForSession(activeSessionId, leftTab);
  }, [activeSessionId, leftTab, store, restoring]);
}

export function useFileTabRestoration({
  store,
  activeSessionId,
  leftTab,
  setLeftTab,
  setOpenFileTabs,
  openFileTabs,
  currentTabs,
}: FileTabRestorationOptions) {
  const restorationInProgressRef = useRef<boolean>(false);
  const [restoredSessionId, setRestoredSessionId] = useState<string | null>(null);
  const visit = useRestorationVisit(store, activeSessionId, openFileTabs);
  useLayoutEffect(() => {
    if (!store || !activeSessionId) return;
    return subscribeTabletFileDeletion(store, activeSessionId, (path) => {
      visit.deleted.add(path);
      setOpenFileTabs(retireDeletedTabletFile(store, activeSessionId, path, currentTabs.current));
    });
  }, [store, activeSessionId, visit, setOpenFileTabs, currentTabs]);

  useEffect(() => {
    if (!activeSessionId) return;
    setRestoredSessionId(null);
    const onRestored = () => {
      restorationInProgressRef.current = false;
      setRestoredSessionId(activeSessionId);
    };
    let current = true;
    const release = () => {
      current = false;
    };
    restorationInProgressRef.current = false;
    setOpenFileTabs([]);
    const savedTabs = getOpenFileTabs(activeSessionId);
    const savedActiveTab = getActiveTabForSession(activeSessionId, "chat");
    if (savedTabs.length === 0) {
      const currentTab = store?.getState().tabletSession.activeTabBySessionId[activeSessionId];
      if (
        currentTab === undefined ||
        (currentTab === visit.initial &&
          (!store || tabletSelectionOwner(store, activeSessionId) === visit.owner))
      )
        setLeftTab(
          currentTab?.startsWith("file:") ? "chat" : (currentTab ?? toPrimaryTab(savedActiveTab)),
        );
      onRestored();
      return release;
    }
    restorationInProgressRef.current = true;
    const loadTabs = async (retryCount = 0): Promise<void> => {
      if (!current) return;
      if (store) {
        while (
          current &&
          savedTabs.some((tab) => isRenameReserved(store, activeSessionId, tab.path, tab.repo))
        )
          await waitForRenameRelease(store, activeSessionId, savedTabs);
      }
      if (!current) return;
      const pending = store
        ? savedTabs.map((tab) => trackRenameOpen(store, activeSessionId, tab.path, tab.repo))
        : [];
      const loadedTabs = await loadSavedFileTabs(activeSessionId, savedTabs);
      const invalid = pending.some((entry) => !entry.current());
      pending.forEach((entry) => entry.dispose());
      if (!current) return;
      if (invalid) {
        void loadTabs(retryCount);
        return;
      }
      if (!loadedTabs) {
        if (retryCount < 5) setTimeout(() => loadTabs(retryCount + 1), 200);
        else onRestored();
        return;
      }
      restoreLoadedTabs({
        loadedTabs,
        savedActiveTab,
        isCurrent: () => current,
        onRestored,
        setOpenFileTabs,
        setLeftTab,
        store,
        sessionId: activeSessionId,
        visit,
      });
    };
    void loadTabs();
    return release;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- Tab changes do not start another restoration visit.
  }, [activeSessionId]);
  useLegacyTabPreference(store, activeSessionId, leftTab, restorationInProgressRef);

  return { restorationInProgressRef, restoredSessionId, retireFile: visit.retire };
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

export function useFileSaveDelete({
  activeSessionId,
  openFileTabs,
  setOpenFileTabs,
  handleCloseFileTab,
}: FileSaveDeleteOptions) {
  const { toast } = useToast();
  const ownership = useFileMutationOwner(activeSessionId, openFileTabs);
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
        ownership.setPendingSaves((prev) => {
          if (prev.get(fileKey) !== owner) return prev;
          const next = new Map(prev);
          next.delete(fileKey);
          return next;
        });
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

  return { handleFileSave, handleFileDelete, savingFiles: ownership.savingFiles };
}
