'use client';

import { useEffect, useLayoutEffect, useState, useCallback, useRef, useMemo } from 'react';
import { getSessionStorage } from '@/lib/local-storage';
import { useAppStore } from '@/components/state-provider';
import { stopProcess } from '@/lib/api';
import { stopUserShell, createUserShell } from '@/lib/api/domains/user-shell-api';
import { useUserShells } from './use-user-shells';
import type { RepositoryScript } from '@/lib/types/http';
import type { MouseEvent } from 'react';

export type TerminalType = 'dev-server' | 'shell' | 'script';

export type Terminal = {
  id: string;
  type: TerminalType;
  label: string;
  closable: boolean; // Whether the terminal can be closed (backend is source of truth)
};

interface UseTerminalsOptions {
  sessionId: string | null;
  initialTerminals?: Terminal[]; // Initial terminals from SSR
}

interface UseTerminalsReturn {
  terminals: Terminal[];
  activeTab: string | undefined;
  terminalTabValue: string;
  addTerminal: () => void;
  removeTerminal: (id: string) => void;
  handleCloseDevTab: (event: MouseEvent) => Promise<void>;
  handleCloseTab: (event: MouseEvent, terminalId: string) => void;
  handleRunCommand: (script: RepositoryScript) => void;
  isStoppingDev: boolean;
  devProcessId: string | undefined;
  devOutput: string;
}

