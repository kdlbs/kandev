import { createStore } from 'zustand/vanilla';
import { immer } from 'zustand/middleware/immer';
import { hydrateState, type HydrationOptions } from './hydration/hydrator';
import type { Repository, Branch, Message, Turn, TaskSession } from '@/lib/types/http';
import {
  createKanbanSlice,
  createWorkspaceSlice,
  createSettingsSlice,
  createSessionSlice,
  createSessionRuntimeSlice,
  createUISlice,
  defaultKanbanState,
  defaultWorkspaceState,
  defaultSettingsState,
  defaultSessionState,
  defaultSessionRuntimeState,
  defaultUIState,
  type WorkspaceState,
  type BoardState,
  type ExecutorsState,
  type EnvironmentsState,
  type SettingsAgentsState,
  type AgentDiscoveryState,
  type AvailableAgentsState,
  type AgentProfilesState,
  type EditorsState,
  type PromptsState,
  type NotificationProvidersState,
  type SettingsDataState,
  type UserSettingsState,
  type ProcessStatusEntry,
  type Worktree,
  type GitStatusEntry,
  type GitSnapshot,
  type SessionCommit,
  type ContextWindowEntry,
  type SessionAgentctlStatus,
  type PreviewStage,
  type PreviewViewMode,
  type PreviewDevicePreset,
  type ConnectionState,
} from './slices';

// Re-export all types from slices for backwards compatibility
export type {
  KanbanState,
  BoardState,
  TaskState,
  WorkspaceState,
  RepositoriesState,
  RepositoryBranchesState,
  ExecutorsState,
  EnvironmentsState,
  SettingsAgentsState,
  AgentDiscoveryState,
  AvailableAgentsState,
  AgentProfileOption,
  AgentProfilesState,
  EditorsState,
  PromptsState,
  NotificationProvidersState,
  SettingsDataState,
  UserSettingsState,
  MessagesState,
  TurnsState,
  TaskSessionsState,
  TaskSessionsByTaskState,
  SessionAgentctlStatus,
  SessionAgentctlState,
  Worktree,
  WorktreesState,
  SessionWorktreesState,
  PendingModelState,
  ActiveModelState,
  TerminalState,
  ShellState,
  ProcessStatusEntry,
  ProcessState,
  FileInfo,
  GitStatusEntry,
  GitStatusState,
  ContextWindowEntry,
  ContextWindowState,
  AgentState,
  PreviewStage,
  PreviewViewMode,
  PreviewDevicePreset,
  PreviewPanelState,
  RightPanelState,
  DiffState,
  ConnectionState,
  MobileKanbanState,
} from './slices';

