import type { StateCreator } from "zustand";
import type { SessionRuntimeSlice, SessionRuntimeSliceState } from "./types";

const maxProcessOutputBytes = 2 * 1024 * 1024;

function trimProcessOutput(value: string) {
  if (value.length <= maxProcessOutputBytes) {
    return value;
  }
  return value.slice(value.length - maxProcessOutputBytes);
}

export const defaultSessionRuntimeState: SessionRuntimeSliceState = {
  terminal: { terminals: [] },
  shell: { outputs: {}, statuses: {} },
  processes: {
    outputsByProcessId: {},
    processesById: {},
    processIdsBySessionId: {},
    activeProcessBySessionId: {},
    devProcessBySessionId: {},
  },
  environmentIdBySessionId: {},
  sessionCommits: { byEnvironmentId: {}, loading: {}, refetchTrigger: {} },
  contextWindow: { bySessionId: {} },
  agents: { agents: [] },
  availableCommands: { bySessionId: {} },
  agentCapabilities: { bySessionId: {} },
  promptUsage: { bySessionId: {} },
  userShells: { byEnvironmentId: {}, loading: {}, loaded: {} },
};

type ImmerSet = Parameters<typeof createSessionRuntimeSlice>[0];

function buildTerminalShellProcessActions(set: ImmerSet) {
  return {
    setTerminalOutput: (terminalId: string, data: string) =>
      set((draft) => {
        const existing = draft.terminal.terminals.find((terminal) => terminal.id === terminalId);
        if (existing) {
          existing.output.push(data);
        } else {
          draft.terminal.terminals.push({ id: terminalId, output: [data] });
        }
      }),
    appendShellOutput: (sessionId: string, data: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.shell.outputs[envKey] = (draft.shell.outputs[envKey] || "") + data;
      }),
    setShellStatus: (
      sessionId: string,
      status: { available: boolean; running?: boolean; shell?: string; cwd?: string },
    ) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.shell.statuses[envKey] = status;
      }),
    clearShellOutput: (sessionId: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.shell.outputs[envKey] = "";
      }),
    appendProcessOutput: (processId: string, data: string) =>
      set((draft) => {
        const next = (draft.processes.outputsByProcessId[processId] || "") + data;
        draft.processes.outputsByProcessId[processId] = trimProcessOutput(next);
      }),
    upsertProcessStatus: (status: Parameters<SessionRuntimeSlice["upsertProcessStatus"]>[0]) =>
      set((draft) => {
        draft.processes.processesById[status.processId] = status;
        const list = draft.processes.processIdsBySessionId[status.sessionId] || [];
        if (!list.includes(status.processId)) {
          draft.processes.processIdsBySessionId[status.sessionId] = [...list, status.processId];
        }
        if (status.kind === "dev") {
          draft.processes.devProcessBySessionId[status.sessionId] = status.processId;
        }
      }),
    clearProcessOutput: (processId: string) =>
      set((draft) => {
        draft.processes.outputsByProcessId[processId] = "";
      }),
    setActiveProcess: (sessionId: string, processId: string) =>
      set((draft) => {
        draft.processes.activeProcessBySessionId[sessionId] = processId;
      }),
  };
}

function buildSessionCommitActions(set: ImmerSet) {
  return {
    setSessionCommits: (
      sessionId: string,
      commits: Parameters<SessionRuntimeSlice["setSessionCommits"]>[1],
      opts?: { allowEmpty?: boolean },
    ) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const existing = draft.sessionCommits.byEnvironmentId[envKey];
        // Default guard: prevent a stale empty-array response from overwriting
        // commits that arrived via incremental notifications while the request
        // was in flight (race between fetch start and commit_created events).
        //
        // Under stale-while-revalidate, a `commits_reset` or `branch_switched`
        // refetch can *legitimately* return [] — the backend actually has no
        // commits. The caller must opt in to that path with `allowEmpty: true`
        // so the panel stops showing the pre-reset list.
        if (!opts?.allowEmpty && commits.length === 0 && existing && existing.length > 0) {
          return;
        }
        draft.sessionCommits.byEnvironmentId[envKey] = commits;
      }),
    setSessionCommitsLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.sessionCommits.loading[envKey] = loading;
      }),
    addSessionCommit: (
      sessionId: string,
      commit: Parameters<SessionRuntimeSlice["addSessionCommit"]>[1],
    ) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const existing = draft.sessionCommits.byEnvironmentId[envKey] || [];
        // For amend: only replace HEAD (first entry) if it has the same parent
        if (existing.length > 0 && existing[0].parent_sha === commit.parent_sha) {
          existing[0] = commit;
          draft.sessionCommits.byEnvironmentId[envKey] = existing;
        } else {
          draft.sessionCommits.byEnvironmentId[envKey] = [commit, ...existing];
        }
      }),
    clearSessionCommits: (sessionId: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        delete draft.sessionCommits.byEnvironmentId[envKey];
      }),
    bumpSessionCommitsRefetch: (sessionId: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const prev = draft.sessionCommits.refetchTrigger[envKey] ?? 0;
        draft.sessionCommits.refetchTrigger[envKey] = prev + 1;
      }),
  };
}

