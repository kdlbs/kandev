import type { ReactNode } from 'react';
import type {
  Agent,
  AgentDiscovery,
  TaskSessionState,
  Branch,
  Environment,
  Executor,
  Message,
  Repository,
  TaskState as TaskStatus,
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
  loaded: boolean;
};

export type TerminalState = {
  terminals: Array<{ id: string; output: string[] }>;
};

export type FileInfo = {
  path: string;
  status: 'modified' | 'added' | 'deleted' | 'untracked' | 'renamed';
  additions?: number;
  deletions?: number;
  old_path?: string;
  diff?: string;
};

export type GitStatusState = {
  taskId: string | null;
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

export type DiffState = {
  files: Array<{ path: string; status: 'A' | 'M' | 'D'; plus: number; minus: number }>;
};

export type ConnectionState = {
  status: 'disconnected' | 'connecting' | 'connected' | 'error' | 'reconnecting';
  error: string | null;
};

export type MessagesState = {
  sessionId: string | null;
  items: Message[];
  isLoading: boolean;
  hasMore: boolean;
  oldestCursor: string | null;
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
  diffs: DiffState;
  gitStatus: GitStatusState;
  connection: ConnectionState;
  messages: MessagesState;
  taskSessionStatesByTaskId: Record<string, TaskSessionState>;
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
  setConnectionStatus: (status: ConnectionState['status'], error?: string | null) => void;
  setMessages: (
    sessionId: string | null,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null }
  ) => void;
  setMessagesSessionId: (sessionId: string | null) => void;
  addMessage: (message: Message) => void;
  updateMessage: (message: Message) => void;
  prependMessages: (messages: Message[], meta?: { hasMore?: boolean; oldestCursor?: string | null }) => void;
  setMessagesMetadata: (meta: { hasMore?: boolean; isLoading?: boolean; oldestCursor?: string | null }) => void;
  setMessagesLoading: (loading: boolean) => void;
  setTaskSessionState: (taskId: string, state: TaskSessionState) => void;
  setGitStatus: (taskId: string, gitStatus: Omit<GitStatusState, 'taskId'>) => void;
  clearGitStatus: () => void;
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
  tasks: { activeTaskId: null },
  agents: { agents: [] },
  agentProfiles: { items: [], version: 0 },
  userSettings: { workspaceId: null, boardId: null, repositoryIds: [], loaded: false },
  terminal: { terminals: [] },
  diffs: { files: [] },
  gitStatus: {
    taskId: null,
    branch: null,
    remote_branch: null,
    modified: [],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: {},
    timestamp: null,
  },
  connection: { status: 'disconnected', error: null },
  messages: { sessionId: null, items: [], isLoading: false, hasMore: false, oldestCursor: null },
  taskSessionStatesByTaskId: {},
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
  setConnectionStatus: () => undefined,
  setMessages: () => undefined,
  setMessagesSessionId: () => undefined,
  addMessage: () => undefined,
  updateMessage: () => undefined,
  prependMessages: () => undefined,
  setMessagesMetadata: () => undefined,
  setMessagesLoading: () => undefined,
  setTaskSessionState: () => undefined,
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
  | 'setConnectionStatus'
  | 'setMessages'
  | 'setMessagesSessionId'
  | 'addMessage'
  | 'updateMessage'
  | 'prependMessages'
  | 'setMessagesMetadata'
  | 'setMessagesLoading'
  | 'setTaskSessionState'
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
    diffs: { ...defaultState.diffs, ...initialState.diffs },
    gitStatus: { ...defaultState.gitStatus, ...initialState.gitStatus },
    connection: { ...defaultState.connection, ...initialState.connection },
    messages: { ...defaultState.messages, ...initialState.messages },
    taskSessionStatesByTaskId: {
      ...defaultState.taskSessionStatesByTaskId,
      ...initialState.taskSessionStatesByTaskId,
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
          if (state.diffs) Object.assign(draft.diffs, state.diffs);
          if (state.connection) Object.assign(draft.connection, state.connection);
          if (state.messages) Object.assign(draft.messages, state.messages);
          if (state.taskSessionStatesByTaskId) {
            Object.assign(draft.taskSessionStatesByTaskId, state.taskSessionStatesByTaskId);
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
      setConnectionStatus: (status, error = null) =>
        set((draft) => {
          draft.connection.status = status;
          draft.connection.error = error;
        }),
      setMessages: (sessionId, messages, meta) =>
        set((draft) => {
          draft.messages.sessionId = sessionId;
          draft.messages.items = messages;
          draft.messages.isLoading = false;
          draft.messages.hasMore = meta?.hasMore ?? false;
          if (meta?.oldestCursor !== undefined) {
            draft.messages.oldestCursor = meta.oldestCursor;
          } else if (messages.length) {
            draft.messages.oldestCursor = messages[0].id;
          } else {
            draft.messages.oldestCursor = null;
          }
        }),
      setMessagesSessionId: (sessionId) =>
        set((draft) => {
          draft.messages.sessionId = sessionId;
          // Initialize items array if null to allow addMessage to work
          // before setMessages is called
          if (draft.messages.items === null) {
            draft.messages.items = [];
          }
        }),
      addMessage: (message) =>
        set((draft) => {
          // Initialize items array if null
          if (draft.messages.items === null) {
            draft.messages.items = [];
          }
          if (!draft.messages.sessionId) {
            draft.messages.sessionId = message.task_session_id;
          }
          // Only add if this message is for the current session
          if (draft.messages.sessionId === message.task_session_id) {
            // Check if message already exists (avoid duplicates)
            const exists = draft.messages.items.some((item) => item.id === message.id);
            if (!exists) {
              draft.messages.items.push(message);
              if (!draft.messages.oldestCursor) {
                draft.messages.oldestCursor = message.id;
              }
            }
          }
        }),
      updateMessage: (message) =>
        set((draft) => {
          if (!draft.messages.sessionId) {
            draft.messages.sessionId = message.task_session_id;
          }
          // Only update if this message is for the current session
          if (draft.messages.sessionId === message.task_session_id && draft.messages.items) {
            const index = draft.messages.items.findIndex((item) => item.id === message.id);
            if (index !== -1) {
              draft.messages.items[index] = message;
            }
          }
        }),
      prependMessages: (messages, meta) =>
        set((draft) => {
          if (draft.messages.items === null) {
            draft.messages.items = [];
          }
          const existingIds = new Set(draft.messages.items.map((item) => item.id));
          const incoming = messages.filter((item) => !existingIds.has(item.id));
          if (incoming.length) {
            draft.messages.items = [...incoming, ...draft.messages.items];
          }
          if (meta?.hasMore !== undefined) {
            draft.messages.hasMore = meta.hasMore;
          }
          if (meta?.oldestCursor !== undefined) {
            draft.messages.oldestCursor = meta.oldestCursor;
          } else if (draft.messages.items.length) {
            draft.messages.oldestCursor = draft.messages.items[0].id;
          } else {
            draft.messages.oldestCursor = null;
          }
          draft.messages.isLoading = false;
        }),
      setMessagesMetadata: (meta) =>
        set((draft) => {
          if (meta.hasMore !== undefined) {
            draft.messages.hasMore = meta.hasMore;
          }
          if (meta.isLoading !== undefined) {
            draft.messages.isLoading = meta.isLoading;
          }
          if (meta.oldestCursor !== undefined) {
            draft.messages.oldestCursor = meta.oldestCursor;
          }
        }),
      setMessagesLoading: (loading) =>
        set((draft) => {
          draft.messages.isLoading = loading;
        }),
      setTaskSessionState: (taskId, state) =>
        set((draft) => {
          draft.taskSessionStatesByTaskId[taskId] = state;
        }),
      setGitStatus: (taskId, gitStatus) =>
        set((draft) => {
          draft.gitStatus = {
            taskId,
            ...gitStatus,
          };
        }),
      clearGitStatus: () =>
        set((draft) => {
          draft.gitStatus = {
            taskId: null,
            branch: null,
            remote_branch: null,
            modified: [],
            added: [],
            deleted: [],
            untracked: [],
            renamed: [],
            ahead: 0,
            behind: 0,
            files: {},
            timestamp: null,
          };
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