// Combined AppState type
export type AppState = {
  // Kanban slice
  kanban: typeof defaultKanbanState['kanban'];
  boards: typeof defaultKanbanState['boards'];
  tasks: typeof defaultKanbanState['tasks'];

  // Workspace slice
  workspaces: typeof defaultWorkspaceState['workspaces'];
  repositories: typeof defaultWorkspaceState['repositories'];
  repositoryBranches: typeof defaultWorkspaceState['repositoryBranches'];

  // Settings slice
  executors: typeof defaultSettingsState['executors'];
  environments: typeof defaultSettingsState['environments'];
  settingsAgents: typeof defaultSettingsState['settingsAgents'];
  agentDiscovery: typeof defaultSettingsState['agentDiscovery'];
  availableAgents: typeof defaultSettingsState['availableAgents'];
  agentProfiles: typeof defaultSettingsState['agentProfiles'];
  editors: typeof defaultSettingsState['editors'];
  prompts: typeof defaultSettingsState['prompts'];
  notificationProviders: typeof defaultSettingsState['notificationProviders'];
  settingsData: typeof defaultSettingsState['settingsData'];
  userSettings: typeof defaultSettingsState['userSettings'];

  // Session slice
  messages: typeof defaultSessionState['messages'];
  turns: typeof defaultSessionState['turns'];
  taskSessions: typeof defaultSessionState['taskSessions'];
  taskSessionsByTask: typeof defaultSessionState['taskSessionsByTask'];
  sessionAgentctl: typeof defaultSessionState['sessionAgentctl'];
  worktrees: typeof defaultSessionState['worktrees'];
  sessionWorktreesBySessionId: typeof defaultSessionState['sessionWorktreesBySessionId'];
  pendingModel: typeof defaultSessionState['pendingModel'];
  activeModel: typeof defaultSessionState['activeModel'];
  taskPlans: typeof defaultSessionState['taskPlans'];

  // Session Runtime slice
  terminal: typeof defaultSessionRuntimeState['terminal'];
  shell: typeof defaultSessionRuntimeState['shell'];
  processes: typeof defaultSessionRuntimeState['processes'];
  gitStatus: typeof defaultSessionRuntimeState['gitStatus'];
  gitSnapshots: typeof defaultSessionRuntimeState['gitSnapshots'];
  sessionCommits: typeof defaultSessionRuntimeState['sessionCommits'];
  contextWindow: typeof defaultSessionRuntimeState['contextWindow'];
  agents: typeof defaultSessionRuntimeState['agents'];
  availableCommands: typeof defaultSessionRuntimeState['availableCommands'];

  // UI slice
  previewPanel: typeof defaultUIState['previewPanel'];
  rightPanel: typeof defaultUIState['rightPanel'];
  diffs: typeof defaultUIState['diffs'];
  connection: typeof defaultUIState['connection'];
  mobileKanban: typeof defaultUIState['mobileKanban'];

  // Actions from all slices
  hydrate: (state: Partial<AppState>, options?: HydrationOptions) => void;
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState['items']) => void;
  setActiveBoard: (boardId: string | null) => void;
  setBoards: (boards: BoardState['items']) => void;
  setExecutors: (executors: ExecutorsState['items']) => void;
  setEnvironments: (environments: EnvironmentsState['items']) => void;
  setSettingsAgents: (agents: SettingsAgentsState['items']) => void;
  setAgentDiscovery: (agents: AgentDiscoveryState['items']) => void;
  setAvailableAgents: (agents: AvailableAgentsState['items']) => void;
  setAvailableAgentsLoading: (loading: boolean) => void;
  setAgentProfiles: (profiles: AgentProfilesState['items']) => void;
  setRepositories: (workspaceId: string, repositories: Repository[]) => void;
  setRepositoriesLoading: (workspaceId: string, loading: boolean) => void;
  setRepositoryBranches: (repositoryId: string, branches: Branch[]) => void;
  setRepositoryBranchesLoading: (repositoryId: string, loading: boolean) => void;
  setSettingsData: (next: Partial<SettingsDataState>) => void;
  setEditors: (editors: EditorsState['items']) => void;
  setEditorsLoading: (loading: boolean) => void;
  setPrompts: (prompts: PromptsState['items']) => void;
  setPromptsLoading: (loading: boolean) => void;
  setNotificationProviders: (state: NotificationProvidersState) => void;
  setNotificationProvidersLoading: (loading: boolean) => void;
  setUserSettings: (settings: UserSettingsState) => void;
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
  setPreviewOpen: (sessionId: string, open: boolean) => void;
  togglePreviewOpen: (sessionId: string) => void;
  setPreviewView: (sessionId: string, view: PreviewViewMode) => void;
  setPreviewDevice: (sessionId: string, device: PreviewDevicePreset) => void;
  setPreviewStage: (sessionId: string, stage: PreviewStage) => void;
  setPreviewUrl: (sessionId: string, url: string) => void;
  setPreviewUrlDraft: (sessionId: string, url: string) => void;
  setRightPanelActiveTab: (sessionId: string, tab: string) => void;
  setConnectionStatus: (status: ConnectionState['status'], error?: string | null) => void;
  setMobileKanbanColumnIndex: (index: number) => void;
  setMobileKanbanMenuOpen: (open: boolean) => void;
  setMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null }
  ) => void;
  addMessage: (message: Message) => void;
  addTurn: (turn: Turn) => void;
  completeTurn: (sessionId: string, turnId: string, completedAt: string) => void;
  setActiveTurn: (sessionId: string, turnId: string | null) => void;
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
  setGitSnapshots: (sessionId: string, snapshots: GitSnapshot[]) => void;
  setGitSnapshotsLoading: (sessionId: string, loading: boolean) => void;
  addGitSnapshot: (sessionId: string, snapshot: GitSnapshot) => void;
  setSessionCommits: (sessionId: string, commits: SessionCommit[]) => void;
  setSessionCommitsLoading: (sessionId: string, loading: boolean) => void;
  addSessionCommit: (sessionId: string, commit: SessionCommit) => void;
  clearSessionCommits: (sessionId: string) => void;
  setContextWindow: (sessionId: string, contextWindow: ContextWindowEntry) => void;
  bumpAgentProfilesVersion: () => void;
  setPendingModel: (sessionId: string, modelId: string) => void;
  clearPendingModel: (sessionId: string) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  // Task plan actions
  setTaskPlan: (taskId: string, plan: import('@/lib/types/http').TaskPlan | null) => void;
  setTaskPlanLoading: (taskId: string, loading: boolean) => void;
  setTaskPlanSaving: (taskId: string, saving: boolean) => void;
  clearTaskPlan: (taskId: string) => void;
  // Available commands actions
  setAvailableCommands: (sessionId: string, commands: import('./slices/session-runtime/types').AvailableCommand[]) => void;
  clearAvailableCommands: (sessionId: string) => void;
};

