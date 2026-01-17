import type { ReactNode } from 'react';
import type {
  Agent,
  AgentDiscovery,
  Branch,
  Environment,
  Executor,
  Message,
  Repository,
  TaskState as TaskStatus,
  TaskSession,
} from '@/lib/types/http';
import { createStore } from 'zustand/vanilla';
import { immer } from 'zustand/middleware/immer';

export type KanbanState = {
  boardId: string | null;
  columns: Array<{ id: string; title: string; color: string; position: number }>;
  tasks: Array<{
    id: string;
    columnId: string;
    title: string;
    description?: string;
    position: number;
    state?: TaskStatus;
    repositoryId?: string;
  }>;
};

export type WorkspaceState = {
  items: Array<{
    id: string;
    name: string;
    description?: string | null;
    owner_id: string;
    default_executor_id?: string | null;
    default_environment_id?: string | null;
    default_agent_profile_id?: string | null;
    created_at: string;
    updated_at: string;
  }>;
  activeId: string | null;
};

export type AgentProfileOption = {
  id: string;
  label: string;
  agent_id: string;
};

export type ExecutorsState = {
  items: Executor[];
};

export type EnvironmentsState = {
  items: Environment[];
};

export type SettingsAgentsState = {
  items: Agent[];
};

export type AgentDiscoveryState = {
  items: AgentDiscovery[];
};

export type BoardState = {
  items: Array<{ id: string; workspaceId: string; name: string }>;
  activeId: string | null;
};

export type TaskState = {
  activeTaskId: string | null;
  activeSessionId: string | null;
};

export type AgentState = {
  agents: Array<{ id: string; status: 'idle' | 'running' | 'error' }>;
};

export type AgentProfilesState = {
  items: AgentProfileOption[];
  version: number;
};

export type RepositoriesState = {
  itemsByWorkspaceId: Record<string, Repository[]>;
  loadingByWorkspaceId: Record<string, boolean>;
  loadedByWorkspaceId: Record<string, boolean>;
};

export type RepositoryBranchesState = {
  itemsByRepositoryId: Record<string, Branch[]>;
  loadingByRepositoryId: Record<string, boolean>;
  loadedByRepositoryId: Record<string, boolean>;
};

export type SettingsDataState = {
  executorsLoaded: boolean;
  environmentsLoaded: boolean;
  agentsLoaded: boolean;
};

export type UserSettingsState = {
  workspaceId: string | null;
  boardId: string | null;
  repositoryIds: string[];
  preferredShell: string | null;
  loaded: boolean;
};

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

