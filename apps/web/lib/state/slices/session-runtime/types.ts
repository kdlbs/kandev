export type TerminalState = {
  terminals: Array<{ id: string; output: string[] }>;
};

export type ShellState = {
  /** Shell output keyed by environmentId (shared across sessions in the same environment).
   *  Falls back to sessionId when no environment mapping exists. */
  outputs: Record<string, string>;
  /** Shell status keyed by environmentId (shared across sessions in the same environment).
   *  Falls back to sessionId when no environment mapping exists. */
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
  diff_skip_reason?: "too_large" | "binary" | "truncated" | "budget_exceeded";
  /**
   * Repository this file belongs to in multi-repo task workspaces. Stamped
   * by useSessionGit when aggregating per-repo statuses; empty for single-
   * repo workspaces. The Changes panel uses it to group files under
   * per-repository headers.
   */
  repository_name?: string;
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
  branch_additions?: number;
  branch_deletions?: number;
  /**
   * Repository this status belongs to in multi-repo task workspaces. Empty
   * for single-repo and used as the per-repo key in
   * GitStatusState.byEnvironmentRepo.
   */
  repository_name?: string;
};

export type GitStatusState = {
  /** Git status keyed by environment ID (shared across sessions in the same environment).
   *  Falls back to session ID when no environment exists.
   *  For multi-repo workspaces this holds the most recently received status
   *  (whichever repo emitted last); per-repo state lives in byEnvironmentRepo.
   */
  byEnvironmentId: Record<string, GitStatusEntry>;
  /**
   * Per-repository git status for multi-repo task workspaces, keyed by
   * environment ID then repository name. Empty for single-repo workspaces.
   */
  byEnvironmentRepo: Record<string, Record<string, GitStatusEntry>>;
};

// Git Snapshot types for historical tracking
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
  /** Multi-repo: name of the repo this commit was made in. Empty for single-repo. */
  repository_name?: string;
  /**
   * True when the commit is reachable from the branch's upstream tracking ref.
   * Sourced from git on the backend so it stays correct without an open PR.
   * Optional because incremental commit_created notifications don't carry it
   * (newly-made commits are always unpushed); the next full refetch fills it
   * in with the real value.
   */
  pushed?: boolean;
};

export type CumulativeDiff = {
  session_id: string;
  base_commit: string;
  head_commit: string;
  total_commits: number;
  files: Record<string, FileInfo>;
  /**
   * Files dropped from `files` because the cumulative range exceeded the
   * backend's per-request file cap (a mid-rebase base→working-tree diff can
   * enumerate tens of thousands of files). Zero/absent when the full set fit.
   * Surfaced to the user as a "N more files hidden" banner.
   */
  truncated_files_count?: number;
};