export type AppStore = ReturnType<typeof createAppStore>;

const defaultState = {
  kanban: defaultKanbanState.kanban,
  boards: defaultKanbanState.boards,
  tasks: defaultKanbanState.tasks,
  workspaces: defaultWorkspaceState.workspaces,
  repositories: defaultWorkspaceState.repositories,
  repositoryBranches: defaultWorkspaceState.repositoryBranches,
  executors: defaultSettingsState.executors,
  environments: defaultSettingsState.environments,
  settingsAgents: defaultSettingsState.settingsAgents,
  agentDiscovery: defaultSettingsState.agentDiscovery,
  availableAgents: defaultSettingsState.availableAgents,
  agentProfiles: defaultSettingsState.agentProfiles,
  editors: defaultSettingsState.editors,
  prompts: defaultSettingsState.prompts,
  notificationProviders: defaultSettingsState.notificationProviders,
  settingsData: defaultSettingsState.settingsData,
  userSettings: defaultSettingsState.userSettings,
  messages: defaultSessionState.messages,
  turns: defaultSessionState.turns,
  taskSessions: defaultSessionState.taskSessions,
  taskSessionsByTask: defaultSessionState.taskSessionsByTask,
  sessionAgentctl: defaultSessionState.sessionAgentctl,
  worktrees: defaultSessionState.worktrees,
  sessionWorktreesBySessionId: defaultSessionState.sessionWorktreesBySessionId,
  pendingModel: defaultSessionState.pendingModel,
  activeModel: defaultSessionState.activeModel,
  taskPlans: defaultSessionState.taskPlans,
  terminal: defaultSessionRuntimeState.terminal,
  shell: defaultSessionRuntimeState.shell,
  processes: defaultSessionRuntimeState.processes,
  gitStatus: defaultSessionRuntimeState.gitStatus,
  gitSnapshots: defaultSessionRuntimeState.gitSnapshots,
  sessionCommits: defaultSessionRuntimeState.sessionCommits,
  contextWindow: defaultSessionRuntimeState.contextWindow,
  agents: defaultSessionRuntimeState.agents,
  availableCommands: defaultSessionRuntimeState.availableCommands,
  previewPanel: defaultUIState.previewPanel,
  rightPanel: defaultUIState.rightPanel,
  diffs: defaultUIState.diffs,
  connection: defaultUIState.connection,
  mobileKanban: defaultUIState.mobileKanban,
};