export type FileInfo = {
  path: string;
  status: 'modified' | 'added' | 'deleted' | 'untracked' | 'renamed';
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

export type Worktree = {
  id: string;
  sessionId: string;
  repositoryId?: string;
  path?: string;
  branch?: string;
};

export type WorktreesState = {
  items: Record<string, Worktree>;
};

export type SessionWorktreesState = {
  itemsBySessionId: Record<string, string[]>;
};

export type DiffState = {
  files: Array<{ path: string; status: 'A' | 'M' | 'D'; plus: number; minus: number }>;
};

export type ConnectionState = {
  status: 'disconnected' | 'connecting' | 'connected' | 'error' | 'reconnecting';
  error: string | null;
};

export type MessagesState = {
  bySession: Record<string, Message[]>;
  metaBySession: Record<
    string,
    {
      isLoading: boolean;
      hasMore: boolean;
      oldestCursor: string | null;
    }
  >;
};

export type TaskSessionsState = {
  items: Record<string, TaskSession>;
};

export type TaskSessionsByTaskState = {
  itemsByTaskId: Record<string, TaskSession[]>;
  loadingByTaskId: Record<string, boolean>;
  loadedByTaskId: Record<string, boolean>;
};

export type SessionAgentctlStatus = {
  status: 'starting' | 'ready' | 'error';
  errorMessage?: string;
  agentExecutionId?: string;
  updatedAt?: string;
};

export type SessionAgentctlState = {
  itemsBySessionId: Record<string, SessionAgentctlStatus>;
};

export type AppState = {
  kanban: KanbanState;
  workspaces: WorkspaceState;
  boards: BoardState;
  executors: ExecutorsState;
  environments: EnvironmentsState;
  settingsAgents: SettingsAgentsState;
  agentDiscovery: AgentDiscoveryState;
  repositories: RepositoriesState;
  repositoryBranches: RepositoryBranchesState;
  settingsData: SettingsDataState;
  tasks: TaskState;
  agents: AgentState;
  agentProfiles: AgentProfilesState;
  userSettings: UserSettingsState;
  terminal: TerminalState;
  shell: ShellState;
  diffs: DiffState;
  gitStatus: GitStatusState;
  connection: ConnectionState;
  messages: MessagesState;
  taskSessions: TaskSessionsState;
  taskSessionsByTask: TaskSessionsByTaskState;
  sessionAgentctl: SessionAgentctlState;
  worktrees: WorktreesState;
  sessionWorktreesBySessionId: SessionWorktreesState;
  hydrate: (state: Partial<AppState>) => void;
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState['items']) => void;
  setActiveBoard: (boardId: string | null) => void;
  setBoards: (boards: BoardState['items']) => void;
  setExecutors: (executors: ExecutorsState['items']) => void;
  setEnvironments: (environments: EnvironmentsState['items']) => void;
  setSettingsAgents: (agents: SettingsAgentsState['items']) => void;
  setAgentDiscovery: (agents: AgentDiscoveryState['items']) => void;
  setAgentProfiles: (profiles: AgentProfilesState['items']) => void;
  setRepositories: (workspaceId: string, repositories: Repository[]) => void;
  setRepositoriesLoading: (workspaceId: string, loading: boolean) => void;
  setRepositoryBranches: (repositoryId: string, branches: Branch[]) => void;
  setRepositoryBranchesLoading: (repositoryId: string, loading: boolean) => void;
  setSettingsData: (next: Partial<SettingsDataState>) => void;
  setUserSettings: (settings: UserSettingsState) => void;
  setTerminalOutput: (terminalId: string, data: string) => void;
  appendShellOutput: (sessionId: string, data: string) => void;
  setShellStatus: (
    sessionId: string,
    status: { available: boolean; running?: boolean; shell?: string; cwd?: string }
  ) => void;
  clearShellOutput: (sessionId: string) => void;
  setConnectionStatus: (status: ConnectionState['status'], error?: string | null) => void;
  setMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null }
  ) => void;
  addMessage: (message: Message) => void;
  updateMessage: (message: Message) => void;
  prependMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null }
  ) => void;
  setMessagesMetadata: (
    sessionId: string,
    meta: { hasMore?: boolean; isLoading?: boolean; oldestCursor?: string | null }
  ) => void;
  setMessagesLoading: (sessionId: string, loading: boolean) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
  clearActiveSession: () => void;
  setTaskSession: (session: TaskSession) => void;
  setTaskSessionsForTask: (taskId: string, sessions: TaskSession[]) => void;
  setTaskSessionsLoading: (taskId: string, loading: boolean) => void;
  setSessionAgentctlStatus: (sessionId: string, status: SessionAgentctlStatus) => void;
  setWorktree: (worktree: Worktree) => void;
  setSessionWorktrees: (sessionId: string, worktreeIds: string[]) => void;
  setGitStatus: (sessionId: string, gitStatus: GitStatusEntry) => void;
  clearGitStatus: (sessionId: string) => void;
  bumpAgentProfilesVersion: () => void;
};

export type AppStore = ReturnType<typeof createAppStore>;

