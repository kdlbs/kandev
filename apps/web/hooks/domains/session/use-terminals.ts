"use client";

import { useEffect, useLayoutEffect, useState, useCallback, useRef } from "react";
import { getSessionStorage } from "@/lib/local-storage";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { stopProcess } from "@/lib/api";
import {
  resumeUserShell,
  renameUserShell,
  createUserShell,
} from "@/lib/api/domains/user-shell-api";
import { useUserShells } from "./use-user-shells";
import { useTerminalDestroy } from "./use-terminal-destroy";
import {
  appendTerminalIfMissing,
  buildParkedTerminals,
  buildTerminalsFromShells,
  computeTerminalTabValue,
  syncDevTerminal,
  TERMINAL_TYPE_DEV_SERVER,
} from "./use-terminals-build";
import type { Terminal, TerminalType } from "./use-terminals-types";
import type { UserShellInfo } from "@/lib/state/slices/session-runtime/types";
import {
  captureUserShellPublicationOwner,
  isActiveUserShellPublicationOwner,
  makeUserShellScope,
  type UserShellScope,
} from "@/lib/state/user-shell-scope";
import { createOrdinaryShellOnce } from "@/lib/state/user-shell-creation";
import type { RepositoryScript } from "@/lib/types/http";
import type { Dispatch, SetStateAction, MouseEvent } from "react";
import type { PreviewStage } from "@/lib/state/slices";
import { t } from "@/lib/i18n";

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
  destroyTerminal: (id: string) => Promise<boolean>;
  isStoppingDev: boolean;
  devProcessId: string | undefined;
  devOutput: string;
}

type TerminalSyncOptions = {
  environmentId: string | null;
  taskId: string | null;
  userShells: UserShellInfo[];
  userShellsLoaded: boolean;
  previewOpen: boolean;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
};

