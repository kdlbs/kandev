"use client";

import { useEffect, useLayoutEffect, useState, useCallback, useRef, useMemo } from "react";
import { getSessionStorage } from "@/lib/local-storage";
import { useAppStore } from "@/components/state-provider";
import { stopProcess } from "@/lib/api";
import {
  destroyUserShell,
  parkUserShell,
  resumeUserShell,
  renameUserShell,
  createUserShell,
} from "@/lib/api/domains/user-shell-api";
import { useUserShells } from "./use-user-shells";
import {
  buildTerminalsFromShells,
  computeTerminalTabValue,
  shellToTerminal,
  syncDevTerminal,
} from "./use-terminals-build";
import type { Terminal, TerminalType } from "./use-terminals-types";
import type { UserShellInfo } from "@/lib/state/slices/session-runtime/types";
import type { RepositoryScript } from "@/lib/types/http";
import type { Dispatch, SetStateAction, MouseEvent } from "react";
import type { PreviewStage } from "@/lib/state/slices";

export type { Terminal, TerminalType };

interface UseTerminalsOptions {
  /** Session id used for the active-tab UX (tab restoration is per-session). */
  sessionId: string | null;
  /** Task environment id — the agentctl scope. Required for stop/destroy. */
  environmentId: string | null;
  initialTerminals?: Terminal[];
}

interface UseTerminalsReturn {
  terminals: Terminal[];
  parkedTerminals: Terminal[];
  activeTab: string | undefined;
  terminalTabValue: string;
  addTerminal: () => void;
  removeTerminal: (id: string) => void;
  handleCloseDevTab: (event: MouseEvent) => Promise<void>;
  handleCloseTab: (event: MouseEvent, terminalId: string) => void;
  handleRunCommand: (script: RepositoryScript) => void;
  renameTerminal: (id: string, name: string | null) => Promise<void>;
  resumeTerminal: (id: string) => Promise<void>;
  destroyTerminal: (id: string) => Promise<void>;
  isStoppingDev: boolean;
  devProcessId: string | undefined;
  devOutput: string;
}

type TerminalSyncOptions = {
  environmentId: string | null;
  userShells: UserShellInfo[];
  userShellsLoaded: boolean;
  previewOpen: boolean;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
};