const defaultState: AppState = {
  kanban: { boardId: null, columns: [], tasks: [] },
  workspaces: { items: [], activeId: null },
  boards: { items: [], activeId: null },
  executors: { items: [] },
  environments: { items: [] },
  settingsAgents: { items: [] },
  agentDiscovery: { items: [] },
  repositories: { itemsByWorkspaceId: {}, loadingByWorkspaceId: {}, loadedByWorkspaceId: {} },
  repositoryBranches: { itemsByRepositoryId: {}, loadingByRepositoryId: {}, loadedByRepositoryId: {} },
  settingsData: { executorsLoaded: false, environmentsLoaded: false, agentsLoaded: false },
  tasks: { activeTaskId: null, activeSessionId: null },
  agents: { agents: [] },
  agentProfiles: { items: [], version: 0 },
  userSettings: {
    workspaceId: null,
    boardId: null,
    repositoryIds: [],
    preferredShell: null,
    loaded: false,
  },
  terminal: { terminals: [] },
  shell: { outputs: {}, statuses: {} },
  diffs: { files: [] },
  gitStatus: { bySessionId: {} },
  connection: { status: 'disconnected', error: null },
  messages: { bySession: {}, metaBySession: {} },
  taskSessions: { items: {} },
  taskSessionsByTask: { itemsByTaskId: {}, loadingByTaskId: {}, loadedByTaskId: {} },
  sessionAgentctl: { itemsBySessionId: {} },
  worktrees: { items: {} },
  sessionWorktreesBySessionId: { itemsBySessionId: {} },
  hydrate: () => undefined,
  setActiveWorkspace: () => undefined,
  setWorkspaces: () => undefined,
  setActiveBoard: () => undefined,
  setBoards: () => undefined,
  setExecutors: () => undefined,
  setEnvironments: () => undefined,
  setSettingsAgents: () => undefined,
  setAgentDiscovery: () => undefined,
  setAgentProfiles: () => undefined,
  setRepositories: () => undefined,
  setRepositoriesLoading: () => undefined,
  setRepositoryBranches: () => undefined,
  setRepositoryBranchesLoading: () => undefined,
  setSettingsData: () => undefined,
  setUserSettings: () => undefined,
  setTerminalOutput: () => undefined,
  appendShellOutput: () => undefined,
  setShellStatus: () => undefined,
  clearShellOutput: () => undefined,
  setConnectionStatus: () => undefined,
  setMessages: () => undefined,
  addMessage: () => undefined,
  updateMessage: () => undefined,
  prependMessages: () => undefined,
  setMessagesMetadata: () => undefined,
  setMessagesLoading: () => undefined,
  setActiveSession: () => undefined,
  setActiveTask: () => undefined,
  clearActiveSession: () => undefined,
  setTaskSession: () => undefined,
  setTaskSessionsForTask: () => undefined,
  setTaskSessionsLoading: () => undefined,
  setSessionAgentctlStatus: () => undefined,
  setWorktree: () => undefined,
  setSessionWorktrees: () => undefined,
  setGitStatus: () => undefined,
  clearGitStatus: () => undefined,
  bumpAgentProfilesVersion: () => undefined,
};

function mergeInitialState(
  initialState?: Partial<AppState>
): Omit<
  AppState,
  | 'hydrate'
  | 'setActiveWorkspace'
  | 'setWorkspaces'
  | 'setActiveBoard'
  | 'setBoards'
  | 'setExecutors'
  | 'setEnvironments'
  | 'setSettingsAgents'
  | 'setAgentDiscovery'
  | 'setAgentProfiles'
  | 'setRepositories'
  | 'setRepositoriesLoading'
  | 'setRepositoryBranches'
  | 'setRepositoryBranchesLoading'
  | 'setSettingsData'
  | 'setUserSettings'
  | 'setTerminalOutput'
  | 'appendShellOutput'
  | 'setShellStatus'
  | 'clearShellOutput'
  | 'setConnectionStatus'
  | 'setMessages'
  | 'addMessage'
  | 'updateMessage'
  | 'prependMessages'
  | 'setMessagesMetadata'
  | 'setMessagesLoading'
  | 'setActiveSession'
  | 'setActiveTask'
  | 'clearActiveSession'
  | 'setTaskSession'
  | 'setTaskSessionsForTask'
  | 'setTaskSessionsLoading'
  | 'setSessionAgentctlStatus'
  | 'setWorktree'
  | 'setSessionWorktrees'
  | 'setGitStatus'
  | 'clearGitStatus'
  | 'bumpAgentProfilesVersion'
