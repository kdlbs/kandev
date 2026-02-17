'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useDockviewStore, type FileEditorState } from '@/lib/state/dockview-store';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileContent, updateFileContent, deleteFile } from '@/lib/ws/workspace-files';
import { getOpenFileTabs, setOpenFileTabs as saveOpenFileTabs, getActiveTabForSession, setActiveTabForSession } from '@/lib/local-storage';
import { generateUnifiedDiff, calculateHash } from '@/lib/utils/file-diff';
import { useToast } from '@/components/toast-provider';
import type { FileContentResponse } from '@/lib/types/backend';

// Module-level guard: ensures restoration only runs once across all hook instances
let _restoredSessionId: string | null = null;
let _restorationInProgress = false;

// Pending cursor positions: set before opening a file, consumed by the editor on mount.
// Used by LSP Go-to-Definition to jump to the correct line/column.
const _pendingCursorPositions = new Map<string, { line: number; column: number }>();

export function setPendingCursorPosition(path: string, line: number, column: number) {
  _pendingCursorPositions.set(path, { line, column });
}

export function consumePendingCursorPosition(path: string): { line: number; column: number } | undefined {
  const pos = _pendingCursorPositions.get(path);
  if (pos) _pendingCursorPositions.delete(path);
  return pos;
}

/** Read openFiles from the store without subscribing to changes. */
function getOpenFiles() {
  return useDockviewStore.getState().openFiles;
}

