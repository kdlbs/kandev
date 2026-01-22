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
  contextWindow: { bySessionId: {} },
  agents: { agents: [] },
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
});