export type SessionCommitsState = {
  byEnvironmentId: Record<string, SessionCommit[]>;
  loading: Record<string, boolean>;
  // Stale-while-revalidate signal: bumped by WS handlers (commits_reset /
  // branch_switched) that previously cleared `byEnvironmentId` outright.
  // useSessionCommits watches this counter and refetches without nulling the
  // visible list, so the Changes panel doesn't flicker through its empty
  // state while the refetch is in flight.
  refetchTrigger: Record<string, number>;
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

export type SessionModeEntry = {
  id: string;
  name: string;
  description?: string;
};

export type SessionModeState = {
  bySessionId: Record<
    string,
    {
      currentModeId: string;
      availableModes: SessionModeEntry[];
    }
  >;
};

export type AuthMethodEntry = {
  id: string;
  name: string;
  description?: string;
  terminalAuth?: { command: string; args?: string[]; label?: string };
  meta?: Record<string, unknown>;
};

export type SessionModelEntry = {
  modelId: string;
  name: string;
  description?: string;
  usageMultiplier?: string;
  meta?: Record<string, unknown>;
};

export type ConfigOptionEntry = {
  type: string;
  id: string;
  name: string;
  currentValue: string;
  category?: string;
  options?: { value: string; name: string }[];
};

export type AgentCapabilitiesEntry = {
  supportsImage: boolean;
  supportsAudio: boolean;
  supportsEmbeddedContext: boolean;
  authMethods: AuthMethodEntry[];
};

export type PromptUsageEntry = {
  inputTokens: number;
  outputTokens: number;
  cachedReadTokens?: number;
  cachedWriteTokens?: number;
  totalTokens: number;
};

export type AgentCapabilitiesState = {
  bySessionId: Record<string, AgentCapabilitiesEntry>;
};

export type SessionModelsState = {
  bySessionId: Record<
    string,
    {
      currentModelId: string;
      models: SessionModelEntry[];
      configOptions: ConfigOptionEntry[];
    }
  >;
};

export type PromptUsageState = {
  bySessionId: Record<string, PromptUsageEntry>;
};

/**
 * User shell terminal info. Discriminated by `kind`:
 * - `ordinary` — a DB-backed first-class terminal. Carries seq + custom_name
 *   + state. Renameable, parkable, gets a `#N` badge.
 * - `fixed` — the hardcoded `bottom-panel` terminal (cmd+J). No badge, no
 *   rename, never parked.
 * - `script` — a script-driven terminal. Lifecycle tied to the script.
 *
 * Legacy fields (processId, running, label, closable) are kept optional so
 * old wire shapes still parse cleanly during the transition; new UI reads
 * the discriminated fields below.
 */
export type UserShellKind = "ordinary" | "fixed" | "script";
export type UserShellState = "open" | "parked";
export type UserShellPTYStatus = "running" | "stopped";

export type UserShellInfo = {
  terminalId: string;
  kind?: UserShellKind;

  // Ordinary-only metadata.
  seq?: number;
  customName?: string | null;
  displayName?: string;
  state?: UserShellState;
  ptyStatus?: UserShellPTYStatus;

  // Legacy / common fields.
  processId?: string;
  running?: boolean;
  label?: string;
  closable?: boolean;
  initialCommand?: string;
};

export type UserShellsState = {
  /** User shells keyed by environmentId (shared across sessions in the same environment). */
  byEnvironmentId: Record<string, UserShellInfo[]>;
  /** Keyed by environmentId (same key strategy as byEnvironmentId). */
  loading: Record<string, boolean>;
  /** Keyed by environmentId (same key strategy as byEnvironmentId). */
  loaded: Record<string, boolean>;
};

export type PrepareStepInfo = {
  name: string;
  command?: string;
  status: string;
  output?: string;
  error?: string;
  warning?: string;
  warningDetail?: string;
  startedAt?: string;
  endedAt?: string;
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

export type TodoEntry = {
  description: string;
  status: "pending" | "in_progress" | "completed" | "failed";
  priority?: string;
};

export type SessionTodosState = {
  bySessionId: Record<string, TodoEntry[]>;
};

export type SessionPollMode = "fast" | "slow" | "paused";

export type SessionPollModeState = {
  bySessionId: Record<string, SessionPollMode>;
};

export type SessionRuntimeSliceState = {
  terminal: TerminalState;
  shell: ShellState;
  processes: ProcessState;
  gitStatus: GitStatusState;
  /** Maps sessionId → environmentId for workspace state sharing. */
  environmentIdBySessionId: Record<string, string>;
  sessionCommits: SessionCommitsState;
  contextWindow: ContextWindowState;
  agents: AgentState;
  availableCommands: AvailableCommandsState;
  sessionMode: SessionModeState;
  agentCapabilities: AgentCapabilitiesState;
  sessionModels: SessionModelsState;
  promptUsage: PromptUsageState;
  sessionTodos: SessionTodosState;
  userShells: UserShellsState;
  prepareProgress: PrepareProgressState;
  sessionPollMode: SessionPollModeState;
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
  /** Returns true when the update meaningfully changed git state (so callers
   *  can invalidate derived caches without repeating the deep comparison). */
  setGitStatus: (sessionId: string, gitStatus: GitStatusEntry) => boolean;
  clearGitStatus: (sessionId: string) => void;
  /** Drops the pre-multi-repo (empty-repo-name) git-status entries so a
   *  freshly-multi-branch session doesn't surface a stale snapshot from the
   *  workspace tracker that was replaced on the backend during rescan. */
  clearLegacyGitStatusEntry: (sessionId: string) => void;
  registerSessionEnvironment: (sessionId: string, environmentId: string) => void;
  setContextWindow: (sessionId: string, contextWindow: ContextWindowEntry) => void;
  clearContextWindow: (sessionId: string) => void;
  // Session commit actions
  setSessionCommits: (
    sessionId: string,
    commits: SessionCommit[],
    opts?: { allowEmpty?: boolean },
  ) => void;
  setSessionCommitsLoading: (sessionId: string, loading: boolean) => void;
  addSessionCommit: (sessionId: string, commit: SessionCommit) => void;
  clearSessionCommits: (sessionId: string) => void;
  // Signal a refetch without clearing the visible list — see
  // SessionCommitsState.refetchTrigger.
  bumpSessionCommitsRefetch: (sessionId: string) => void;
  // Available commands actions
  setAvailableCommands: (sessionId: string, commands: AvailableCommand[]) => void;
  clearAvailableCommands: (sessionId: string) => void;
  // Session mode actions
  setSessionMode: (sessionId: string, modeId: string, availableModes?: SessionModeEntry[]) => void;
  clearSessionMode: (sessionId: string) => void;
  // Agent capabilities actions
  setAgentCapabilities: (sessionId: string, caps: AgentCapabilitiesEntry) => void;
  // Session models actions
  setSessionModels: (
    sessionId: string,
    data: {
      currentModelId: string;
      models: SessionModelEntry[];
      configOptions: ConfigOptionEntry[];
    },
  ) => void;
  // Prompt usage actions
  setPromptUsage: (sessionId: string, usage: PromptUsageEntry) => void;
  // Session todos actions
  setSessionTodos: (sessionId: string, entries: TodoEntry[]) => void;
  // User shells actions — env-scoped (sessions in the same task share one shell list)
  setUserShells: (environmentId: string, shells: UserShellInfo[]) => void;
  setUserShellsLoading: (environmentId: string, loading: boolean) => void;
  addUserShell: (environmentId: string, shell: UserShellInfo) => void;
  removeUserShell: (environmentId: string, terminalId: string) => void;
  updateUserShell: (
    environmentId: string,
    terminalId: string,
    // `terminalId` is the row key — patching it would silently break
    // future lookups while leaving the array index pointing at the old
    // entry. `Omit` removes it from the patch surface.
    patch: Partial<Omit<UserShellInfo, "terminalId">>,
  ) => void;
  setSessionPollMode: (sessionId: string, mode: SessionPollMode) => void;
};

export type SessionRuntimeSlice = SessionRuntimeSliceState & SessionRuntimeSliceActions;