function buildUserShellActions(set: ImmerSet) {
  return {
    setUserShells: (
      environmentId: string,
      shells: Parameters<SessionRuntimeSlice["setUserShells"]>[1],
    ) =>
      set((draft) => {
        if (!environmentId) return;
        draft.userShells.byEnvironmentId[environmentId] = shells;
        draft.userShells.loaded[environmentId] = true;
        draft.userShells.loading[environmentId] = false;
      }),
    setUserShellsLoading: (environmentId: string, loading: boolean) =>
      set((draft) => {
        if (!environmentId) return;
        draft.userShells.loading[environmentId] = loading;
      }),
    addUserShell: (
      environmentId: string,
      shell: Parameters<SessionRuntimeSlice["addUserShell"]>[1],
    ) =>
      set((draft) => {
        if (!environmentId) return;
        const existing = draft.userShells.byEnvironmentId[environmentId] || [];
        if (!existing.some((s) => s.terminalId === shell.terminalId)) {
          draft.userShells.byEnvironmentId[environmentId] = [...existing, shell];
        }
      }),
    removeUserShell: (environmentId: string, terminalId: string) =>
      set((draft) => {
        if (!environmentId) return;
        const existing = draft.userShells.byEnvironmentId[environmentId] || [];
        draft.userShells.byEnvironmentId[environmentId] = existing.filter(
          (s) => s.terminalId !== terminalId,
        );
      }),
    updateUserShell: (
      environmentId: string,
      terminalId: string,
      patch: Parameters<SessionRuntimeSlice["updateUserShell"]>[2],
    ) =>
      set((draft) => {
        if (!environmentId) return;
        const existing = draft.userShells.byEnvironmentId[environmentId];
        if (!existing) return;
        draft.userShells.byEnvironmentId[environmentId] = existing.map((s) =>
          s.terminalId === terminalId ? { ...s, ...patch } : s,
        );
      }),
  };
}

/**
 * Migrate any env-keyed data stored under the fallback `sessionId` key to the
 * proper `environmentId` key so selectors don't see stale data after the
 * session→environment mapping is registered.
 */
export function migrateEnvKeyedData(
  draft: SessionRuntimeSliceState,
  sessionId: string,
  environmentId: string,
) {
  if (sessionId === environmentId) return;
  const migrate = <T>(store: Record<string, T>) => {
    if (sessionId in store) {
      if (!(environmentId in store)) {
        store[environmentId] = store[sessionId];
      }
      delete store[sessionId];
    }
  };
  migrate(draft.sessionCommits.byEnvironmentId);
  migrate(draft.sessionCommits.loading);
  migrate(draft.sessionCommits.refetchTrigger);
  migrate(draft.shell.outputs);
  migrate(draft.shell.statuses);
  migrate(draft.userShells.byEnvironmentId);
  migrate(draft.userShells.loading);
  migrate(draft.userShells.loaded);
}

export const createSessionRuntimeSlice: StateCreator<
  SessionRuntimeSlice,
  [["zustand/immer", never]],
  [],
  SessionRuntimeSlice
> = (set) => ({
  ...defaultSessionRuntimeState,
  ...buildTerminalShellProcessActions(set),
  registerSessionEnvironment: (sessionId, environmentId) =>
    set((draft) => {
      draft.environmentIdBySessionId[sessionId] = environmentId;
      migrateEnvKeyedData(draft, sessionId, environmentId);
    }),
  setContextWindow: (sessionId, contextWindow) =>
    set((draft) => {
      draft.contextWindow.bySessionId[sessionId] = contextWindow;
    }),
  ...buildSessionCommitActions(set),
  setAvailableCommands: (sessionId, commands) =>
    set((draft) => {
      draft.availableCommands.bySessionId[sessionId] = commands;
    }),
  clearAvailableCommands: (sessionId) =>
    set((draft) => {
      delete draft.availableCommands.bySessionId[sessionId];
    }),
  setAgentCapabilities: (sessionId, caps) =>
    set((draft) => {
      draft.agentCapabilities.bySessionId[sessionId] = caps;
    }),
  setPromptUsage: (sessionId, usage) =>
    set((draft) => {
      draft.promptUsage.bySessionId[sessionId] = usage;
    }),
  ...buildUserShellActions(set),
});