function useTerminalSync({
  environmentId,
  userShells,
  userShellsLoaded,
  previewOpen,
  setTerminals,
}: TerminalSyncOptions) {
  const tabRestoredRef = useRef(false);

  // Note: deliberately no "reset on env change" branch. Stranded process
  // recovery requires the UI to keep its terminal references stable; the
  // server's list call is authoritative once it lands.

  useEffect(() => {
    if (!environmentId || !userShellsLoaded) return;
    setTerminals((prev) => buildTerminalsFromShells(prev, userShells));
  }, [environmentId, userShells, userShellsLoaded, setTerminals]);

  useEffect(() => {
    if (!environmentId) return;
    setTerminals((prev) => syncDevTerminal(prev, previewOpen));
  }, [previewOpen, environmentId, setTerminals]);

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

type AddTerminalOpts = {
  environmentId: string | null;
  taskID: string | null;
  sessionId: string | null;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
};

function useAddTerminal({
  environmentId,
  taskID,
  sessionId,
  setTerminals,
  setRightPanelActiveTab,
}: AddTerminalOpts) {
  return useCallback(async () => {
    if (!environmentId) return;
    try {
      const result = await createUserShell(environmentId, { taskId: taskID ?? undefined });
      // During rollout an older backend returns the legacy payload
      // (terminal_id, label, closable) without `kind` or `seq`. Defaulting
      // to "ordinary" in that case marks the tab as managed and routes
      // close → park, which fails on the old backend with unknown-action.
      // `seq` is the only field a backend that supports the DB-backed
      // path always populates, so gate on its presence alone — a
      // bare-bones `kind: "ordinary"` without `seq` is treated as legacy.
      const ordinary = result.seq !== undefined;
      const newTerm: Terminal = {
        id: result.terminalId,
        type: "shell",
        label: result.displayName ?? result.label ?? "Terminal",
        closable: true,
        kind: ordinary ? "ordinary" : result.kind,
        seq: result.seq,
        state: ordinary ? (result.state ?? "open") : result.state,
        ptyStatus: result.ptyStatus ?? "stopped",
      };
      setTerminals((prev) => [...prev, newTerm]);
      if (sessionId) setRightPanelActiveTab(sessionId, result.terminalId);
    } catch (error) {
      console.error("Failed to create user shell:", error);
    }
  }, [environmentId, taskID, sessionId, setRightPanelActiveTab, setTerminals]);
}

type RemoveTerminalOpts = {
  /**
   * Source-of-truth getter for the currently active tab. The handler reads
   * this at call-time rather than capturing the value in the closure so an
   * async close (`parkUserShell(...).then(removeTerminal)`) that resolves
   * after the user switches tabs doesn't clobber the new selection with a
   * stale fallback shift.
   */
  getActiveTab: () => string | undefined;
  sessionId: string | null;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
};

function useRemoveTerminal({
  getActiveTab,
  sessionId,
  setTerminals,
  setRightPanelActiveTab,
}: RemoveTerminalOpts) {
  return useCallback(
    (id: string) => {
      const activeTab = getActiveTab();
      setTerminals((prev) => {
        const indexToRemove = prev.findIndex((t) => t.id === id);
        if (indexToRemove === -1) return prev;
        if (activeTab === id && sessionId) {
          const nextTerminals = prev.filter((_, i) => i !== indexToRemove);
          const next = indexToRemove > 0 ? prev[indexToRemove - 1] : nextTerminals[0];
          if (next) setRightPanelActiveTab(sessionId, next.id);
        }
        return prev.filter((t) => t.id !== id);
      });
    },
    [getActiveTab, sessionId, setRightPanelActiveTab, setTerminals],
  );
}

type CloseDevTabOpts = {
  sessionId: string | null;
  devProcessId: string | undefined;
  terminals: Terminal[];
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
  setPreviewOpen: (sessionId: string, open: boolean) => void;
  setPreviewStage: (sessionId: string, stage: PreviewStage) => void;
  setIsStoppingDev: Dispatch<SetStateAction<boolean>>;
};

function useCloseDevTab({
  sessionId,
  devProcessId,
  terminals,
  setRightPanelActiveTab,
  setPreviewOpen,
  setPreviewStage,
  setIsStoppingDev,
}: CloseDevTabOpts) {
  return useCallback(
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
    [
      sessionId,
      devProcessId,
      terminals,
      setRightPanelActiveTab,
      setPreviewOpen,
      setPreviewStage,
      setIsStoppingDev,
    ],
  );
}

type CloseTabOpts = {
  environmentId: string | null;
  taskID: string | null;
  terminals: Terminal[];
  removeTerminal: (id: string) => void;
  removeUserShellStore: (environmentId: string, terminalId: string) => void;
  updateUserShell: (
    environmentId: string,
    terminalId: string,
    patch: { state?: "open" | "parked" },
  ) => void;
};

/**
 * X-button close. For ordinary terminals this PARKS the tab (PTY keeps
 * running, user can resume from the "Parked terminals" submenu). For
 * scripts and any non-ordinary terminal it falls back to destroy.
 *
 * The local tab is removed only AFTER the backend call resolves — that
 * way a transient failure (network, backend 500) leaves the tab on the
 * strip rather than disappearing into thin air. The next `user_shell.list`
 * poll then reflects whatever state the backend actually settled on.
 */
function useCloseTab({
  environmentId,
  taskID,
  terminals,
  removeTerminal,
  removeUserShellStore,
  updateUserShell,
}: CloseTabOpts) {
  return useCallback(
    (event: MouseEvent, terminalId: string) => {
      event.preventDefault();
      event.stopPropagation();
      if (!environmentId) return;
      const term = terminals.find((t) => t.id === terminalId);
      const isOrdinaryTab = term?.kind === "ordinary";
      if (isOrdinaryTab) {
        parkUserShell(terminalId, taskID ?? undefined)
          .then(() => {
            updateUserShell(environmentId, terminalId, { state: "parked" });
            removeTerminal(terminalId);
          })
          .catch((error) => console.error("Failed to park terminal:", error));
      } else {
        destroyUserShell(environmentId, terminalId, taskID ?? undefined)
          .then(() => {
            removeUserShellStore(environmentId, terminalId);
            removeTerminal(terminalId);
          })
          .catch((error) => console.error("Failed to destroy terminal:", error));
      }
    },
    [environmentId, taskID, terminals, removeTerminal, removeUserShellStore, updateUserShell],
  );
}

type ManagedTerminalActionsOpts = {
  environmentId: string | null;
  taskID: string | null;
  updateUserShell: (
    environmentId: string,
    terminalId: string,
    patch: { customName?: string | null; state?: "open" | "parked" },
  ) => void;
  removeUserShellStore: (environmentId: string, terminalId: string) => void;
};

function useManagedTerminalActions({
  environmentId,
  taskID,
  updateUserShell,
  removeUserShellStore,
}: ManagedTerminalActionsOpts) {
  const renameTerminal = useCallback(
    async (id: string, name: string | null) => {
      if (!environmentId) return;
      const trimmed = name === null ? null : name.trim();
      const normalized = trimmed === "" ? null : trimmed;
      try {
        await renameUserShell(id, normalized, taskID ?? undefined);
        updateUserShell(environmentId, id, { customName: normalized });
      } catch (error) {
        console.error("Failed to rename terminal:", error);
      }
    },
    [environmentId, taskID, updateUserShell],
  );

  const resumeTerminal = useCallback(
    async (id: string) => {
      if (!environmentId) return;
      try {
        await resumeUserShell(id, taskID ?? undefined);
        updateUserShell(environmentId, id, { state: "open" });
      } catch (error) {
        console.error("Failed to resume terminal:", error);
      }
    },
    [environmentId, taskID, updateUserShell],
  );

  const destroyTerminal = useCallback(
    async (id: string) => {
      if (!environmentId) return;
      try {
        await destroyUserShell(environmentId, id, taskID ?? undefined);
        removeUserShellStore(environmentId, id);
      } catch (error) {
        console.error("Failed to destroy terminal:", error);
      }
    },
    [environmentId, taskID, removeUserShellStore],
  );

  return { renameTerminal, resumeTerminal, destroyTerminal };
}

type TerminalActionsOptions = {
  sessionId: string | null;
  environmentId: string | null;
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
  environmentId,
  activeTab,
  terminals,
  devProcessId,
  setTerminals,
  setRightPanelActiveTab,
  setPreviewOpen,
  setPreviewStage,
}: TerminalActionsOptions) {
  const [isStoppingDev, setIsStoppingDev] = useState(false);
  const updateUserShell = useAppStore((state) => state.updateUserShell);
  const removeUserShellStore = useAppStore((state) => state.removeUserShell);

  const taskID = useAppStore((state) => state.tasks?.activeTaskId ?? null);

  // Track the latest activeTab in a ref so async close handlers read the
  // current selection instead of the value captured when handleCloseTab
  // first ran. Without this, tab switches that happen mid-park/destroy
  // can be silently undone by the .then() fallback-shift logic.
  const activeTabRef = useRef(activeTab);
  useEffect(() => {
    activeTabRef.current = activeTab;
  }, [activeTab]);
  const getActiveTab = useCallback(() => activeTabRef.current, []);

  const addTerminal = useAddTerminal({
    environmentId,
    taskID,
    sessionId,
    setTerminals,
    setRightPanelActiveTab,
  });
  const removeTerminal = useRemoveTerminal({
    getActiveTab,
    sessionId,
    setTerminals,
    setRightPanelActiveTab,
  });

  const handleCloseDevTab = useCloseDevTab({
    sessionId,
    devProcessId,
    terminals,
    setRightPanelActiveTab,
    setPreviewOpen,
    setPreviewStage,
    setIsStoppingDev,
  });

  const handleRunCommand = useCallback(
    async (script: RepositoryScript) => {
      if (!environmentId) return;
      try {
        const result = await createUserShell(environmentId, { scriptId: script.id });
        const newTerm: Terminal = {
          id: result.terminalId,
          type: "script",
          label: result.label ?? script.name ?? "Script",
          closable: true,
          kind: "script",
        };
        setTerminals((prev) => [...prev, newTerm]);
        if (sessionId) setRightPanelActiveTab(sessionId, result.terminalId);
      } catch (error) {
        console.error("Failed to create script terminal:", error);
      }
    },
    [environmentId, sessionId, setRightPanelActiveTab, setTerminals],
  );

  const handleCloseTab = useCloseTab({
    environmentId,
    taskID,
    terminals,
    removeTerminal,
    removeUserShellStore,
    updateUserShell,
  });

  const { renameTerminal, resumeTerminal, destroyTerminal } = useManagedTerminalActions({
    environmentId,
    taskID,
    updateUserShell,
    removeUserShellStore,
  });

  return {
    isStoppingDev,
    addTerminal,
    removeTerminal,
    handleCloseDevTab,
    handleRunCommand,
    handleCloseTab,
    renameTerminal,
    resumeTerminal,
    destroyTerminal,
  };
}

