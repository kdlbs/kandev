export type TerminalState = {
  terminals: Array<{ id: string; output: string[] }>;
};

export type ShellState = {
  // Map of sessionId to shell output buffer (raw bytes as string)
  outputs: Record<string, string>;
  // Map of sessionId to shell status
  statuses: Record<
    string,
    {
      available: boolean;
      running?: boolean;
      shell?: string;
      cwd?: string;
    }
  >;
};

export type ProcessStatusEntry = {
  processId: string;
  sessionId: string;
  kind: string;
  scriptName?: string;
  status: string;
  command?: string;
  workingDir?: string;
  exitCode?: number | null;
  startedAt?: string;
  updatedAt?: string;
};

export type ProcessState = {
  outputsByProcessId: Record<string, string>;
  processesById: Record<string, ProcessStatusEntry>;
  processIdsBySessionId: Record<string, string[]>;
  activeProcessBySessionId: Record<string, string>;
  devProcessBySessionId: Record<string, string>;
};

export type FileInfo = {
  path: string;
  status: 'modified' | 'added' | 'deleted' | 'untracked' | 'renamed';
  staged: boolean;
  additions?: number;
  deletions?: number;
  old_path?: string;
  diff?: string;
};

export type GitStatusEntry = {
  branch: string | null;
  remote_branch: string | null;
  modified: string[];
  added: string[];
  deleted: string[];
  untracked: string[];
  renamed: string[];
  ahead: number;
  behind: number;
  files: Record<string, FileInfo>;
  timestamp: string | null;
};

export type GitStatusState = {
  bySessionId: Record<string, GitStatusEntry>;
};

export type ContextWindowEntry = {
  size: number;
  used: number;
  remaining: number;
  efficiency: number;
  timestamp?: string;
};

export type ContextWindowState = {
  bySessionId: Record<string, ContextWindowEntry>;
};

export type AgentState = {
  agents: Array<{ id: string; status: 'idle' | 'running' | 'error' }>;
};

export type SessionRuntimeSliceState = {
  terminal: TerminalState;
  shell: ShellState;
  processes: ProcessState;
  gitStatus: GitStatusState;
  contextWindow: ContextWindowState;
  agents: AgentState;
};

export type SessionRuntimeSliceActions = {
  setTerminalOutput: (terminalId: string, data: string) => void;
  appendShellOutput: (sessionId: string, data: string) => void;
  setShellStatus: (
    sessionId: string,
    status: { available: boolean; running?: boolean; shell?: string; cwd?: string }
  ) => void;
  clearShellOutput: (sessionId: string) => void;
  appendProcessOutput: (processId: string, data: string) => void;
  upsertProcessStatus: (status: ProcessStatusEntry) => void;
  clearProcessOutput: (processId: string) => void;
  setActiveProcess: (sessionId: string, processId: string) => void;
  setGitStatus: (sessionId: string, gitStatus: GitStatusEntry) => void;
  clearGitStatus: (sessionId: string) => void;
  setContextWindow: (sessionId: string, contextWindow: ContextWindowEntry) => void;
};

export type SessionRuntimeSlice = SessionRuntimeSliceState & SessionRuntimeSliceActions;
