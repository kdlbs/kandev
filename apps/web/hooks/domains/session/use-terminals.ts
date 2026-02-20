"use client";

import { useEffect, useLayoutEffect, useState, useCallback, useRef, useMemo } from "react";
import { getSessionStorage } from "@/lib/local-storage";
import { useAppStore } from "@/components/state-provider";
import { stopProcess } from "@/lib/api";
import { stopUserShell, createUserShell } from "@/lib/api/domains/user-shell-api";
import { useUserShells } from "./use-user-shells";
import type { RepositoryScript } from "@/lib/types/http";
import type { Dispatch, SetStateAction, MouseEvent } from "react";
import type { PreviewStage } from "@/lib/state/slices";

const TERMINAL_TYPE_DEV_SERVER = "dev-server";

export type TerminalType = "dev-server" | "shell" | "script";

export type Terminal = {
  id: string;
  type: TerminalType;
  label: string;
  closable: boolean;
};

interface UseTerminalsOptions {
  sessionId: string | null;
  initialTerminals?: Terminal[];
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

/** Compute the effective active tab value, preferring store, then sessionStorage, then fallback. */
function computeTerminalTabValue(
  activeTab: string | undefined,
  sessionJustChanged: boolean,
  savedTabFromStorage: string | null,
  terminals: Terminal[],
  savedTabExists: boolean,
): string {
  const effectiveActiveTab =
    !sessionJustChanged && activeTab && activeTab !== "" ? activeTab : null;
  return (
    effectiveActiveTab ??
    (savedTabFromStorage && (terminals.length === 0 || savedTabExists)
      ? savedTabFromStorage
      : null) ??
    terminals.find((t) => t.type === "shell")?.id ??
    "commands"
  );
}

/** Build terminal list from user shells, preserving dev-server terminal if present. */
function buildTerminalsFromShells(
  prev: Terminal[],
  userShells: Array<{ terminalId: string; label: string; closable: boolean }>,
): Terminal[] {
  const devTerminal = prev.find((t) => t.type === TERMINAL_TYPE_DEV_SERVER);
  const userTerminals: Terminal[] = userShells.map((shell) => {
    const isScript = shell.terminalId.startsWith("script-");
    return {
      id: shell.terminalId,
      type: isScript ? ("script" as const) : ("shell" as const),
      label: shell.label || (isScript ? "Script" : "Terminal"),
      closable: shell.closable,
    };
  });
  const result: Terminal[] = [];
  if (devTerminal) result.push(devTerminal);
  result.push(...userTerminals);
  return result;
}

/** Sync the dev-server terminal with preview open state. */
function syncDevTerminal(prev: Terminal[], previewOpen: boolean): Terminal[] {
  const hasDevTerminal = prev.some((t) => t.type === TERMINAL_TYPE_DEV_SERVER);
  if (previewOpen && !hasDevTerminal) {
    return [
      {
        id: TERMINAL_TYPE_DEV_SERVER,
        type: TERMINAL_TYPE_DEV_SERVER as TerminalType,
        label: "Dev Server",
        closable: true,
      },
      ...prev,
    ];
  }
  if (!previewOpen && hasDevTerminal) {
    return prev.filter((t) => t.type !== TERMINAL_TYPE_DEV_SERVER);
  }
  return prev;
}

type TerminalSyncOptions = {
  sessionId: string | null;
  userShells: Array<{ terminalId: string; label: string; closable: boolean }>;
  userShellsLoaded: boolean;
  previewOpen: boolean;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
};

function useTerminalSync({
  sessionId,
  userShells,
  userShellsLoaded,
  previewOpen,
  setTerminals,
  setRightPanelActiveTab,
}: TerminalSyncOptions) {
  const prevSessionIdRef = useRef(sessionId);
  const tabRestoredRef = useRef(false);

  useEffect(() => {
    if (prevSessionIdRef.current === sessionId) return;
    prevSessionIdRef.current = sessionId;
    tabRestoredRef.current = false;
    setTerminals([]);
    if (sessionId) setRightPanelActiveTab(sessionId, "");
  }, [sessionId, setRightPanelActiveTab, setTerminals]);

  useEffect(() => {
    if (!sessionId || !userShellsLoaded) return;
    setTerminals((prev) => buildTerminalsFromShells(prev, userShells));
  }, [sessionId, userShells, userShellsLoaded, setTerminals]);

  useEffect(() => {
    if (!sessionId) return;
    setTerminals((prev) => syncDevTerminal(prev, previewOpen));
  }, [previewOpen, sessionId, setTerminals]);

  return tabRestoredRef;
}

function useTabRestoration(
  sessionId: string | null,
  terminals: Terminal[],
  activeTab: string | undefined,
  tabRestoredRef: React.MutableRefObject<boolean>,
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void,
) {
  useLayoutEffect(() => {
    const hasActiveTab = activeTab && activeTab !== "";
    if (!sessionId || tabRestoredRef.current || hasActiveTab) return;
    const savedTab = getSessionStorage<string | null>(`rightPanel-tab-${sessionId}`, null);
    if (!savedTab) return;
    if (terminals.some((t) => t.id === savedTab)) {
      setRightPanelActiveTab(sessionId, savedTab);
      tabRestoredRef.current = true;
    }
  }, [sessionId, terminals, activeTab, setRightPanelActiveTab, tabRestoredRef]);

  useEffect(() => {
    if (!sessionId || !activeTab || activeTab === "") return;
    if (terminals.length === 0 || !tabRestoredRef.current) return;
    const tabExists = activeTab === "commands" || terminals.some((t) => t.id === activeTab);
    if (!tabExists) {
      const fallbackShell = terminals.find((t) => t.type === "shell");
      if (fallbackShell) setRightPanelActiveTab(sessionId, fallbackShell.id);
    }
  }, [activeTab, sessionId, terminals, setRightPanelActiveTab, tabRestoredRef]);
}

function useTerminalStore(sessionId: string | null, devProcessId: string | undefined) {
  const activeTab = useAppStore((state) =>
    sessionId ? state.rightPanel.activeTabBySessionId[sessionId] : undefined,
  );
  const setRightPanelActiveTab = useAppStore((state) => state.setRightPanelActiveTab);
  const devOutput = useAppStore((state) =>
    devProcessId ? (state.processes.outputsByProcessId[devProcessId] ?? "") : "",
  );
  const previewOpen = useAppStore((state) =>
    sessionId ? (state.previewPanel.openBySessionId[sessionId] ?? false) : false,
  );
  const setPreviewOpen = useAppStore((state) => state.setPreviewOpen);
  const setPreviewStage = useAppStore((state) => state.setPreviewStage);
  return {
    activeTab,
    setRightPanelActiveTab,
    devOutput,
    previewOpen,
    setPreviewOpen,
    setPreviewStage,
  };
}

type TerminalActionsOptions = {
  sessionId: string | null;
  activeTab: string | undefined;
  terminals: Terminal[];
  devProcessId: string | undefined;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
  setPreviewOpen: (sessionId: string, open: boolean) => void;
  setPreviewStage: (sessionId: string, stage: PreviewStage) => void;
};

function useTerminalActions({
  sessionId,
  activeTab,
  terminals,
  devProcessId,
  setTerminals,
  setRightPanelActiveTab,
  setPreviewOpen,
  setPreviewStage,
}: TerminalActionsOptions) {
  const [isStoppingDev, setIsStoppingDev] = useState(false);

  const addTerminal = useCallback(async () => {
    if (!sessionId) return;
    try {
      const result = await createUserShell(sessionId);
      setTerminals((prev) => [
        ...prev,
        { id: result.terminalId, type: "shell", label: result.label, closable: result.closable },
      ]);
      setRightPanelActiveTab(sessionId, result.terminalId);
    } catch (error) {
      console.error("Failed to create user shell:", error);
    }
  }, [sessionId, setRightPanelActiveTab, setTerminals]);

  const removeTerminal = useCallback(
    (id: string) => {
      setTerminals((prev) => {
        const indexToRemove = prev.findIndex((t) => t.id === id);
        if (indexToRemove === -1 || !prev[indexToRemove].closable) return prev;
        if (activeTab === id && sessionId) {
          const nextTerminals = prev.filter((_, i) => i !== indexToRemove);
          const next = indexToRemove > 0 ? prev[indexToRemove - 1] : nextTerminals[0];
          if (next) setRightPanelActiveTab(sessionId, next.id);
        }
        return prev.filter((t) => t.id !== id);
      });
    },
    [activeTab, sessionId, setRightPanelActiveTab, setTerminals],
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
      const fallbackShell = terminals.find((t) => t.type === "shell");
      if (fallbackShell) setRightPanelActiveTab(sessionId, fallbackShell.id);
      setPreviewOpen(sessionId, false);
      setPreviewStage(sessionId, "closed");
    },
    [sessionId, devProcessId, terminals, setRightPanelActiveTab, setPreviewOpen, setPreviewStage],
  );

