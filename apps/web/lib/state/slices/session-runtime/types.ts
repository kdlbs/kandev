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
  status: "modified" | "added" | "deleted" | "untracked" | "renamed";
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

// Git Snapshot types for historical tracking
export type SnapshotType =
  | "status_update"
  | "pre_commit"
  | "post_commit"
  | "pre_stage"
  | "post_stage";

export type GitSnapshot = {
  id: string;
  session_id: string;
  snapshot_type: SnapshotType;
  branch: string;
  remote_branch: string;
  head_commit: string;
  base_commit: string;
  ahead: number;
  behind: number;
  files: Record<string, FileInfo>;
  triggered_by: string;
  metadata?: Record<string, unknown>;
  created_at: string;
};

export type SessionCommit = {
  id: string;
  session_id: string;
  commit_sha: string;
  parent_sha: string;
  author_name: string;
  author_email: string;
  commit_message: string;
  committed_at: string;
  pre_commit_snapshot_id?: string;
  post_commit_snapshot_id?: string;
  files_changed: number;
  insertions: number;
  deletions: number;
  created_at: string;
};

export type CumulativeDiff = {
  session_id: string;
  base_commit: string;
  head_commit: string;
  total_commits: number;
  files: Record<string, FileInfo>;
};

export type GitSnapshotsState = {
  bySessionId: Record<string, GitSnapshot[]>;
  latestBySessionId: Record<string, GitSnapshot | null>;
  loading: Record<string, boolean>;
};

export type SessionCommitsState = {
  bySessionId: Record<string, SessionCommit[]>;
  loading: Record<string, boolean>;
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
  agents: Array<{ id: string; status: "idle" | "running" | "error" }>;
};

export type AvailableCommand = {
  name: string;
  description?: string;
  input_hint?: string;
};

export type AvailableCommandsState = {
  bySessionId: Record<string, AvailableCommand[]>;
};

export type SessionModeState = {
  bySessionId: Record<string, string>;
};

export type UserShellInfo = {
  terminalId: string;
  processId: string;
  running: boolean;
  label: string; // Display name (e.g., "Terminal" or script name)
  closable: boolean; // Whether the terminal can be closed (first terminal is not closable)
  initialCommand?: string; // Command that was run (empty for plain shells)
};

export type UserShellsState = {
  bySessionId: Record<string, UserShellInfo[]>;
  loading: Record<string, boolean>;
  loaded: Record<string, boolean>;
};

export type PrepareStepInfo = {
  name: string;
  status: string;
  output?: string;
  error?: string;
};

export type SessionPrepareState = {
  sessionId: string;
  status: string;
  steps: PrepareStepInfo[];
  errorMessage?: string;
  durationMs?: number;
};

export type PrepareProgressState = {
  bySessionId: Record<string, SessionPrepareState>;
};

export type SessionRuntimeSliceState = {
  terminal: TerminalState;
  shell: ShellState;
  processes: ProcessState;
  gitStatus: GitStatusState;
  gitSnapshots: GitSnapshotsState;
  sessionCommits: SessionCommitsState;
  contextWindow: ContextWindowState;
  agents: AgentState;
  availableCommands: AvailableCommandsState;
  sessionMode: SessionModeState;
  userShells: UserShellsState;
  prepareProgress: PrepareProgressState;
};

export type SessionRuntimeSliceActions = {
  setTerminalOutput: (terminalId: string, data: string) => void;
  appendShellOutput: (sessionId: string, data: string) => void;
  setShellStatus: (
    sessionId: string,
    status: { available: boolean; running?: boolean; shell?: string; cwd?: string },
  ) => void;
  clearShellOutput: (sessionId: string) => void;
  appendProcessOutput: (processId: string, data: string) => void;
  upsertProcessStatus: (status: ProcessStatusEntry) => void;
  clearProcessOutput: (processId: string) => void;
  setActiveProcess: (sessionId: string, processId: string) => void;
  setGitStatus: (sessionId: string, gitStatus: GitStatusEntry) => void;
  clearGitStatus: (sessionId: string) => void;
  setContextWindow: (sessionId: string, contextWindow: ContextWindowEntry) => void;
  // Git snapshot actions
  setGitSnapshots: (sessionId: string, snapshots: GitSnapshot[]) => void;
  setGitSnapshotsLoading: (sessionId: string, loading: boolean) => void;
  addGitSnapshot: (sessionId: string, snapshot: GitSnapshot) => void;
  // Session commit actions
  setSessionCommits: (sessionId: string, commits: SessionCommit[]) => void;
  setSessionCommitsLoading: (sessionId: string, loading: boolean) => void;
  addSessionCommit: (sessionId: string, commit: SessionCommit) => void;
  clearSessionCommits: (sessionId: string) => void;
  // Available commands actions
  setAvailableCommands: (sessionId: string, commands: AvailableCommand[]) => void;
  clearAvailableCommands: (sessionId: string) => void;
  // Session mode actions
  setSessionMode: (sessionId: string, modeId: string) => void;
  // User shells actions
  setUserShells: (sessionId: string, shells: UserShellInfo[]) => void;
  setUserShellsLoading: (sessionId: string, loading: boolean) => void;
  addUserShell: (sessionId: string, shell: UserShellInfo) => void;
  removeUserShell: (sessionId: string, terminalId: string) => void;
};

export type SessionRuntimeSlice = SessionRuntimeSliceState & SessionRuntimeSliceActions;
