"use client";

import { useRef, useEffect, useCallback } from "react";
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

export type FileTabRestorationOptions = {
  activeSessionId: string | null;
  leftTab: string;
  setLeftTab: (tab: string) => void;
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
};

export type FileSaveDeleteOptions = {
  activeSessionId: string | null;
  openFileTabs: OpenFileTab[];
  setOpenFileTabs: React.Dispatch<React.SetStateAction<OpenFileTab[]>>;
  setSavingFiles: React.Dispatch<React.SetStateAction<Set<string>>>;
  handleCloseFileTab: (path: string) => void;
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
      const response = await requestFileContent(client, sessionId, savedTab.path);
      const hash = await calculateHash(response.content);
      loadedTabs.push({
        path: savedTab.path,
        name: savedTab.name,
        content: response.content,
        originalContent: response.content,
        originalHash: hash,
        isDirty: false,
        isBinary: response.is_binary,
      });
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
      const filePath = savedActiveTab.replace("file:", "");
      if (loadedTabs.some((t) => t.path === filePath)) {
        setTimeout(() => {
          setLeftTab(savedActiveTab);
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
      setOpenFileTabs([]);
    } else if (restorationInProgressRef.current || restoredTabsRef.current === activeSessionId) {
      return;
    }
    const savedTabs = getOpenFileTabs(activeSessionId);
    const savedActiveTab = getActiveTabForSession(activeSessionId, "chat");
    if (savedTabs.length === 0) {
      setLeftTab(toPrimaryTab(savedActiveTab));
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
      if (restoredTabsRef.current !== activeSessionId) {
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

export function useFileSaveDelete({
  activeSessionId,
  openFileTabs,
  setOpenFileTabs,
  setSavingFiles,
  handleCloseFileTab,
}: FileSaveDeleteOptions) {
  const { toast } = useToast();

  const handleFileSave = useCallback(
    async (path: string) => {
      const tab = openFileTabs.find((t) => t.path === path);
      if (!tab || !tab.isDirty) return;
      const client = getWebSocketClient();
      if (!client || !activeSessionId) return;
      setSavingFiles((prev) => new Set(prev).add(path));
      try {
        const diff = generateUnifiedDiff(tab.originalContent, tab.content, tab.path);
        const response = await updateFileContent(client, activeSessionId, path, diff, tab.originalHash);
        if (response.success && response.new_hash) {
          setOpenFileTabs((prev) =>
            prev.map((t) =>
              t.path === path
                ? { ...t, originalContent: t.content, originalHash: response.new_hash!, isDirty: false }
                : t,
            ),
          );
        } else {
          toast({ title: "Save failed", description: response.error || "Failed to save file", variant: "error" });
        }
      } catch (error) {
        toast({
          title: "Save failed",
          description: error instanceof Error ? error.message : "An error occurred while saving the file",
          variant: "error",
        });
      } finally {
        setSavingFiles((prev) => {
          const next = new Set(prev);
          next.delete(path);
          return next;
        });
      }
    },
    [openFileTabs, activeSessionId, toast, setOpenFileTabs, setSavingFiles],
  );

  const handleFileDelete = useCallback(
    async (path: string) => {
      const client = getWebSocketClient();
      if (!client || !activeSessionId) return;
      try {
        const response = await deleteFile(client, activeSessionId, path);
        if (response.success) {
          handleCloseFileTab(path);
        } else {
          toast({ title: "Delete failed", description: response.error || "Failed to delete file", variant: "error" });
        }
      } catch (error) {
        toast({
          title: "Delete failed",
          description: error instanceof Error ? error.message : "An error occurred while deleting the file",
          variant: "error",
        });
      }
    },
    [activeSessionId, handleCloseFileTab, toast],
  );

  return { handleFileSave, handleFileDelete };
}
