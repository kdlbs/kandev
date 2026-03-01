import type { StateCreator } from "zustand";
import type {
  SessionRuntimeSlice,
  SessionRuntimeSliceState,
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
  gitStatus: { bySessionId: {} },
  gitSnapshots: { bySessionId: {}, latestBySessionId: {}, loading: {} },
  sessionCommits: { bySessionId: {}, loading: {} },
  contextWindow: { bySessionId: {} },
  agents: { agents: [] },
  availableCommands: { bySessionId: {} },
  sessionMode: { bySessionId: {} },
  userShells: { bySessionId: {}, loading: {}, loaded: {} },
  prepareProgress: { bySessionId: {} },
};

type ImmerSet = Parameters<typeof createSessionRuntimeSlice>[0];

function buildGitSnapshotActions(set: ImmerSet) {
  return {
    setGitSnapshots: (
      sessionId: string,
      snapshots: Parameters<SessionRuntimeSlice["setGitSnapshots"]>[1],
    ) =>
      set((draft) => {
        draft.gitSnapshots.bySessionId[sessionId] = snapshots;
        draft.gitSnapshots.latestBySessionId[sessionId] =
          snapshots.length > 0 ? snapshots[0] : null;
      }),
    setGitSnapshotsLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        draft.gitSnapshots.loading[sessionId] = loading;
      }),
    addGitSnapshot: (
      sessionId: string,
      snapshot: Parameters<SessionRuntimeSlice["addGitSnapshot"]>[1],
    ) =>
      set((draft) => {
        const existing = draft.gitSnapshots.bySessionId[sessionId] || [];
        draft.gitSnapshots.bySessionId[sessionId] = [snapshot, ...existing];
        draft.gitSnapshots.latestBySessionId[sessionId] = snapshot;
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
        draft.userShells.bySessionId[sessionId] = shells;
        draft.userShells.loaded[sessionId] = true;
        draft.userShells.loading[sessionId] = false;
      }),
    setUserShellsLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        draft.userShells.loading[sessionId] = loading;
      }),
    addUserShell: (sessionId: string, shell: Parameters<SessionRuntimeSlice["addUserShell"]>[1]) =>
      set((draft) => {
        const existing = draft.userShells.bySessionId[sessionId] || [];
        if (!existing.some((s) => s.terminalId === shell.terminalId)) {
          draft.userShells.bySessionId[sessionId] = [...existing, shell];
        }
      }),
    removeUserShell: (sessionId: string, terminalId: string) =>
      set((draft) => {
        const existing = draft.userShells.bySessionId[sessionId] || [];
        draft.userShells.bySessionId[sessionId] = existing.filter(
          (s) => s.terminalId !== terminalId,
        );
      }),
  };
}

export const createSessionRuntimeSlice: StateCreator<
  SessionRuntimeSlice,
  [["zustand/immer", never]],
  [],
  SessionRuntimeSlice
> = (set) => ({
  ...defaultSessionRuntimeState,
  setTerminalOutput: (terminalId, data) =>
    set((draft) => {
      const existing = draft.terminal.terminals.find((terminal) => terminal.id === terminalId);
      if (existing) {
        existing.output.push(data);
      } else {
        draft.terminal.terminals.push({ id: terminalId, output: [data] });
      }
    }),
  appendShellOutput: (sessionId, data) =>
    set((draft) => {
      draft.shell.outputs[sessionId] = (draft.shell.outputs[sessionId] || "") + data;
    }),
  setShellStatus: (sessionId, status) =>
    set((draft) => {
      draft.shell.statuses[sessionId] = status;
    }),
  clearShellOutput: (sessionId) =>
    set((draft) => {
      draft.shell.outputs[sessionId] = "";
    }),
  appendProcessOutput: (processId, data) =>
    set((draft) => {
      const next = (draft.processes.outputsByProcessId[processId] || "") + data;
      draft.processes.outputsByProcessId[processId] = trimProcessOutput(next);
    }),
  upsertProcessStatus: (status) =>
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
  clearProcessOutput: (processId) =>
    set((draft) => {
      draft.processes.outputsByProcessId[processId] = "";
    }),
  setActiveProcess: (sessionId, processId) =>
    set((draft) => {
      draft.processes.activeProcessBySessionId[sessionId] = processId;
    }),
  setGitStatus: (sessionId, gitStatus) =>
    set((draft) => {
      const existing = draft.gitStatus.bySessionId[sessionId];
      if (existing && !hasGitStatusChanged(existing, gitStatus)) return;
      draft.gitStatus.bySessionId[sessionId] = gitStatus;
    }),
  clearGitStatus: (sessionId) =>
    set((draft) => {
      delete draft.gitStatus.bySessionId[sessionId];
    }),
  setContextWindow: (sessionId, contextWindow) =>
    set((draft) => {
      draft.contextWindow.bySessionId[sessionId] = contextWindow;
    }),
  ...buildGitSnapshotActions(set),
  setSessionCommits: (sessionId, commits) =>
    set((draft) => {
      draft.sessionCommits.bySessionId[sessionId] = commits;
    }),
  setSessionCommitsLoading: (sessionId, loading) =>
    set((draft) => {
      draft.sessionCommits.loading[sessionId] = loading;
    }),
  addSessionCommit: (sessionId, commit) =>
    set((draft) => {
      const existing = draft.sessionCommits.bySessionId[sessionId] || [];
      // For amend: if a commit with the same parent already exists, replace it
      const parentMatch = existing.findIndex((c) => c.parent_sha === commit.parent_sha);
      if (parentMatch !== -1) {
        // Replace the commit with matching parent (this is an amend)
        existing[parentMatch] = commit;
        draft.sessionCommits.bySessionId[sessionId] = existing;
      } else {
        // Normal commit: prepend to list
        draft.sessionCommits.bySessionId[sessionId] = [commit, ...existing];
      }
    }),
  clearSessionCommits: (sessionId) =>
    set((draft) => {
      delete draft.sessionCommits.bySessionId[sessionId];
    }),
  setAvailableCommands: (sessionId, commands) =>
    set((draft) => {
      draft.availableCommands.bySessionId[sessionId] = commands;
    }),
  clearAvailableCommands: (sessionId) =>
    set((draft) => {
      delete draft.availableCommands.bySessionId[sessionId];
    }),
  setSessionMode: (sessionId, modeId) =>
    set((draft) => {
      draft.sessionMode.bySessionId[sessionId] = modeId;
    }),
  ...buildUserShellActions(set),
});