  const handleRunCommand = useCallback(
    async (script: RepositoryScript) => {
      if (!sessionId) return;
      try {
        const result = await createUserShell(sessionId, script.id);
        setTerminals((prev) => [
          ...prev,
          { id: result.terminalId, type: "script", label: result.label, closable: result.closable },
        ]);
        setRightPanelActiveTab(sessionId, result.terminalId);
      } catch (error) {
        console.error("Failed to create script terminal:", error);
      }
    },
    [sessionId, setRightPanelActiveTab, setTerminals],
  );

  const handleCloseTab = useCallback(
    (event: MouseEvent, terminalId: string) => {
      event.preventDefault();
      event.stopPropagation();
      removeTerminal(terminalId);
      if (sessionId) {
        stopUserShell(sessionId, terminalId).catch((error) => {
          console.error("Failed to stop terminal:", error);
        });
      }
    },
    [sessionId, removeTerminal],
  );

  return {
    isStoppingDev,
    addTerminal,
    removeTerminal,
    handleCloseDevTab,
    handleRunCommand,
    handleCloseTab,
  };
}

export function useTerminals({
  sessionId,
  initialTerminals,
}: UseTerminalsOptions): UseTerminalsReturn {
  const [terminals, setTerminals] = useState<Terminal[]>(() => initialTerminals ?? []);
  const [prevSessionId, setPrevSessionId] = useState(sessionId);
  const sessionJustChanged = sessionId !== prevSessionId;
  if (sessionJustChanged) setPrevSessionId(sessionId);

  const devProcessId = useAppStore((state) =>
    sessionId ? state.processes.devProcessBySessionId[sessionId] : undefined,
  );
  const {
    activeTab,
    setRightPanelActiveTab,
    devOutput,
    previewOpen,
    setPreviewOpen,
    setPreviewStage,
  } = useTerminalStore(sessionId, devProcessId);

  const { shells: userShells, isLoaded: userShellsLoaded } = useUserShells(sessionId);

  const tabRestoredRef = useTerminalSync({
    sessionId,
    userShells,
    userShellsLoaded,
    previewOpen,
    setTerminals,
    setRightPanelActiveTab,
  });

  useTabRestoration(sessionId, terminals, activeTab, tabRestoredRef, setRightPanelActiveTab);

  const {
    isStoppingDev,
    addTerminal,
    removeTerminal,
    handleCloseDevTab,
    handleRunCommand,
    handleCloseTab,
  } = useTerminalActions({
    sessionId,
    activeTab,
    terminals,
    devProcessId,
    setTerminals,
    setRightPanelActiveTab,
    setPreviewOpen,
    setPreviewStage,
  });

  const savedTabFromStorage = useMemo(() => {
    if (!sessionId) return null;
    return getSessionStorage<string | null>(`rightPanel-tab-${sessionId}`, null);
  }, [sessionId]);

  const savedTabExists = savedTabFromStorage && terminals.some((t) => t.id === savedTabFromStorage);

  const terminalTabValue = computeTerminalTabValue(
    activeTab,
    sessionJustChanged,
    savedTabFromStorage,
    terminals,
    !!savedTabExists,
  );

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