export function useFileEditors() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { toast } = useToast();
  const [savingFiles, setSavingFiles] = useState<Set<string>>(new Set());

  // No reactive subscription to openFiles — callbacks use getState() instead.
  // Persistence is handled by a zustand store subscriber (see below).
  const setFileState = useDockviewStore((s) => s.setFileState);
  const updateFileState = useDockviewStore((s) => s.updateFileState);
  const removeFileState = useDockviewStore((s) => s.removeFileState);
  const clearFileStates = useDockviewStore((s) => s.clearFileStates);
  const addFileEditorPanel = useDockviewStore((s) => s.addFileEditorPanel);
  const api = useDockviewStore((s) => s.api);

  // Use refs for callbacks that don't need to cause re-renders of consumers.
  const activeSessionIdRef = useRef(activeSessionId);
  activeSessionIdRef.current = activeSessionId;

  // Restore file tabs from sessionStorage on session change.
  // The module-level guard prevents multiple hook instances from running this.
  useEffect(() => {
    if (!activeSessionId) return;
    if (_restoredSessionId === activeSessionId) return;

    _restoredSessionId = activeSessionId;
    _restorationInProgress = false;

    // Clear zustand file content state (panels are handled by fromJSON layout restore)
    clearFileStates();

    const savedTabs = getOpenFileTabs(activeSessionId);
    const savedActiveTab = getActiveTabForSession(activeSessionId, 'chat');
    if (savedTabs.length === 0) return;

    _restorationInProgress = true;

    const loadTabs = async (retryCount = 0): Promise<void> => {
      const maxRetries = 5;
      const retryDelay = 200;

      const client = getWebSocketClient();
      if (!client) {
        if (retryCount < maxRetries) {
          setTimeout(() => loadTabs(retryCount + 1), retryDelay);
          return;
        }
        _restorationInProgress = false;
        return;
      }

      if (_restoredSessionId !== activeSessionId) {
        _restorationInProgress = false;
        return;
      }

      for (const savedTab of savedTabs) {
        try {
          const response = await requestFileContent(client, activeSessionId, savedTab.path);
          const hash = await calculateHash(response.content);
          const state: FileEditorState = {
            path: savedTab.path,
            name: savedTab.name,
            content: response.content,
            originalContent: response.content,
            originalHash: hash,
            isDirty: false,
            isBinary: response.is_binary,
          };
          setFileState(savedTab.path, state);
          // quiet=true: don't steal focus during bulk restoration
          addFileEditorPanel(savedTab.path, savedTab.name, true);
        } catch {
          // Failed to restore tab, skip
        }
      }

      // Restore the previously active panel
      const dockApi = useDockviewStore.getState().api;
      if (dockApi) {
        const targetPanel = dockApi.getPanel(savedActiveTab);
        if (targetPanel) {
          targetPanel.api.setActive();
        }
      }

      _restorationInProgress = false;
    };

    void loadTabs();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSessionId]);

  // Persist open file tabs via zustand store subscriber — does NOT cause React re-renders.
  // This replaces the old useEffect that subscribed reactively to `openFiles`.
  useEffect(() => {
    const unsub = useDockviewStore.subscribe((state, prevState) => {
      if (state.openFiles === prevState.openFiles) return;
      const sessionId = activeSessionIdRef.current;
      if (!sessionId) return;
      if (_restorationInProgress) return;
      if (state.isRestoringLayout) return;
      const tabsToSave = Array.from(state.openFiles.values()).map(({ path, name }) => ({ path, name }));
      saveOpenFileTabs(sessionId, tabsToSave);
    });
    return unsub;
  }, []);

  // Persist active center panel to sessionStorage
  useEffect(() => {
    if (!api || !activeSessionId) return;
    const disposable = api.onDidActivePanelChange((event) => {
      if (_restorationInProgress) return;
      if (event) {
        setActiveTabForSession(activeSessionId, event.id);
      }
    });
    return () => disposable.dispose();
  }, [api, activeSessionId]);

  // Clean up file state when dockview panel is removed
  useEffect(() => {
    if (!api) return;
    const disposable = api.onDidRemovePanel((event) => {
      if (event.id.startsWith('file:')) {
        const path = event.id.replace('file:', '');
        removeFileState(path);
      }
    });
    return () => disposable.dispose();
  }, [api, removeFileState]);

  const openFile = useCallback(async (filePath: string) => {
    const client = getWebSocketClient();
    const currentSessionId = activeSessionIdRef.current;
    if (!client || !currentSessionId) return;

    // Read current state without subscribing
    const files = getOpenFiles();
    if (files.has(filePath)) {
      const name = filePath.split('/').pop() || filePath;
      addFileEditorPanel(filePath, name);
      return;
    }

    try {
      const response: FileContentResponse = await requestFileContent(client, currentSessionId, filePath);
      const fileName = filePath.split('/').pop() || filePath;
      const hash = await calculateHash(response.content);

      const state: FileEditorState = {
        path: filePath,
        name: fileName,
        content: response.content,
        originalContent: response.content,
        originalHash: hash,
        isDirty: false,
        isBinary: response.is_binary,
      };
      setFileState(filePath, state);
      addFileEditorPanel(filePath, fileName);
    } catch (error) {
      const reason = error instanceof Error ? error.message : 'Unknown error';
      toast({
        title: 'Failed to open file',
        description: reason,
        variant: 'error',
      });
    }
  }, [setFileState, addFileEditorPanel, toast]);

  const handleFileChange = useCallback((path: string, newContent: string) => {
    // Read current state without subscribing
    const file = getOpenFiles().get(path);
    if (!file) return;
    updateFileState(path, {
      content: newContent,
      isDirty: newContent !== file.originalContent,
    });
  }, [updateFileState]);

  const saveFile = useCallback(async (path: string) => {
    // Read current state without subscribing
    const file = getOpenFiles().get(path);
    if (!file || !file.isDirty) return;

    const client = getWebSocketClient();
    const currentSessionId = activeSessionIdRef.current;
    if (!client || !currentSessionId) return;

    setSavingFiles((prev) => new Set(prev).add(path));

    try {
      const diff = generateUnifiedDiff(file.originalContent, file.content, file.path);
      const response = await updateFileContent(client, currentSessionId, path, diff, file.originalHash);

      if (response.success && response.new_hash) {
        updateFileState(path, {
          originalContent: file.content,
          originalHash: response.new_hash,
          isDirty: false,
        });

        // Update dockview tab title (remove dirty indicator)
        const dockApi = useDockviewStore.getState().api;
        const panel = dockApi?.getPanel(`file:${path}`);
        if (panel) {
          panel.api.updateParameters({ isDirty: false });
          panel.setTitle(file.name);
        }
      } else {
        toast({
          title: 'Save failed',
          description: response.error || 'Failed to save file',
          variant: 'error',
        });
      }
    } catch (error) {
      toast({
        title: 'Save failed',
        description: error instanceof Error ? error.message : 'An error occurred while saving the file',
        variant: 'error',
      });
    } finally {
      setSavingFiles((prev) => {
        const next = new Set(prev);
        next.delete(path);
        return next;
      });
    }
  }, [updateFileState, toast]);

  const deleteFileAction = useCallback(async (path: string) => {
    const client = getWebSocketClient();
    const currentSessionId = activeSessionIdRef.current;
    if (!client || !currentSessionId) return;

    // Close the editor tab immediately (optimistic)
    const dockApi = useDockviewStore.getState().api;
    const panel = dockApi?.getPanel(`file:${path}`);
    if (panel) {
      dockApi?.removePanel(panel);
    }

    try {
      const response = await deleteFile(client, currentSessionId, path);
      if (!response.success) {
        toast({
          title: 'Delete failed',
          description: response.error || 'Failed to delete file',
          variant: 'error',
        });
      }
    } catch (error) {
      toast({
        title: 'Delete failed',
        description: error instanceof Error ? error.message : 'An error occurred while deleting the file',
        variant: 'error',
      });
    }
  }, [toast]);

  return {
    savingFiles,
    openFile,
    saveFile,
    deleteFile: deleteFileAction,
    handleFileChange,
  };
}