function mergeInitialState(initialState?: Partial<AppState>): typeof defaultState {
  if (!initialState) return defaultState;

  return {
    ...defaultState,
    ...initialState,
    // Ensure nested objects are properly merged
    kanban: { ...defaultState.kanban, ...initialState.kanban },
    boards: { ...defaultState.boards, ...initialState.boards },
    tasks: { ...defaultState.tasks, ...initialState.tasks },
    workspaces: { ...defaultState.workspaces, ...initialState.workspaces },
    repositories: { ...defaultState.repositories, ...initialState.repositories },
    repositoryBranches: { ...defaultState.repositoryBranches, ...initialState.repositoryBranches },
    executors: { ...defaultState.executors, ...initialState.executors },
    environments: { ...defaultState.environments, ...initialState.environments },
    settingsAgents: { ...defaultState.settingsAgents, ...initialState.settingsAgents },
    agentDiscovery: { ...defaultState.agentDiscovery, ...initialState.agentDiscovery },
    availableAgents: { ...defaultState.availableAgents, ...initialState.availableAgents },
    agentProfiles: { ...defaultState.agentProfiles, ...initialState.agentProfiles },
    editors: { ...defaultState.editors, ...initialState.editors },
    prompts: { ...defaultState.prompts, ...initialState.prompts },
    notificationProviders: { ...defaultState.notificationProviders, ...initialState.notificationProviders },
    settingsData: { ...defaultState.settingsData, ...initialState.settingsData },
    userSettings: { ...defaultState.userSettings, ...initialState.userSettings },
    messages: { ...defaultState.messages, ...initialState.messages },
    turns: { ...defaultState.turns, ...initialState.turns },
    taskSessions: { ...defaultState.taskSessions, ...initialState.taskSessions },
    taskSessionsByTask: { ...defaultState.taskSessionsByTask, ...initialState.taskSessionsByTask },
    sessionAgentctl: { ...defaultState.sessionAgentctl, ...initialState.sessionAgentctl },
    worktrees: { ...defaultState.worktrees, ...initialState.worktrees },
    sessionWorktreesBySessionId: { ...defaultState.sessionWorktreesBySessionId, ...initialState.sessionWorktreesBySessionId },
    pendingModel: { ...defaultState.pendingModel, ...initialState.pendingModel },
    activeModel: { ...defaultState.activeModel, ...initialState.activeModel },
    taskPlans: { ...defaultState.taskPlans, ...initialState.taskPlans },
    terminal: { ...defaultState.terminal, ...initialState.terminal },
    shell: { ...defaultState.shell, ...initialState.shell },
    processes: { ...defaultState.processes, ...initialState.processes },
    gitStatus: { ...defaultState.gitStatus, ...initialState.gitStatus },
    gitSnapshots: { ...defaultState.gitSnapshots, ...initialState.gitSnapshots },
    sessionCommits: { ...defaultState.sessionCommits, ...initialState.sessionCommits },
    contextWindow: { ...defaultState.contextWindow, ...initialState.contextWindow },
    agents: { ...defaultState.agents, ...initialState.agents },
    previewPanel: { ...defaultState.previewPanel, ...initialState.previewPanel },
    rightPanel: { ...defaultState.rightPanel, ...initialState.rightPanel },
    diffs: { ...defaultState.diffs, ...initialState.diffs },
    connection: { ...defaultState.connection, ...initialState.connection },
    mobileKanban: { ...defaultState.mobileKanban, ...initialState.mobileKanban },
  };
}

export function createAppStore(initialState?: Partial<AppState>) {
  const merged = mergeInitialState(initialState);

  return createStore<AppState>()(
    immer((set, get, api) => ({
      ...merged,
      // Compose all slices
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createKanbanSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createWorkspaceSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSettingsSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSessionSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSessionRuntimeSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createUISlice(set as any, get as any, api as any),
      // Override state with merged initial state
      kanban: merged.kanban,
      boards: merged.boards,
      tasks: merged.tasks,
      workspaces: merged.workspaces,
      repositories: merged.repositories,
      repositoryBranches: merged.repositoryBranches,
      executors: merged.executors,
      environments: merged.environments,
      settingsAgents: merged.settingsAgents,
      agentDiscovery: merged.agentDiscovery,
      availableAgents: merged.availableAgents,
      agentProfiles: merged.agentProfiles,
      editors: merged.editors,
      prompts: merged.prompts,
      notificationProviders: merged.notificationProviders,
      settingsData: merged.settingsData,
      userSettings: merged.userSettings,
      messages: merged.messages,
      turns: merged.turns,
      taskSessions: merged.taskSessions,
      taskSessionsByTask: merged.taskSessionsByTask,
      sessionAgentctl: merged.sessionAgentctl,
      worktrees: merged.worktrees,
      sessionWorktreesBySessionId: merged.sessionWorktreesBySessionId,
      pendingModel: merged.pendingModel,
      activeModel: merged.activeModel,
      terminal: merged.terminal,
      shell: merged.shell,
      processes: merged.processes,
      gitStatus: merged.gitStatus,
      contextWindow: merged.contextWindow,
      agents: merged.agents,
      previewPanel: merged.previewPanel,
      rightPanel: merged.rightPanel,
      diffs: merged.diffs,
      connection: merged.connection,
      mobileKanban: merged.mobileKanban,
      // Add hydrate method
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      hydrate: (state, options) => set((draft) => hydrateState(draft as any, state, options)),
    }))
  );
}

export type StoreProviderProps = {
  children: React.ReactNode;
  initialState?: Partial<AppState>;
};