> {
  if (!initialState) return defaultState;
  return {
    workspaces: { ...defaultState.workspaces, ...initialState.workspaces },
    boards: { ...defaultState.boards, ...initialState.boards },
    executors: { ...defaultState.executors, ...initialState.executors },
    environments: { ...defaultState.environments, ...initialState.environments },
    settingsAgents: { ...defaultState.settingsAgents, ...initialState.settingsAgents },
    agentDiscovery: { ...defaultState.agentDiscovery, ...initialState.agentDiscovery },
    repositories: { ...defaultState.repositories, ...initialState.repositories },
    repositoryBranches: { ...defaultState.repositoryBranches, ...initialState.repositoryBranches },
    settingsData: { ...defaultState.settingsData, ...initialState.settingsData },
    kanban: { ...defaultState.kanban, ...initialState.kanban },
    tasks: { ...defaultState.tasks, ...initialState.tasks },
    agents: { ...defaultState.agents, ...initialState.agents },
    agentProfiles: { ...defaultState.agentProfiles, ...initialState.agentProfiles },
    userSettings: { ...defaultState.userSettings, ...initialState.userSettings },
    terminal: { ...defaultState.terminal, ...initialState.terminal },
    shell: { ...defaultState.shell, ...initialState.shell },
    diffs: { ...defaultState.diffs, ...initialState.diffs },
    gitStatus: { ...defaultState.gitStatus, ...initialState.gitStatus },
    connection: { ...defaultState.connection, ...initialState.connection },
    messages: { ...defaultState.messages, ...initialState.messages },
    taskSessions: { ...defaultState.taskSessions, ...initialState.taskSessions },
    taskSessionsByTask: { ...defaultState.taskSessionsByTask, ...initialState.taskSessionsByTask },
    sessionAgentctl: { ...defaultState.sessionAgentctl, ...initialState.sessionAgentctl },
    worktrees: { ...defaultState.worktrees, ...initialState.worktrees },
    sessionWorktreesBySessionId: {
      ...defaultState.sessionWorktreesBySessionId,
      ...initialState.sessionWorktreesBySessionId,
    },
  };
}