function useTerminalSync({
  environmentId,
  taskId,
  userShells,
  userShellsLoaded,
  previewOpen,
  setTerminals,
}: TerminalSyncOptions) {
  const tabRestoredRef = useRef(false);
  const previousTaskRef = useRef(taskId);

  // Note: deliberately no "reset on env change" branch. Stranded process
  // recovery requires the UI to keep its terminal references stable; the
  // server's list call is authoritative once it lands. A task change is
  // different: ordinary terminals are task-owned, so the previous task's
  // rows must not be rendered while the new scope is still loading.
  useEffect(() => {
    if (previousTaskRef.current === taskId) return;
    previousTaskRef.current = taskId;
    setTerminals((prev) => prev.filter((t) => t.type === TERMINAL_TYPE_DEV_SERVER));
  }, [taskId, setTerminals]);

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

function shellFromCreatedTerminal(
  result: Awaited<ReturnType<typeof createUserShell>>,
): UserShellInfo {
  return {
    ...result,
    terminalId: result.terminalId,
    kind: result.kind,
    state: result.state ?? "open",
    label: result.displayName ?? result.label ?? t("common:terminal"),
    closable: result.closable ?? true,
    running: result.ptyStatus === "running",
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
  const addUserShell = useAppStore((state) => state.addUserShell);
  const storeApi = useAppStoreApi();
  return useCallback(async () => {
    if (!environmentId) return;
    const scope = makeUserShellScope(environmentId, taskID);
    if (!scope) return;
    const owner = captureUserShellPublicationOwner(storeApi.getState(), scope);
    try {
      const shell = taskID
        ? await createOrdinaryShellOnce(storeApi, { environmentId, taskId: taskID }, async () =>
            shellFromCreatedTerminal(await createUserShell(environmentId, { taskId: taskID })),
          )
        : shellFromCreatedTerminal(await createUserShell(environmentId, { taskId: undefined }));
      // During rollout an older backend returns the legacy payload
      // (terminal_id, label, closable) without `kind` or `seq`. Defaulting
      // to "ordinary" in that case marks the tab as managed and routes
      // close → park, which fails on the old backend with unknown-action.
      // `seq` is the only field a backend that supports the DB-backed
      // path always populates, so gate on its presence alone — a
      // bare-bones `kind: "ordinary"` without `seq` is treated as legacy.
      const ordinary = shell.seq !== undefined;
      const newTerm: Terminal = {
        id: shell.terminalId,
        type: "shell",
        label: shell.displayName ?? shell.label ?? t("common:terminal"),
        closable: true,
        kind: ordinary ? "ordinary" : shell.kind,
        seq: shell.seq,
        state: ordinary ? (shell.state ?? "open") : shell.state,
        ptyStatus: shell.ptyStatus ?? "stopped",
      };
      if (!taskID) addUserShell(scope, shell);
      // The captured cache always receives the row; local state and panel
      // selection are only for the task the user is still looking at.
      if (!isActiveUserShellPublicationOwner(storeApi.getState(), owner)) return;
      setTerminals((prev) => appendTerminalIfMissing(prev, newTerm));
      if (sessionId) setRightPanelActiveTab(sessionId, shell.terminalId);
    } catch (error) {
      console.error("Failed to create user shell:", error);
    }
  }, [
    environmentId,
    taskID,
    sessionId,
    addUserShell,
    setRightPanelActiveTab,
    setTerminals,
    storeApi,
  ]);
}

type RemoveTerminalOpts = {
  /**
   * Source-of-truth getter for the currently active tab. The handler reads
   * this at call-time rather than capturing the value in the closure so an
   * async close (`destroyUserShell(...).then(removeTerminal)`) that resolves
   * after the user switches tabs doesn't clobber the new selection with a
   * stale fallback shift.
   */
  getActiveTab: () => string | undefined;
  sessionId: string | null;
  terminals: Terminal[];
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
};

export function useRemoveTerminal({
  getActiveTab,
  sessionId,
  terminals,
  setTerminals,
  setRightPanelActiveTab,
}: RemoveTerminalOpts) {
  const terminalsRef = useRef(terminals);
  terminalsRef.current = terminals;

  return useCallback(
    (id: string) => {
      const currentTerminals = terminalsRef.current;
      const indexToRemove = currentTerminals.findIndex((terminal) => terminal.id === id);
      if (indexToRemove === -1) return;

      const activeTab = getActiveTab();
      const nextTerminals = currentTerminals.filter((_, index) => index !== indexToRemove);
      terminalsRef.current = nextTerminals;
      setTerminals((previous) => previous.filter((terminal) => terminal.id !== id));

      if (activeTab === id && sessionId) {
        const next = indexToRemove > 0 ? currentTerminals[indexToRemove - 1] : nextTerminals[0];
        if (next) setRightPanelActiveTab(sessionId, next.id);
      }
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
  destroyTerminal: (terminalId: string) => Promise<boolean>;
};

/** X-button close. Removes local UI immediately, then destroys the shell in the background. */
function useCloseTab({ destroyTerminal }: CloseTabOpts) {
  return useCallback(
    (event: MouseEvent, terminalId: string) => {
      event.preventDefault();
      event.stopPropagation();
      void destroyTerminal(terminalId);
    },
    [destroyTerminal],
  );
}

type ManagedTerminalActionsOpts = {
  environmentId: string | null;
  taskID: string | null;
  updateUserShell: (
    scope: UserShellScope,
    terminalId: string,
    patch: { customName?: string | null; state?: "open" | "parked" },
  ) => void;
};

function useManagedTerminalActions({
  environmentId,
  taskID,
  updateUserShell,
}: ManagedTerminalActionsOpts) {
  const renameTerminal = useCallback(
    async (id: string, name: string | null) => {
      const scope = makeUserShellScope(environmentId, taskID);
      if (!scope) return;
      const trimmed = name === null ? null : name.trim();
      const normalized = trimmed === "" ? null : trimmed;
      try {
        await renameUserShell(id, normalized, taskID ?? undefined);
        updateUserShell(scope, id, { customName: normalized });
      } catch (error) {
        console.error("Failed to rename terminal:", error);
      }
    },
    [environmentId, taskID, updateUserShell],
  );

  const resumeTerminal = useCallback(
    async (id: string) => {
      const scope = makeUserShellScope(environmentId, taskID);
      if (!scope) return;
      try {
        await resumeUserShell(id, taskID ?? undefined);
        updateUserShell(scope, id, { state: "open" });
      } catch (error) {
        console.error("Failed to resume terminal:", error);
      }
    },
    [environmentId, taskID, updateUserShell],
  );

  return { renameTerminal, resumeTerminal };
}

type TerminalActionsOptions = {
  sessionId: string | null;
  environmentId: string | null;
  terminals: Terminal[];
  devProcessId: string | undefined;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
  setPreviewOpen: (sessionId: string, open: boolean) => void;
  setPreviewStage: (sessionId: string, stage: PreviewStage) => void;
};

type ScriptTerminalOptions = {
  sessionId: string | null;
  environmentId: string | null;
  taskID: string | null;
  setTerminals: Dispatch<SetStateAction<Terminal[]>>;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
  storeApi: ReturnType<typeof useAppStoreApi>;
};

/** Creates an environment-shared script terminal and opens its tab. */
function useScriptTerminalCreator({
  sessionId,
  environmentId,
  taskID,
  setTerminals,
  setRightPanelActiveTab,
  storeApi,
}: ScriptTerminalOptions) {
  return useCallback(
    async (script: RepositoryScript) => {
      if (!environmentId) return;
      const scope = makeUserShellScope(environmentId, taskID);
      if (!scope) return;
      const owner = captureUserShellPublicationOwner(storeApi.getState(), scope);
      try {
        const result = await createUserShell(environmentId, { scriptId: script.id });
        const newTerm: Terminal = {
          id: result.terminalId,
          type: "script",
          label: result.label ?? script.name ?? t("common:script"),
          closable: true,
          kind: "script",
        };
        storeApi.getState().addUserShell(scope, {
          ...result,
          kind: "script",
          label: newTerm.label,
          closable: true,
          running: result.ptyStatus === "running",
        });
        if (!isActiveUserShellPublicationOwner(storeApi.getState(), owner)) return;
        setTerminals((prev) => appendTerminalIfMissing(prev, newTerm));
        if (sessionId) setRightPanelActiveTab(sessionId, result.terminalId);
      } catch (error) {
        console.error("Failed to create script terminal:", error);
      }
    },
    [environmentId, sessionId, setRightPanelActiveTab, setTerminals, storeApi, taskID],
  );
}

function useTerminalActions({
  sessionId,
  environmentId,
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

  // getActiveTab reads the live store at call-time rather than capturing
  // `activeTab` in the useCallback closure. Async close handlers that
  // resolve after a user tab-switch see the new selection immediately
  // — no useEffect/useLayoutEffect race window because the store is the
  // single source of truth and `getState()` is synchronous.
  const storeApi = useAppStoreApi();
  const getActiveTab = useCallback(
    () => (sessionId ? storeApi.getState().rightPanel.activeTabBySessionId[sessionId] : undefined),
    [storeApi, sessionId],
  );

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
    terminals,
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

  const handleRunCommand = useScriptTerminalCreator({
    sessionId,
    environmentId,
    taskID,
    setTerminals,
    setRightPanelActiveTab,
    storeApi,
  });

  const handleTerminalDestroyed = useCallback(
    (terminalId: string) => {
      const scope = makeUserShellScope(environmentId, taskID);
      if (!scope) return;
      removeUserShellStore(scope, terminalId);
      removeTerminal(terminalId);
    },
    [environmentId, removeTerminal, removeUserShellStore, taskID],
  );
  const { destroyTerminal } = useTerminalDestroy({
    environmentId,
    taskId: taskID,
    onDestroyed: handleTerminalDestroyed,
  });
  const handleCloseTab = useCloseTab({ destroyTerminal });

  const { renameTerminal, resumeTerminal } = useManagedTerminalActions({
    environmentId,
    taskID,
    updateUserShell,
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
  // Read the active task before the terminal sync so a task change can clear
  // the previous task's ordinary terminals immediately.
  const activeTaskId = useAppStore((state) => state.tasks?.activeTaskId ?? null);
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
  const {
    shells: userShells,
    isLoaded: userShellsLoaded,
    hasSuccessfulList,
  } = useUserShells(environmentId, activeTaskId);

  const tabRestoredRef = useTerminalSync({
    environmentId,
    taskId: activeTaskId,
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
    terminals,
    devProcessId,
    setTerminals,
    setRightPanelActiveTab,
    setPreviewOpen,
    setPreviewStage,
  });

  const parkedTerminals = buildParkedTerminals(userShells, hasSuccessfulList);

  const savedTabFromStorage = sessionId
    ? getSessionStorage<string | null>(`rightPanel-tab-${sessionId}`, null)
    : null;
  const terminalTabValue = computeTerminalTabValue(
    activeTab,
    sessionJustChanged,
    savedTabFromStorage,
    terminals,
    !!(savedTabFromStorage && terminals.some((t) => t.id === savedTabFromStorage)),
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
