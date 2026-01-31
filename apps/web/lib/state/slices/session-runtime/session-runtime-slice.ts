import type { StateCreator } from 'zustand';
import type { SessionRuntimeSlice, SessionRuntimeSliceState } from './types';

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
  gitStatus: { bySessionId: {} },
  gitSnapshots: { bySessionId: {}, latestBySessionId: {}, loading: {} },
  sessionCommits: { bySessionId: {}, loading: {} },
  contextWindow: { bySessionId: {} },
  agents: { agents: [] },
  availableCommands: { bySessionId: {} },
};

export const createSessionRuntimeSlice: StateCreator<
  SessionRuntimeSlice,
  [['zustand/immer', never]],
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
      draft.shell.outputs[sessionId] = (draft.shell.outputs[sessionId] || '') + data;
    }),
  setShellStatus: (sessionId, status) =>
    set((draft) => {
      draft.shell.statuses[sessionId] = status;
    }),
  clearShellOutput: (sessionId) =>
    set((draft) => {
      draft.shell.outputs[sessionId] = '';
    }),
  appendProcessOutput: (processId, data) =>
    set((draft) => {
      const next = (draft.processes.outputsByProcessId[processId] || '') + data;
      draft.processes.outputsByProcessId[processId] = trimProcessOutput(next);
    }),
  upsertProcessStatus: (status) =>
    set((draft) => {
      draft.processes.processesById[status.processId] = status;
      const list = draft.processes.processIdsBySessionId[status.sessionId] || [];
      if (!list.includes(status.processId)) {
        draft.processes.processIdsBySessionId[status.sessionId] = [...list, status.processId];
      }
      // Track dev processes (kind === 'dev')
      if (status.kind === 'dev') {
        draft.processes.devProcessBySessionId[status.sessionId] = status.processId;
      }
    }),
  clearProcessOutput: (processId) =>
    set((draft) => {
      draft.processes.outputsByProcessId[processId] = '';
    }),
  setActiveProcess: (sessionId, processId) =>
    set((draft) => {
      draft.processes.activeProcessBySessionId[sessionId] = processId;
    }),
  setGitStatus: (sessionId, gitStatus) =>
    set((draft) => {
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
  // Git snapshot actions
  setGitSnapshots: (sessionId, snapshots) =>
    set((draft) => {
      draft.gitSnapshots.bySessionId[sessionId] = snapshots;
      draft.gitSnapshots.latestBySessionId[sessionId] = snapshots.length > 0 ? snapshots[0] : null;
    }),
  setGitSnapshotsLoading: (sessionId, loading) =>
    set((draft) => {
      draft.gitSnapshots.loading[sessionId] = loading;
    }),
  addGitSnapshot: (sessionId, snapshot) =>
    set((draft) => {
      const existing = draft.gitSnapshots.bySessionId[sessionId] || [];
      // Add to front (newest first)
      draft.gitSnapshots.bySessionId[sessionId] = [snapshot, ...existing];
      draft.gitSnapshots.latestBySessionId[sessionId] = snapshot;
    }),
  // Session commit actions
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
      // Add to front (newest first)
      draft.sessionCommits.bySessionId[sessionId] = [commit, ...existing];
    }),
  clearSessionCommits: (sessionId) =>
    set((draft) => {
      // Delete the entry so hook will refetch (undefined triggers fetch, [] does not)
      delete draft.sessionCommits.bySessionId[sessionId];
    }),
  // Available commands actions
  setAvailableCommands: (sessionId, commands) =>
    set((draft) => {
      draft.availableCommands.bySessionId[sessionId] = commands;
    }),
  clearAvailableCommands: (sessionId) =>
    set((draft) => {
      delete draft.availableCommands.bySessionId[sessionId];
    }),
});