export function createAppStore(initialState?: Partial<AppState>) {
  const merged = mergeInitialState(initialState);
  return createStore<AppState>()(
    immer((set) => ({
      ...merged,
      hydrate: (state) =>
        set((draft) => {
          // Deep merge to avoid overwriting nested objects with undefined
          if (state.workspaces) Object.assign(draft.workspaces, state.workspaces);
          if (state.boards) Object.assign(draft.boards, state.boards);
          if (state.executors) Object.assign(draft.executors, state.executors);
          if (state.environments) Object.assign(draft.environments, state.environments);
          if (state.settingsAgents) Object.assign(draft.settingsAgents, state.settingsAgents);
          if (state.agentDiscovery) Object.assign(draft.agentDiscovery, state.agentDiscovery);
          if (state.repositories) Object.assign(draft.repositories, state.repositories);
          if (state.repositoryBranches) Object.assign(draft.repositoryBranches, state.repositoryBranches);
          if (state.settingsData) Object.assign(draft.settingsData, state.settingsData);
          if (state.kanban) Object.assign(draft.kanban, state.kanban);
          if (state.tasks) Object.assign(draft.tasks, state.tasks);
          if (state.agents) Object.assign(draft.agents, state.agents);
          if (state.agentProfiles) Object.assign(draft.agentProfiles, state.agentProfiles);
          if (state.userSettings) Object.assign(draft.userSettings, state.userSettings);
          if (state.terminal) Object.assign(draft.terminal, state.terminal);
          if (state.shell) Object.assign(draft.shell, state.shell);
          if (state.diffs) Object.assign(draft.diffs, state.diffs);
          if (state.gitStatus) Object.assign(draft.gitStatus, state.gitStatus);
          if (state.connection) Object.assign(draft.connection, state.connection);
          if (state.messages) Object.assign(draft.messages, state.messages);
          if (state.taskSessions) Object.assign(draft.taskSessions, state.taskSessions);
          if (state.taskSessionsByTask) {
            Object.assign(draft.taskSessionsByTask, state.taskSessionsByTask);
          }
          if (state.sessionAgentctl) {
            Object.assign(draft.sessionAgentctl, state.sessionAgentctl);
          }
          if (state.worktrees) Object.assign(draft.worktrees, state.worktrees);
          if (state.sessionWorktreesBySessionId) {
            Object.assign(draft.sessionWorktreesBySessionId, state.sessionWorktreesBySessionId);
          }
        }),
      setActiveWorkspace: (workspaceId) =>
        set((draft) => {
          draft.workspaces.activeId = workspaceId;
        }),
      setWorkspaces: (workspaces) =>
        set((draft) => {
          draft.workspaces.items = workspaces;
          if (!draft.workspaces.activeId && workspaces.length) {
            draft.workspaces.activeId = workspaces[0].id;
          }
        }),
      setBoards: (boards) =>
        set((draft) => {
          draft.boards.items = boards;
          if (!draft.boards.activeId && boards.length) {
            draft.boards.activeId = boards[0].id;
          }
        }),
      setActiveBoard: (boardId) =>
        set((draft) => {
          draft.boards.activeId = boardId;
        }),
      setExecutors: (executors) =>
        set((draft) => {
          draft.executors.items = executors;
        }),
      setEnvironments: (environments) =>
        set((draft) => {
          draft.environments.items = environments;
        }),
      setSettingsAgents: (agents) =>
        set((draft) => {
          draft.settingsAgents.items = agents;
        }),
      setAgentDiscovery: (agents) =>
        set((draft) => {
          draft.agentDiscovery.items = agents;
        }),
      setAgentProfiles: (profiles) =>
        set((draft) => {
          draft.agentProfiles.items = profiles;
        }),
      setRepositories: (workspaceId, repositories) =>
        set((draft) => {
          draft.repositories.itemsByWorkspaceId[workspaceId] = repositories;
          draft.repositories.loadingByWorkspaceId[workspaceId] = false;
          draft.repositories.loadedByWorkspaceId[workspaceId] = true;
        }),
      setRepositoriesLoading: (workspaceId, loading) =>
        set((draft) => {
          draft.repositories.loadingByWorkspaceId[workspaceId] = loading;
          if (!loading && !draft.repositories.loadedByWorkspaceId[workspaceId]) {
            draft.repositories.loadedByWorkspaceId[workspaceId] = false;
          }
        }),
      setRepositoryBranches: (repositoryId, branches) =>
        set((draft) => {
          draft.repositoryBranches.itemsByRepositoryId[repositoryId] = branches;
          draft.repositoryBranches.loadingByRepositoryId[repositoryId] = false;
          draft.repositoryBranches.loadedByRepositoryId[repositoryId] = true;
        }),
      setRepositoryBranchesLoading: (repositoryId, loading) =>
        set((draft) => {
          draft.repositoryBranches.loadingByRepositoryId[repositoryId] = loading;
          if (!loading && !draft.repositoryBranches.loadedByRepositoryId[repositoryId]) {
            draft.repositoryBranches.loadedByRepositoryId[repositoryId] = false;
          }
        }),
      setSettingsData: (next) =>
        set((draft) => {
          draft.settingsData = { ...draft.settingsData, ...next };
        }),
      setUserSettings: (settings) =>
        set((draft) => {
          draft.userSettings = settings;
        }),
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
      setConnectionStatus: (status, error = null) =>
        set((draft) => {
          draft.connection.status = status;
          draft.connection.error = error;
        }),
      setMessages: (sessionId, messages, meta) =>
        set((draft) => {
          draft.messages.bySession[sessionId] = messages;
          const existingMeta = draft.messages.metaBySession[sessionId];
          draft.messages.metaBySession[sessionId] = {
            isLoading: false,
            hasMore: meta?.hasMore ?? existingMeta?.hasMore ?? false,
            oldestCursor:
              meta?.oldestCursor ?? (messages.length ? messages[0].id : existingMeta?.oldestCursor ?? null),
          };
        }),
      addMessage: (message) =>
        set((draft) => {
          if (!message.session_id) {
            return;
          }
          const sessionId = message.session_id;
          const list = draft.messages.bySession[sessionId] ?? [];
          const exists = list.some((item) => item.id === message.id);
          if (exists) {
            return;
          }
          draft.messages.bySession[sessionId] = [...list, message];
          const meta = draft.messages.metaBySession[sessionId] ?? {
            isLoading: false,
            hasMore: false,
            oldestCursor: null,
          };
          if (!meta.oldestCursor) {
            meta.oldestCursor = message.id;
          }
          draft.messages.metaBySession[sessionId] = meta;
        }),
      updateMessage: (message) =>
        set((draft) => {
          if (!message.session_id) {
            return;
          }
          const sessionId = message.session_id;
          const list = draft.messages.bySession[sessionId];
          if (!list) return;
          const index = list.findIndex((item) => item.id === message.id);
          if (index !== -1) {
            list[index] = message;
          }
        }),
      prependMessages: (sessionId, messages, meta) =>
        set((draft) => {
          const existing = draft.messages.bySession[sessionId] ?? [];
          const existingIds = new Set(existing.map((item) => item.id));
          const incoming = messages.filter((item) => !existingIds.has(item.id));
          const next = incoming.length ? [...incoming, ...existing] : existing;
          draft.messages.bySession[sessionId] = next;
          const currentMeta = draft.messages.metaBySession[sessionId] ?? {
            isLoading: false,
            hasMore: false,
            oldestCursor: null,
          };
          draft.messages.metaBySession[sessionId] = {
            isLoading: false,
            hasMore: meta?.hasMore ?? currentMeta.hasMore,
            oldestCursor:
              meta?.oldestCursor ?? (next.length ? next[0].id : currentMeta.oldestCursor ?? null),
          };
        }),
      setMessagesMetadata: (sessionId, meta) =>
        set((draft) => {
          const currentMeta = draft.messages.metaBySession[sessionId] ?? {
            isLoading: false,
            hasMore: false,
            oldestCursor: null,
          };
          draft.messages.metaBySession[sessionId] = {
            isLoading: meta.isLoading ?? currentMeta.isLoading,
            hasMore: meta.hasMore ?? currentMeta.hasMore,
            oldestCursor: meta.oldestCursor ?? currentMeta.oldestCursor,
          };
        }),
      setMessagesLoading: (sessionId, loading) =>
        set((draft) => {
          const currentMeta = draft.messages.metaBySession[sessionId] ?? {
            isLoading: false,
            hasMore: false,
            oldestCursor: null,
          };
          currentMeta.isLoading = loading;
          draft.messages.metaBySession[sessionId] = currentMeta;
        }),
      setActiveSession: (taskId, sessionId) =>
        set((draft) => {
          draft.tasks.activeTaskId = taskId;
          draft.tasks.activeSessionId = sessionId;
        }),
      setActiveTask: (taskId) =>
        set((draft) => {
          draft.tasks.activeTaskId = taskId;
          draft.tasks.activeSessionId = null;
        }),
      clearActiveSession: () =>
        set((draft) => {
          draft.tasks.activeTaskId = null;
          draft.tasks.activeSessionId = null;
        }),
      setTaskSession: (session) =>
        set((draft) => {
          draft.taskSessions.items[session.id] = session;
          if (session.worktree_id) {
            draft.worktrees.items[session.worktree_id] = {
              id: session.worktree_id,
              sessionId: session.id,
              repositoryId: session.repository_id ?? undefined,
              path: session.worktree_path ?? undefined,
              branch: session.worktree_branch ?? undefined,
            };
            draft.sessionWorktreesBySessionId.itemsBySessionId[session.id] = [session.worktree_id];
          }
        }),
      setTaskSessionsForTask: (taskId, sessions) =>
        set((draft) => {
          sessions.forEach((session) => {
            draft.taskSessions.items[session.id] = session;
            if (session.worktree_id) {
              draft.worktrees.items[session.worktree_id] = {
                id: session.worktree_id,
                sessionId: session.id,
                repositoryId: session.repository_id ?? undefined,
                path: session.worktree_path ?? undefined,
                branch: session.worktree_branch ?? undefined,
              };
              draft.sessionWorktreesBySessionId.itemsBySessionId[session.id] = [session.worktree_id];
            }
          });
          draft.taskSessionsByTask.itemsByTaskId[taskId] = sessions;
          draft.taskSessionsByTask.loadedByTaskId[taskId] = true;
          draft.taskSessionsByTask.loadingByTaskId[taskId] = false;
        }),
      setTaskSessionsLoading: (taskId, loading) =>
        set((draft) => {
          draft.taskSessionsByTask.loadingByTaskId[taskId] = loading;
        }),
      setSessionAgentctlStatus: (sessionId, status) =>
        set((draft) => {
          draft.sessionAgentctl.itemsBySessionId[sessionId] = status;
        }),
      setWorktree: (worktree) =>
        set((draft) => {
          draft.worktrees.items[worktree.id] = worktree;
          const existing = draft.sessionWorktreesBySessionId.itemsBySessionId[worktree.sessionId] ?? [];
          if (!existing.includes(worktree.id)) {
            draft.sessionWorktreesBySessionId.itemsBySessionId[worktree.sessionId] = [...existing, worktree.id];
          }
        }),
      setSessionWorktrees: (sessionId, worktreeIds) =>
        set((draft) => {
          draft.sessionWorktreesBySessionId.itemsBySessionId[sessionId] = worktreeIds;
        }),
      setGitStatus: (sessionId, gitStatus) =>
        set((draft) => {
          draft.gitStatus.bySessionId[sessionId] = gitStatus;
        }),
      clearGitStatus: (sessionId) =>
        set((draft) => {
          delete draft.gitStatus.bySessionId[sessionId];
        }),
      bumpAgentProfilesVersion: () =>
        set((draft) => {
          draft.agentProfiles.version += 1;
        }),
    }))
  );
}

export type StoreProviderProps = {
  children: ReactNode;
  initialState?: Partial<AppState>;
};