export function useTerminals({ sessionId, initialTerminals }: UseTerminalsOptions): UseTerminalsReturn {
  // Initialize terminals from SSR props if provided, otherwise empty
  const [terminals, setTerminals] = useState<Terminal[]>(() => initialTerminals ?? []);
  const [isStoppingDev, setIsStoppingDev] = useState(false);

  // Track refs
  const tabRestoredRef = useRef(false);
  const prevSessionIdRef = useRef(sessionId);
  // Track if sessionId just changed - used to ignore stale store values during transition
  const sessionJustChanged = sessionId !== prevSessionIdRef.current;

  // Store selectors
  const activeTab = useAppStore((state) =>
    sessionId ? state.rightPanel.activeTabBySessionId[sessionId] : undefined
  );
  const setRightPanelActiveTab = useAppStore((state) => state.setRightPanelActiveTab);
  const devProcessId = useAppStore((state) =>
    sessionId ? state.processes.devProcessBySessionId[sessionId] : undefined
  );
  const devOutput = useAppStore((state) =>
    devProcessId ? state.processes.outputsByProcessId[devProcessId] ?? '' : ''
  );
  const previewOpen = useAppStore((state) =>
    sessionId ? state.previewPanel.openBySessionId[sessionId] ?? false : false
  );
  const setPreviewOpen = useAppStore((state) => state.setPreviewOpen);
  const setPreviewStage = useAppStore((state) => state.setPreviewStage);

  // Use the user shells hook - this fetches running shells from backend
  // Backend now returns label and initialCommand for each shell
  const { shells: userShells, isLoaded: userShellsLoaded } = useUserShells(sessionId);

  // Reset terminals and refs when sessionId changes
  useEffect(() => {
    if (prevSessionIdRef.current === sessionId) return;

    prevSessionIdRef.current = sessionId;

    // Reset refs
    tabRestoredRef.current = false;

    // Clear terminals - will be populated by user shells effect
    setTerminals([]);

    // Clear active tab from store so sessionStorage restoration can set the correct value
    // This prevents stale activeTab from store being used when switching back to a session
    if (sessionId) {
      setRightPanelActiveTab(sessionId, '');
    }
  }, [sessionId, setRightPanelActiveTab]);

  // Populate terminals from user shells hook
  // Backend is source of truth - always returns at least one "Terminal" entry
  useEffect(() => {
    if (!sessionId || !userShellsLoaded) return;

    setTerminals(prev => {
      // Keep dev-server terminal if it exists
      const devTerminal = prev.find(t => t.type === 'dev-server');

      // Build terminals from backend data, preserving the order returned by backend
      // Backend returns them sorted by CreatedAt for stable ordering
      const userTerminals: Terminal[] = userShells.map((shell) => {
        const isScript = shell.terminalId.startsWith('script-');
        return {
          id: shell.terminalId,
          type: isScript ? 'script' as const : 'shell' as const,
          label: shell.label || (isScript ? 'Script' : 'Terminal'),
          closable: shell.closable,
        };
      });

      const result: Terminal[] = [];
      if (devTerminal) result.push(devTerminal);
      result.push(...userTerminals);

      return result;
    });
  }, [sessionId, userShells, userShellsLoaded]);

  // Sync dev server terminal with preview state
  useEffect(() => {
    if (!sessionId) return;

    setTerminals((prev) => {
      const hasDevTerminal = prev.some((t) => t.type === 'dev-server');

      if (previewOpen && !hasDevTerminal) {
        return [{ id: 'dev-server', type: 'dev-server' as const, label: 'Dev Server', closable: true }, ...prev];
      }

      if (!previewOpen && hasDevTerminal) {
        return prev.filter((t) => t.type !== 'dev-server');
      }

      return prev;
    });
  }, [previewOpen, sessionId]);

  // Sync saved tab to store - useLayoutEffect runs before paint
  // This ensures the store has the correct value for subsequent interactions
  useLayoutEffect(() => {
    // Treat empty string as no active tab (we clear it on session change)
    const hasActiveTab = activeTab && activeTab !== '';

    if (!sessionId || tabRestoredRef.current || hasActiveTab) {
      return;
    }

    const savedTab = getSessionStorage<string | null>(`rightPanel-tab-${sessionId}`, null);
    if (!savedTab) {
      return;
    }

    // Restore if the terminal exists
    const exists = terminals.some(t => t.id === savedTab);

    if (exists) {
      setRightPanelActiveTab(sessionId, savedTab);
      tabRestoredRef.current = true;
    }
  }, [sessionId, terminals, activeTab, setRightPanelActiveTab]);

  // Validate active tab exists in terminals
  // IMPORTANT: Only validate after terminals have been loaded for this session
  // Skip validation if terminals are empty (still loading) or if tab hasn't been restored yet
  useEffect(() => {
    if (!sessionId || !activeTab || activeTab === '') return;

    // Skip validation if terminals haven't been loaded yet (avoid validating against stale data)
    if (terminals.length === 0) return;

    // Skip validation if we haven't restored from sessionStorage yet
    if (!tabRestoredRef.current) return;

    const tabExists =
      activeTab === 'commands' ||
      terminals.some((t) => t.id === activeTab);

    if (!tabExists) {
      const fallbackShell = terminals.find((t) => t.type === 'shell');
      if (fallbackShell) {
        setRightPanelActiveTab(sessionId, fallbackShell.id);
      }
    }
  }, [activeTab, sessionId, terminals, setRightPanelActiveTab]);

  const addTerminal = useCallback(async () => {
    if (!sessionId) return;

    try {
      // Ask backend to create terminal - backend assigns ID, label, and closable
      const result = await createUserShell(sessionId);

      const newTerminal: Terminal = {
        id: result.terminalId,
        type: 'shell',
        label: result.label,
        closable: result.closable,
      };

      setTerminals((prev) => [...prev, newTerminal]);
      setRightPanelActiveTab(sessionId, result.terminalId);
    } catch (error) {
      console.error('Failed to create user shell:', error);
    }
  }, [sessionId, setRightPanelActiveTab]);

  const removeTerminal = useCallback(
    (id: string) => {
      setTerminals((prev) => {
        const indexToRemove = prev.findIndex((t) => t.id === id);
        if (indexToRemove === -1) return prev;

        const terminalToRemove = prev[indexToRemove];

        // Don't allow removing non-closable terminals (backend is source of truth)
        if (!terminalToRemove.closable) {
          return prev;
        }

        const nextTerminals = prev.filter((t) => t.id !== id);

        // If we're closing the active tab, focus the one to the left (or right if leftmost)
        if (activeTab === id && sessionId) {
          // Find the terminal to the left, or to the right if there's none on the left
          const leftIndex = indexToRemove - 1;
          const newFocusTerminal = leftIndex >= 0 ? prev[leftIndex] : nextTerminals[0];
          if (newFocusTerminal) {
            setRightPanelActiveTab(sessionId, newFocusTerminal.id);
          }
        }

        return nextTerminals;
      });
    },
    [activeTab, sessionId, setRightPanelActiveTab]
  );

  const handleCloseDevTab = useCallback(
    async (event: MouseEvent) => {
      event.preventDefault();
      event.stopPropagation();
      if (!sessionId) return;
      if (devProcessId) {
        setIsStoppingDev(true);
        try {
          await stopProcess(sessionId, { process_id: devProcessId });
        } finally {
          setIsStoppingDev(false);
        }
      }
      const fallbackShell = terminals.find((t) => t.type === 'shell');
      if (fallbackShell) {
        setRightPanelActiveTab(sessionId, fallbackShell.id);
      }
      setPreviewOpen(sessionId, false);
      setPreviewStage(sessionId, 'closed');
    },
    [sessionId, devProcessId, terminals, setRightPanelActiveTab, setPreviewOpen, setPreviewStage]
  );

  // Handle running a custom command - creates a script terminal via backend
  // Backend looks up the script by ID, assigns terminal ID, label, and registers it
  const handleRunCommand = useCallback(async (script: RepositoryScript) => {
    if (!sessionId) return;

    try {
      const result = await createUserShell(sessionId, script.id);

      const newTerminal: Terminal = {
        id: result.terminalId,
        type: 'script',
        label: result.label,
        closable: result.closable,
      };

      setTerminals(prev => [...prev, newTerminal]);
      setRightPanelActiveTab(sessionId, result.terminalId);
    } catch (error) {
      console.error('Failed to create script terminal:', error);
    }
  }, [sessionId, setRightPanelActiveTab]);

  // Handle closing a terminal tab (shell or script) - stops the user shell process
  const handleCloseTab = useCallback((event: MouseEvent, terminalId: string) => {
    event.preventDefault();
    event.stopPropagation();

    removeTerminal(terminalId);

    if (sessionId) {
      stopUserShell(sessionId, terminalId).catch((error) => {
        console.error('Failed to stop terminal:', error);
      });
    }
  }, [sessionId, removeTerminal]);

  // Read saved tab from sessionStorage synchronously to avoid flicker on initial render
  const savedTabFromStorage = useMemo(() => {
    if (!sessionId) return null;
    return getSessionStorage<string | null>(`rightPanel-tab-${sessionId}`, null);
  }, [sessionId]);

  // Check if saved tab exists in current terminals
  const savedTabExists = savedTabFromStorage && terminals.some(t => t.id === savedTabFromStorage);

  // Use saved tab as fallback before store is updated - this prevents flicker
  // Trust the saved tab if terminals haven't loaded yet (length === 0) or if it exists
  // Treat empty string activeTab as falsy (we clear it on session change)
  // IMPORTANT: Ignore store's activeTab if session just changed - it may be stale from before switching away
  const effectiveActiveTab = (!sessionJustChanged && activeTab && activeTab !== '') ? activeTab : null;
  const terminalTabValue = effectiveActiveTab
    ?? (savedTabFromStorage && (terminals.length === 0 || savedTabExists) ? savedTabFromStorage : null)
    ?? terminals.find((t) => t.type === 'shell')?.id
    ?? 'commands';

  return {
    terminals,
    activeTab,
    terminalTabValue,
    addTerminal,
    removeTerminal,
    handleCloseDevTab,
    handleCloseTab,
    handleRunCommand,
    isStoppingDev,
    devProcessId,
    devOutput,
  };
}
