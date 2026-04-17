import type { StateCreator } from "zustand";
import type {
  SessionRuntimeSlice,
  SessionRuntimeSliceState,
  SessionPollMode,
  GitStatusEntry,
  FileInfo,
} from "./types";

const maxProcessOutputBytes = 2 * 1024 * 1024;

/** Compute total additions/deletions across all files. */
function computeFileStats(files: Record<string, FileInfo> | undefined): {
  additions: number;
  deletions: number;
} {
  if (!files) return { additions: 0, deletions: 0 };
  let additions = 0;
  let deletions = 0;
  for (const f of Object.values(files)) {
    additions += f.additions || 0;
    deletions += f.deletions || 0;
  }
  return { additions, deletions };
}

/** Check if any file's staged status differs between two git statuses. */
function hasStagedDifference(
  existingFiles: Record<string, FileInfo> | undefined,
  newFiles: Record<string, FileInfo> | undefined,
): boolean {
  if (!existingFiles || !newFiles) return existingFiles !== newFiles;
  for (const key of Object.keys(newFiles)) {
    if (existingFiles[key]?.staged !== newFiles[key]?.staged) return true;
  }
  return false;
}

/** Compare two git status entries to determine if a meaningful change occurred. */
function hasGitStatusChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  // Timestamp change means the backend detected a real change — always accept.
  if (existing.timestamp !== incoming.timestamp) return true;

  if (existing.branch !== incoming.branch || existing.remote_branch !== incoming.remote_branch)
    return true;
  if (existing.ahead !== incoming.ahead || existing.behind !== incoming.behind) return true;

  const existingFileKeys = existing.files ? Object.keys(existing.files).sort().join(",") : "";
  const newFileKeys = incoming.files ? Object.keys(incoming.files).sort().join(",") : "";
  if (existingFileKeys !== newFileKeys) return true;

  const existingTotal = computeFileStats(existing.files);
  const newTotal = computeFileStats(incoming.files);
  if (
    existingTotal.additions !== newTotal.additions ||
    existingTotal.deletions !== newTotal.deletions
  )
    return true;

  return hasStagedDifference(existing.files, incoming.files);
}

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
  gitStatus: { byEnvironmentId: {} },
  environmentIdBySessionId: {},
  sessionCommits: { byEnvironmentId: {}, loading: {} },
  contextWindow: { bySessionId: {} },
  agents: { agents: [] },
  availableCommands: { bySessionId: {} },
  sessionMode: { bySessionId: {} },
  agentCapabilities: { bySessionId: {} },
  sessionModels: { bySessionId: {} },
  promptUsage: { bySessionId: {} },
  sessionTodos: { bySessionId: {} },
  userShells: { byEnvironmentId: {}, loading: {}, loaded: {} },
  prepareProgress: { bySessionId: {} },
  sessionPollMode: { bySessionId: {} },
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