export function useTerminals({
  sessionId,
  environmentId,
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

  // Pass taskID so the backend's DB-backed ordinary-shell path fires; without
  // it `user_shell.list` only returns legacy passthrough shells and the
  // parked-terminals submenu stays empty.
  const activeTaskId = useAppStore((state) => state.tasks?.activeTaskId ?? null);
  const { shells: userShells, isLoaded: userShellsLoaded } = useUserShells(
    environmentId,
    activeTaskId,
  );

  const tabRestoredRef = useTerminalSync({
    environmentId,
    userShells,
    userShellsLoaded,
    previewOpen,
    setTerminals,
  });

  useTabRestoration(sessionId, terminals, activeTab, tabRestoredRef, setRightPanelActiveTab);

  const {
    isStoppingDev,
    addTerminal,
    removeTerminal,
    handleCloseDevTab,
    handleRunCommand,
    handleCloseTab,
    renameTerminal,
    resumeTerminal,
    destroyTerminal,
  } = useTerminalActions({
    sessionId,
    environmentId,
    activeTab,
    terminals,
    devProcessId,
    setTerminals,
    setRightPanelActiveTab,
    setPreviewOpen,
    setPreviewStage,
  });

  const parkedTerminals = useMemo<Terminal[]>(() => {
    if (!userShellsLoaded) return [];
    return userShells.filter((s) => s.state === "parked").map(shellToTerminal);
  }, [userShells, userShellsLoaded]);

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
    parkedTerminals,
    activeTab,
    terminalTabValue,
    addTerminal,
    removeTerminal,
    handleCloseDevTab,
    handleCloseTab,
    handleRunCommand,
    renameTerminal,
    resumeTerminal,
    destroyTerminal,
    isStoppingDev,
    devProcessId,
    devOutput,
  };
}
