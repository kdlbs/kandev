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
import type {
  UserShellInfo,
  UserShellKind,
  UserShellState,
  UserShellPTYStatus,
} from "@/lib/state/slices/session-runtime/types";
import type { RepositoryScript } from "@/lib/types/http";
import type { Dispatch, SetStateAction, MouseEvent } from "react";
import type { PreviewStage } from "@/lib/state/slices";

const TERMINAL_TYPE_DEV_SERVER = "dev-server";
const BOTTOM_PANEL_TERMINAL_ID = "bottom-panel";

export type TerminalType = "dev-server" | "shell" | "script";

/**
 * Terminal tab descriptor consumed by the right-panel UI. Ordinary
 * terminals carry the new `kind="ordinary"` discriminator plus seq +
 * customName + state + ptyStatus; non-ordinary tabs (dev-server, scripts,
 * the fixed bottom-panel) leave those undefined.
 */
export type Terminal = {
  id: string;
  type: TerminalType;
  label: string;
  closable: boolean;
  kind?: UserShellKind;
  seq?: number;
  customName?: string | null;
  state?: UserShellState;
  ptyStatus?: UserShellPTYStatus;
};

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

function deriveLabel(shell: UserShellInfo): string {
  if (shell.customName && shell.customName !== "") return shell.customName;
  if (shell.displayName) return shell.displayName;
  if (shell.label) return shell.label;
  if (shell.kind === "ordinary" && shell.seq) return `Terminal ${shell.seq}`;
  return shell.terminalId.startsWith("script-") ? "Script" : "Terminal";
}

function isOrdinary(shell: UserShellInfo): boolean {
  if (shell.kind) return shell.kind === "ordinary";
  // Legacy fallback — ids without script-/bottom-panel prefix are treated
  // as ordinary if they have ordinary-shaped metadata.
  return (
    shell.terminalId !== BOTTOM_PANEL_TERMINAL_ID &&
    !shell.terminalId.startsWith("script-") &&
    shell.seq !== undefined
  );
}

function shellToTerminal(shell: UserShellInfo): Terminal {
  const ordinary = isOrdinary(shell);
  const isScript = shell.terminalId.startsWith("script-");
  return {
    id: shell.terminalId,
    type: isScript ? "script" : "shell",
    label: deriveLabel(shell),
    closable: shell.closable ?? true,
    kind: ordinary ? "ordinary" : shell.kind,
    seq: shell.seq,
    customName: shell.customName,
    state: shell.state,
    ptyStatus: shell.ptyStatus,
  };
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
function buildTerminalsFromShells(prev: Terminal[], userShells: UserShellInfo[]): Terminal[] {
  const devTerminal = prev.find((t) => t.type === TERMINAL_TYPE_DEV_SERVER);
  const visibleShells = userShells.filter((s) => {
    // Parked terminals live in their own submenu, not the main strip.
    if (s.state === "parked") return false;
    // The bottom-panel terminal renders inside its own dedicated component
    // (bottom-terminal-panel.tsx) — exclude it from the right-panel strip.
    if (s.terminalId === BOTTOM_PANEL_TERMINAL_ID) return false;
    return true;
  });
  const userTerminals = visibleShells.map(shellToTerminal);
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
      const newTerm: Terminal = {
        id: result.terminalId,
        type: "shell",
        label: result.displayName ?? result.label ?? "Terminal",
        closable: true,
        kind: result.kind ?? "ordinary",
        seq: result.seq,
        state: result.state ?? "open",
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
  activeTab: string | undefined;
  sessionId: string | null;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
};

function useRemoveTerminal({
  activeTab,
  sessionId,
  setTerminals,
  setRightPanelActiveTab,
}: RemoveTerminalOpts) {
  return useCallback(
    (id: string) => {
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
    [activeTab, sessionId, setRightPanelActiveTab, setTerminals],
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

  const addTerminal = useAddTerminal({
    environmentId,
    taskID,
    sessionId,
    setTerminals,
    setRightPanelActiveTab,
  });
  const removeTerminal = useRemoveTerminal({
    activeTab,
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
  const handleCloseTab = useCallback(
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