function buildUserShellActions(set: ImmerSet) {
  return {
    setUserShells: (
      sessionId: string,
      shells: Parameters<SessionRuntimeSlice["setUserShells"]>[1],
    ) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.userShells.byEnvironmentId[envKey] = shells;
        draft.userShells.loaded[envKey] = true;
        draft.userShells.loading[envKey] = false;
      }),
    setUserShellsLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        draft.userShells.loading[envKey] = loading;
      }),
    addUserShell: (sessionId: string, shell: Parameters<SessionRuntimeSlice["addUserShell"]>[1]) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const existing = draft.userShells.byEnvironmentId[envKey] || [];
        if (!existing.some((s) => s.terminalId === shell.terminalId)) {
          draft.userShells.byEnvironmentId[envKey] = [...existing, shell];
        }
      }),
    removeUserShell: (sessionId: string, terminalId: string) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const existing = draft.userShells.byEnvironmentId[envKey] || [];
        draft.userShells.byEnvironmentId[envKey] = existing.filter(
          (s) => s.terminalId !== terminalId,
        );
      }),
    setSessionPollMode: (sessionId: string, mode: SessionPollMode) =>
      set((draft) => {
        draft.sessionPollMode.bySessionId[sessionId] = mode;
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
  migrate(draft.gitStatus.byEnvironmentId);
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
  setGitStatus: (sessionId, gitStatus) =>
    set((draft) => {
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      const existing = draft.gitStatus.byEnvironmentId[envKey];
      if (existing && !hasGitStatusChanged(existing, gitStatus)) return;
      draft.gitStatus.byEnvironmentId[envKey] = gitStatus;
    }),
  clearGitStatus: (sessionId) =>
    set((draft) => {
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      delete draft.gitStatus.byEnvironmentId[envKey];
    }),
  registerSessionEnvironment: (sessionId, environmentId) =>
    set((draft) => {
      draft.environmentIdBySessionId[sessionId] = environmentId;
      migrateEnvKeyedData(draft, sessionId, environmentId);
    }),
  setContextWindow: (sessionId, contextWindow) =>
    set((draft) => {
      draft.contextWindow.bySessionId[sessionId] = contextWindow;
    }),
  setSessionCommits: (sessionId, commits) =>
    set((draft) => {
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      draft.sessionCommits.byEnvironmentId[envKey] = commits;
    }),
  setSessionCommitsLoading: (sessionId, loading) =>
    set((draft) => {
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      draft.sessionCommits.loading[envKey] = loading;
    }),
  addSessionCommit: (sessionId, commit) =>
    set((draft) => {
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      const existing = draft.sessionCommits.byEnvironmentId[envKey] || [];
      // For amend: only replace HEAD (first entry) if it has the same parent
      if (existing.length > 0 && existing[0].parent_sha === commit.parent_sha) {
        // Replace HEAD commit (this is an amend)
        existing[0] = commit;
        draft.sessionCommits.byEnvironmentId[envKey] = existing;
      } else {
        // Normal commit: prepend to list
        draft.sessionCommits.byEnvironmentId[envKey] = [commit, ...existing];
      }
    }),
  clearSessionCommits: (sessionId) =>
    set((draft) => {
      const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
      delete draft.sessionCommits.byEnvironmentId[envKey];
    }),
  setAvailableCommands: (sessionId, commands) =>
    set((draft) => {
      draft.availableCommands.bySessionId[sessionId] = commands;
    }),
  clearAvailableCommands: (sessionId) =>
    set((draft) => {
      delete draft.availableCommands.bySessionId[sessionId];
    }),
  setSessionMode: (sessionId, modeId, availableModes) =>
    set((draft) => {
      const existing = draft.sessionMode.bySessionId[sessionId];
      draft.sessionMode.bySessionId[sessionId] = {
        currentModeId: modeId,
        availableModes: availableModes ?? existing?.availableModes ?? [],
      };
    }),
  clearSessionMode: (sessionId) =>
    set((draft) => {
      delete draft.sessionMode.bySessionId[sessionId];
    }),
  setAgentCapabilities: (sessionId, caps) =>
    set((draft) => {
      draft.agentCapabilities.bySessionId[sessionId] = caps;
    }),
  setSessionModels: (sessionId, data) =>
    set((draft) => {
      console.log("[store] setSessionModels", {
        sessionId,
        modelsCount: data.models?.length ?? 0,
        models: data.models?.map((m) => m.modelId),
        currentModelId: data.currentModelId,
        configOptionsCount: data.configOptions?.length ?? 0,
        configOptions: data.configOptions?.map((o) => ({ id: o.id, category: o.category })),
      });
      draft.sessionModels.bySessionId[sessionId] = data;
    }),
  setPromptUsage: (sessionId, usage) =>
    set((draft) => {
      draft.promptUsage.bySessionId[sessionId] = usage;
    }),
  setSessionTodos: (sessionId, entries) =>
    set((draft) => {
      draft.sessionTodos.bySessionId[sessionId] = entries;
    }),
  ...buildUserShellActions(set),
});
