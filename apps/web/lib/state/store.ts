import { createStore } from "zustand/vanilla";
import { immer } from "zustand/middleware/immer";
import { hydrateState, type HydrationOptions } from "./hydration/hydrator";
import type {
  Repository,
  Branch,
  RepositoryScript,
  Message,
  Turn,
  TaskSession,
  TaskWalkthrough,
} from "@/lib/types/http";
import type { SystemHealthResponse } from "@/lib/types/health";
import type { UISliceActions as UIA } from "./slices/ui/types";
import type * as UISliceTypes from "./slices/ui/types";
import { mergeInitialState } from "./default-state";
import { buildStateOverrides } from "./store-overrides";
import {
  createKanbanSlice,
  createWorkspaceSlice,
  createSettingsSlice,
  createSessionSlice,
  createSessionRuntimeSlice,
  createUISlice,
  createGitHubSlice,
  createGitLabSlice,
  createJiraSlice,
  createLinearSlice,
  createOfficeSlice,
  createFeaturesSlice,
  createAutomationsSlice,
  createSystemSlice,
  defaultKanbanState,
  defaultWorkspaceState,
  defaultSettingsState,
  defaultSessionState,
  defaultSessionRuntimeState,
  defaultUIState,
  defaultGitHubState,
  defaultGitLabState,
  defaultJiraState,
  defaultLinearState,
  defaultOfficeState,
  defaultFeaturesState,
  defaultAutomationsState,
  defaultSystemState,
  type WorkspaceState,
  type WorkflowsState,
  type ExecutorsState,
  type SettingsAgentsState,
  type AgentDiscoveryState,
  type AvailableAgentsState,
  type AgentProfilesState,
  type EditorsState,
  type PromptsState,
  type SecretsState,
  type NotificationProvidersState,
  type SettingsDataState,
  type UserSettingsState,
  type ProcessStatusEntry,
  type Worktree,
  type GitStatusEntry,
  type SessionCommit,
  type ContextWindowEntry,
  type SessionAgentctlStatus,
  type PreviewStage,
  type PreviewViewMode,
  type PreviewDevicePreset,
  type ConnectionState,
  type SystemSliceActions,
  type AutomationsSliceActions,
  type FeaturesSliceActions,
  type GitHubSliceActions,
} from "./slices";
import type {
  AvailableCommand,
  SessionModeEntry,
  AgentCapabilitiesEntry,
  SessionModelEntry,
  ConfigOptionEntry,
  PromptUsageEntry,
  SessionPollMode,
  TodoEntry,
  UserShellInfo,
} from "./slices/session-runtime/types";

// Re-export all types from slices for backwards compatibility.
export type * from "./store-reexports";
import type { TaskMR } from "@/lib/types/gitlab";
import type { GitLabSliceActions } from "./slices/gitlab/types";
import type { JiraIssueWatch } from "@/lib/types/jira";
import type { LinearIssueWatch } from "@/lib/types/linear";
import type {
  AgentProfile,
  AgentRoutePreview,
  AgentRouteData,
  Skill,
  Project,
  Approval,
  ActivityEntry,
  CostSummary,
  BudgetPolicy,
  Routine,
  InboxItem,
  OfficeMeta,
  ProviderHealth,
  RouteAttempt,
  Run,
  DashboardData,
  OfficeTask,
  TaskFilterState,
  TaskViewMode,
  TaskSortField,
  TaskSortDir,
  TaskGroupBy,
  WorkspaceRouting,
} from "./slices/office/types";

// Combined AppState type
export type AppState = {
  // Kanban slice
  kanban: (typeof defaultKanbanState)["kanban"];
  kanbanMulti: (typeof defaultKanbanState)["kanbanMulti"];
  workflows: (typeof defaultKanbanState)["workflows"];
  tasks: (typeof defaultKanbanState)["tasks"];

  // Workspace slice
  workspaces: (typeof defaultWorkspaceState)["workspaces"];
  repositories: (typeof defaultWorkspaceState)["repositories"];
  repositoryBranches: (typeof defaultWorkspaceState)["repositoryBranches"];
  repositoryScripts: (typeof defaultWorkspaceState)["repositoryScripts"];

  // Settings slice
  executors: (typeof defaultSettingsState)["executors"];
  settingsAgents: (typeof defaultSettingsState)["settingsAgents"];
  agentDiscovery: (typeof defaultSettingsState)["agentDiscovery"];
  availableAgents: (typeof defaultSettingsState)["availableAgents"];
  agentProfiles: (typeof defaultSettingsState)["agentProfiles"];
  installJobs: (typeof defaultSettingsState)["installJobs"];
  editors: (typeof defaultSettingsState)["editors"];
  prompts: (typeof defaultSettingsState)["prompts"];
  secrets: (typeof defaultSettingsState)["secrets"];
  sprites: (typeof defaultSettingsState)["sprites"];
  notificationProviders: (typeof defaultSettingsState)["notificationProviders"];
  settingsData: (typeof defaultSettingsState)["settingsData"];
  userSettings: (typeof defaultSettingsState)["userSettings"];

  // Session slice
  messages: (typeof defaultSessionState)["messages"];
  turns: (typeof defaultSessionState)["turns"];
  taskSessions: (typeof defaultSessionState)["taskSessions"];
  taskSessionsByTask: (typeof defaultSessionState)["taskSessionsByTask"];
  sessionAgentctl: (typeof defaultSessionState)["sessionAgentctl"];
  worktrees: (typeof defaultSessionState)["worktrees"];
  sessionWorktreesBySessionId: (typeof defaultSessionState)["sessionWorktreesBySessionId"];
  pendingModel: (typeof defaultSessionState)["pendingModel"];
  activeModel: (typeof defaultSessionState)["activeModel"];
  taskPlans: (typeof defaultSessionState)["taskPlans"];
  walkthroughs: (typeof defaultSessionState)["walkthroughs"];
  queue: (typeof defaultSessionState)["queue"];

  // Session Runtime slice
  terminal: (typeof defaultSessionRuntimeState)["terminal"];
  shell: (typeof defaultSessionRuntimeState)["shell"];
  processes: (typeof defaultSessionRuntimeState)["processes"];
  gitStatus: (typeof defaultSessionRuntimeState)["gitStatus"];
  environmentIdBySessionId: (typeof defaultSessionRuntimeState)["environmentIdBySessionId"];
  sessionCommits: (typeof defaultSessionRuntimeState)["sessionCommits"];
  contextWindow: (typeof defaultSessionRuntimeState)["contextWindow"];
  agents: (typeof defaultSessionRuntimeState)["agents"];
  availableCommands: (typeof defaultSessionRuntimeState)["availableCommands"];
  sessionMode: (typeof defaultSessionRuntimeState)["sessionMode"];
  userShells: (typeof defaultSessionRuntimeState)["userShells"];
  prepareProgress: (typeof defaultSessionRuntimeState)["prepareProgress"];
  sessionTodos: (typeof defaultSessionRuntimeState)["sessionTodos"];
  agentCapabilities: (typeof defaultSessionRuntimeState)["agentCapabilities"];
  sessionModels: (typeof defaultSessionRuntimeState)["sessionModels"];
  promptUsage: (typeof defaultSessionRuntimeState)["promptUsage"];
  sessionPollMode: (typeof defaultSessionRuntimeState)["sessionPollMode"];

  // GitHub slice
  githubStatus: (typeof defaultGitHubState)["githubStatus"];
  taskPRs: (typeof defaultGitHubState)["taskPRs"];
  taskIssues: (typeof defaultGitHubState)["taskIssues"];
  pendingPrUrlByTaskId: (typeof defaultGitHubState)["pendingPrUrlByTaskId"];
  prWatches: (typeof defaultGitHubState)["prWatches"];
  reviewWatches: (typeof defaultGitHubState)["reviewWatches"];
  issueWatches: (typeof defaultGitHubState)["issueWatches"];
  actionPresets: (typeof defaultGitHubState)["actionPresets"];
  prFeedbackCache: (typeof defaultGitHubState)["prFeedbackCache"];
  taskCIAutomation: (typeof defaultGitHubState)["taskCIAutomation"];

  // GitLab slice
  taskMRs: (typeof defaultGitLabState)["taskMRs"];
  gitlabReviewWatches: (typeof defaultGitLabState)["gitlabReviewWatches"];
  gitlabIssueWatches: (typeof defaultGitLabState)["gitlabIssueWatches"];
  gitlabMRWatches: (typeof defaultGitLabState)["gitlabMRWatches"];
  gitlabActionPresets: (typeof defaultGitLabState)["gitlabActionPresets"];
  gitlabStats: (typeof defaultGitLabState)["gitlabStats"];
  gitlabStatus: (typeof defaultGitLabState)["gitlabStatus"];

  // JIRA slice
  jiraIssueWatches: (typeof defaultJiraState)["jiraIssueWatches"];

  // Linear slice
  linearIssueWatches: (typeof defaultLinearState)["linearIssueWatches"];

  // Office slice
  office: (typeof defaultOfficeState)["office"];

  // Feature flags slice
  features: (typeof defaultFeaturesState)["features"];

  // Automations slice
  automations: (typeof defaultAutomationsState)["automations"];
  automationRuns: (typeof defaultAutomationsState)["automationRuns"];

  // System slice (actions merged via SystemSliceActions intersection on AppState)
  system: (typeof defaultSystemState)["system"];

  // UI slice
  previewPanel: (typeof defaultUIState)["previewPanel"];
  rightPanel: (typeof defaultUIState)["rightPanel"];
  diffs: (typeof defaultUIState)["diffs"];
  connection: (typeof defaultUIState)["connection"];
  mobileKanban: (typeof defaultUIState)["mobileKanban"];
  mobileSession: (typeof defaultUIState)["mobileSession"];
  chatInput: (typeof defaultUIState)["chatInput"];
  documentPanel: (typeof defaultUIState)["documentPanel"];
  systemHealth: (typeof defaultUIState)["systemHealth"];
  quickChat: (typeof defaultUIState)["quickChat"];
  configChat: (typeof defaultUIState)["configChat"];
  sessionFailureNotification: (typeof defaultUIState)["sessionFailureNotification"];
  taskDeletedNotification: (typeof defaultUIState)["taskDeletedNotification"];
  bottomTerminal: (typeof defaultUIState)["bottomTerminal"];
  sidebarViews: (typeof defaultUIState)["sidebarViews"];
  collapsedSubtaskParents: (typeof defaultUIState)["collapsedSubtaskParents"];
  kanbanPreviewedTaskId: (typeof defaultUIState)["kanbanPreviewedTaskId"];
  sidebarTaskPrefs: (typeof defaultUIState)["sidebarTaskPrefs"];
  appSidebar: (typeof defaultUIState)["appSidebar"];
  acknowledgedAgentErrors: (typeof defaultUIState)["acknowledgedAgentErrors"];
  dismissedAgentErrors: (typeof defaultUIState)["dismissedAgentErrors"];

  // GitLab actions
  setTaskMRs: (mrs: Record<string, TaskMR[]>) => void;
  setTaskMR: (taskId: string, mr: TaskMR) => void;
  resetTaskMRs: () => void;
  setGitLabReviewWatches: GitLabSliceActions["setGitLabReviewWatches"];
  setGitLabReviewWatchesLoading: GitLabSliceActions["setGitLabReviewWatchesLoading"];
  addGitLabReviewWatch: GitLabSliceActions["addGitLabReviewWatch"];
  updateGitLabReviewWatchInStore: GitLabSliceActions["updateGitLabReviewWatchInStore"];
  removeGitLabReviewWatch: GitLabSliceActions["removeGitLabReviewWatch"];
  setGitLabIssueWatches: GitLabSliceActions["setGitLabIssueWatches"];
  setGitLabIssueWatchesLoading: GitLabSliceActions["setGitLabIssueWatchesLoading"];
  addGitLabIssueWatch: GitLabSliceActions["addGitLabIssueWatch"];
  updateGitLabIssueWatchInStore: GitLabSliceActions["updateGitLabIssueWatchInStore"];
  removeGitLabIssueWatch: GitLabSliceActions["removeGitLabIssueWatch"];
  setGitLabMRWatches: GitLabSliceActions["setGitLabMRWatches"];
  setGitLabMRWatchesLoading: GitLabSliceActions["setGitLabMRWatchesLoading"];
  removeGitLabMRWatch: GitLabSliceActions["removeGitLabMRWatch"];
  setGitLabActionPresets: GitLabSliceActions["setGitLabActionPresets"];
  setGitLabActionPresetsLoading: GitLabSliceActions["setGitLabActionPresetsLoading"];
  setGitLabStats: GitLabSliceActions["setGitLabStats"];
  setGitLabStatsLoading: GitLabSliceActions["setGitLabStatsLoading"];
  setGitLabStatus: GitLabSliceActions["setGitLabStatus"];
  setGitLabStatusLoading: GitLabSliceActions["setGitLabStatusLoading"];

  // JIRA actions
  setJiraIssueWatches: (watches: JiraIssueWatch[]) => void;
  setJiraIssueWatchesLoading: (loading: boolean) => void;
  addJiraIssueWatch: (watch: JiraIssueWatch) => void;
  updateJiraIssueWatch: (watch: JiraIssueWatch) => void;
  removeJiraIssueWatch: (id: string) => void;
  resetJiraIssueWatches: () => void;

  // Linear actions
  setLinearIssueWatches: (watches: LinearIssueWatch[]) => void;
  setLinearIssueWatchesLoading: (loading: boolean) => void;
  addLinearIssueWatch: (watch: LinearIssueWatch) => void;
  updateLinearIssueWatch: (watch: LinearIssueWatch) => void;
  removeLinearIssueWatch: (id: string) => void;
  resetLinearIssueWatches: () => void;

  // Actions from all slices
  hydrate: (state: Partial<AppState>, options?: HydrationOptions) => void;
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState["items"]) => void;
  setActiveWorkflow: (workflowId: string | null) => void;
  setWorkflows: (workflows: WorkflowsState["items"]) => void;
  reorderWorkflowItems: (workflowIds: string[]) => void;
  setWorkflowSnapshot: (
    workflowId: string,
    data: import("./slices/kanban/types").WorkflowSnapshotData,
  ) => void;
  setKanbanMultiLoading: (loading: boolean) => void;
  clearKanbanMulti: () => void;
  updateMultiTask: (
    workflowId: string,
    task: import("./slices/kanban/types").KanbanState["tasks"][number],
  ) => void;
  removeMultiTask: (workflowId: string, taskId: string) => void;
  setExecutors: (executors: ExecutorsState["items"]) => void;
  setSettingsAgents: (agents: SettingsAgentsState["items"]) => void;
  setAgentDiscovery: (agents: AgentDiscoveryState["items"]) => void;
  setAgentDiscoveryLoading: (loading: boolean) => void;
  setAvailableAgents: (
    agents: AvailableAgentsState["items"],
    tools?: AvailableAgentsState["tools"],
  ) => void;
  setAvailableAgentsLoading: (loading: boolean) => void;
  setAgentProfiles: (profiles: AgentProfilesState["items"]) => void;
  setInstallJobs: (jobs: import("@/lib/state/slices/settings/types").InstallJob[]) => void;
  upsertInstallJob: (job: import("@/lib/state/slices/settings/types").InstallJob) => void;
  appendInstallOutput: (agentName: string, chunk: string) => void;
  clearInstallJob: (agentName: string) => void;
  setRepositories: (workspaceId: string, repositories: Repository[]) => void;
  setRepositoriesLoading: (workspaceId: string, loading: boolean) => void;
  setRepositoryBranches: (
    repositoryId: string,
    branches: Branch[],
    meta?: { fetchedAt?: string; fetchError?: string },
  ) => void;
  setRepositoryBranchesLoading: (repositoryId: string, loading: boolean) => void;
  setRepositoryBranchesFetchError: (repositoryId: string, error: string | undefined) => void;
  setRepositoryScripts: (repositoryId: string, scripts: RepositoryScript[]) => void;
  setRepositoryScriptsLoading: (repositoryId: string, loading: boolean) => void;
  clearRepositoryScripts: (repositoryId: string) => void;
  invalidateRepositories: (workspaceId: string) => void;
  setSettingsData: (next: Partial<SettingsDataState>) => void;
  setEditors: (editors: EditorsState["items"]) => void;
  setEditorsLoading: (loading: boolean) => void;
  setPrompts: (prompts: PromptsState["items"]) => void;
  setPromptsLoading: (loading: boolean) => void;
  setSecrets: (items: SecretsState["items"]) => void;
  setSecretsLoading: (loading: boolean) => void;
  addSecret: (item: import("@/lib/types/http-secrets").SecretListItem) => void;
  updateSecret: (item: import("@/lib/types/http-secrets").SecretListItem) => void;
  removeSecret: (id: string) => void;
  setSpritesStatus: (status: import("@/lib/types/http-sprites").SpritesStatus) => void;
  setSpritesInstances: (instances: import("@/lib/types/http-sprites").SpritesInstance[]) => void;
  setSpritesLoading: (loading: boolean) => void;
  removeSpritesInstance: (name: string) => void;
  setNotificationProviders: (state: NotificationProvidersState) => void;
  setNotificationProvidersLoading: (loading: boolean) => void;
  setUserSettings: (settings: UserSettingsState) => void;
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
  setPreviewOpen: (sessionId: string, open: boolean) => void;
  togglePreviewOpen: (sessionId: string) => void;
  setPreviewView: (sessionId: string, view: PreviewViewMode) => void;
  setPreviewDevice: (sessionId: string, device: PreviewDevicePreset) => void;
  setPreviewStage: (sessionId: string, stage: PreviewStage) => void;
  setPreviewUrl: (sessionId: string, url: string) => void;
  setPreviewUrlDraft: (sessionId: string, url: string) => void;
  setRightPanelActiveTab: (sessionId: string, tab: string) => void;
  setConnectionStatus: (status: ConnectionState["status"], error?: string | null) => void;
  setMobileKanbanColumnIndex: (index: number) => void;
  setMobileKanbanMenuOpen: (open: boolean) => void;
  setMobileKanbanSearchOpen: (open: boolean) => void;
  setMobileSessionPanel: (sessionId: string, panel: UISliceTypes.MobileSessionPanel) => void;
  setMobileSessionTaskSwitcherOpen: (open: boolean) => void;
  setPlanMode: (sessionId: string, enabled: boolean) => void;
  setActiveDocument: (sessionId: string, doc: UISliceTypes.ActiveDocument | null) => void;
  setSystemHealth: (response: SystemHealthResponse) => void;
  setSystemHealthLoading: (loading: boolean) => void;
  invalidateSystemHealth: () => void;
  openQuickChat: (sessionId: string, workspaceId: string, agentProfileId?: string) => void;
  closeQuickChat: () => void;
  closeQuickChatSession: (sessionId: string) => void;
  setActiveQuickChatSession: (sessionId: string) => void;
  renameQuickChatSession: (sessionId: string, name: string) => void;
  openConfigChat: (sessionId: string, workspaceId: string) => void;
  startNewConfigChat: (workspaceId: string) => void;
  closeConfigChat: () => void;
  closeConfigChatSession: (sessionId: string) => void;
  setActiveConfigChatSession: (sessionId: string) => void;
  renameConfigChatSession: (sessionId: string, name: string) => void;
  setSessionFailureNotification: (n: UISliceTypes.SessionFailureNotification | null) => void;
  setTaskDeletedNotification: (n: UISliceTypes.TaskDeletedNotification | null) => void;
  toggleBottomTerminal: () => void;
  openBottomTerminalWithCommand: (command: string) => void;
  clearBottomTerminalCommand: () => void;
  setMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
  addMessage: (message: Message) => void;
  mergeMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
  addTurn: (turn: Turn) => void;
  completeTurn: (
    sessionId: string,
    turnId: string,
    completedAt: string,
    metadata?: Record<string, unknown>,
  ) => void;
  setActiveTurn: (sessionId: string, turnId: string | null) => void;
  updateMessage: (message: Message) => void;
  removeMessage: (sessionId: string, messageId: string) => void;
  prependMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
  setMessagesMetadata: (
    sessionId: string,
    meta: { hasMore?: boolean; isLoading?: boolean; oldestCursor?: string | null },
  ) => void;
  setMessagesLoading: (sessionId: string, loading: boolean) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveSessionAuto: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
  clearActiveSession: () => void;
  setTaskSession: (session: TaskSession) => void;
  removeTaskSession: (taskId: string, sessionId: string) => void;
  setTaskSessionsForTask: (taskId: string, sessions: TaskSession[]) => void;
  upsertTaskSessionFromEvent: (taskId: string, session: TaskSession) => void;
  setTaskSessionsLoading: (taskId: string, loading: boolean) => void;
  setSessionAgentctlStatus: (sessionId: string, status: SessionAgentctlStatus) => void;
  setWorktree: (worktree: Worktree) => void;
  setSessionWorktrees: (sessionId: string, worktreeIds: string[]) => void;
  setGitStatus: (sessionId: string, gitStatus: GitStatusEntry) => boolean;
  clearGitStatus: (sessionId: string) => void;
  clearLegacyGitStatusEntry: (sessionId: string) => void;
  registerSessionEnvironment: (sessionId: string, environmentId: string) => void;
  setSessionCommits: (
    sessionId: string,
    commits: SessionCommit[],
    opts?: { allowEmpty?: boolean },
  ) => void;
  setSessionCommitsLoading: (sessionId: string, loading: boolean) => void;
  addSessionCommit: (sessionId: string, commit: SessionCommit) => void;
  clearSessionCommits: (sessionId: string) => void;
  bumpSessionCommitsRefetch: (sessionId: string) => void;
  setContextWindow: (sessionId: string, contextWindow: ContextWindowEntry) => void;
  clearContextWindow: (sessionId: string) => void;
  bumpAgentProfilesVersion: () => void;
  setPendingModel: (sessionId: string, modelId: string) => void;
  clearPendingModel: (sessionId: string) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  // Task plan actions
  setTaskPlan: (taskId: string, plan: import("@/lib/types/http").TaskPlan | null) => void;
  setTaskPlanLoading: (taskId: string, loading: boolean) => void;
  setTaskPlanSaving: (taskId: string, saving: boolean) => void;
  clearTaskPlan: (taskId: string) => void;
  markTaskPlanSeen: (taskId: string) => void;
  // Plan revision actions
  setPlanRevisions: (
    taskId: string,
    revisions: import("@/lib/types/http").TaskPlanRevision[],
  ) => void;
  upsertPlanRevision: (
    taskId: string,
    revision: import("@/lib/types/http").TaskPlanRevision,
  ) => void;
  setPlanRevisionsLoading: (taskId: string, loading: boolean) => void;
  cachePlanRevisionContent: (revisionId: string, content: string) => void;
  // Plan revision preview + compare actions
  setPreviewRevision: (taskId: string, revisionId: string | null) => void;
  toggleComparePair: (taskId: string, revisionId: string) => void;
  clearComparePair: (taskId: string) => void;
  // Walkthrough actions
  setWalkthrough: (taskId: string, walkthrough: TaskWalkthrough | null) => void;
  setWalkthroughActiveStep: (taskId: string, stepIndex: number) => void;
  markWalkthroughSeen: (taskId: string) => void;
  // Queue actions
  setQueueEntries: (
    sessionId: string,
    entries: import("./slices/session/types").QueuedMessage[],
    meta: import("./slices/session/types").QueueMeta,
  ) => void;
  removeQueueEntry: (sessionId: string, entryId: string) => void;
  setQueueLoading: (sessionId: string, loading: boolean) => void;
  clearQueueStatus: (sessionId: string) => void;
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
  // User shells actions
  setUserShells: (sessionId: string, shells: UserShellInfo[]) => void;
  setUserShellsLoading: (sessionId: string, loading: boolean) => void;
  addUserShell: (sessionId: string, shell: UserShellInfo) => void;
  removeUserShell: (sessionId: string, terminalId: string) => void;
  updateUserShell: (
    environmentId: string,
    terminalId: string,
    patch: Partial<Omit<UserShellInfo, "terminalId">>,
  ) => void;
  setSessionPollMode: (sessionId: string, mode: SessionPollMode) => void;
  /* prettier-ignore */ setSidebarActiveView: UIA["setSidebarActiveView"];
  updateSidebarDraft: UIA["updateSidebarDraft"];
  saveSidebarDraftAs: UIA["saveSidebarDraftAs"];
  saveSidebarDraftOverwrite: UIA["saveSidebarDraftOverwrite"];
  discardSidebarDraft: UIA["discardSidebarDraft"];
  deleteSidebarView: UIA["deleteSidebarView"];
  renameSidebarView: UIA["renameSidebarView"];
  duplicateSidebarView: UIA["duplicateSidebarView"];
  reorderSidebarViews: UIA["reorderSidebarViews"];
  toggleSidebarGroupCollapsed: UIA["toggleSidebarGroupCollapsed"];
  toggleSubtaskCollapsed: UIA["toggleSubtaskCollapsed"];
  clearSidebarSyncError: UIA["clearSidebarSyncError"];
  clearSidebarTaskPrefsSyncError: UIA["clearSidebarTaskPrefsSyncError"];
  setKanbanPreviewedTaskId: UIA["setKanbanPreviewedTaskId"];
  togglePinnedTask: UIA["togglePinnedTask"];
  pinTasks: UIA["pinTasks"];
  unpinTasks: UIA["unpinTasks"];
  setSidebarTaskOrder: UIA["setSidebarTaskOrder"];
  setSubtaskOrder: UIA["setSubtaskOrder"];
  removeTaskFromSidebarPrefs: UIA["removeTaskFromSidebarPrefs"];
  toggleAppSidebar: UIA["toggleAppSidebar"];
  setAppSidebarCollapsed: UIA["setAppSidebarCollapsed"];
  toggleAppSidebarSection: UIA["toggleAppSidebarSection"];
  setAppSidebarWidth: UIA["setAppSidebarWidth"];
  setAppSidebarSettingsMode: UIA["setAppSidebarSettingsMode"];
  toggleAppSidebarSettingsMode: UIA["toggleAppSidebarSettingsMode"];
  acknowledgeAgentErrors: UIA["acknowledgeAgentErrors"];
  dismissAgentError: UIA["dismissAgentError"];
  // Office actions
  setOfficeAgentProfiles: (agents: AgentProfile[]) => void;
  addOfficeAgentProfile: (agent: AgentProfile) => void;
  updateOfficeAgentProfile: (id: string, patch: Partial<AgentProfile>) => void;
  removeOfficeAgentProfile: (id: string) => void;
  setSkills: (skills: Skill[]) => void;
  addSkill: (skill: Skill) => void;
  updateSkill: (id: string, patch: Partial<Skill>) => void;
  removeSkill: (id: string) => void;
  setProjects: (projects: Project[]) => void;
  addProject: (project: Project) => void;
  updateProject: (id: string, patch: Partial<Project>) => void;
  removeProject: (id: string) => void;
  setApprovals: (approvals: Approval[]) => void;
  setActivity: (entries: ActivityEntry[]) => void;
  setCostSummary: (summary: CostSummary | null) => void;
  setBudgetPolicies: (policies: BudgetPolicy[]) => void;
  setRoutines: (routines: Routine[]) => void;
  setInboxItems: (items: InboxItem[]) => void;
  setInboxCount: (count: number) => void;
  setRuns: (runs: Run[]) => void;
  setDashboard: (data: DashboardData | null) => void;
  setTasks: (tasks: OfficeTask[]) => void;
  appendTasks: (tasks: OfficeTask[]) => void;
  patchTaskInStore: (taskId: string, patch: Partial<OfficeTask>) => void;
  setTaskFilters: (filters: Partial<TaskFilterState>) => void;
  setTaskViewMode: (mode: TaskViewMode) => void;
  setTaskSortField: (field: TaskSortField) => void;
  setTaskSortDir: (dir: TaskSortDir) => void;
  setTaskGroupBy: (groupBy: TaskGroupBy) => void;
  toggleNesting: () => void;
  setTasksLoading: (loading: boolean) => void;
  setMeta: (meta: OfficeMeta | null) => void;
  setOfficeLoading: (loading: boolean) => void;
  setOfficeRefetchTrigger: (type: string) => void;
  setWorkspaceRouting: (workspaceId: string, cfg: WorkspaceRouting | undefined) => void;
  setKnownProviders: (providers: string[]) => void;
  setRoutingPreview: (workspaceId: string, agents: AgentRoutePreview[]) => void;
  setProviderHealth: (workspaceId: string, health: ProviderHealth[]) => void;
  upsertProviderHealth: (workspaceId: string, row: ProviderHealth) => void;
  setRunAttempts: (runId: string, attempts: RouteAttempt[]) => void;
  appendRunAttempt: (runId: string, attempt: RouteAttempt) => void;
  setAgentRouting: (agentId: string, data: AgentRouteData | undefined) => void;
} & GitHubSliceActions &
  SystemSliceActions &
  FeaturesSliceActions &
  AutomationsSliceActions;

export function createAppStore(initialState?: Partial<AppState>) {
  const merged = mergeInitialState(initialState);

  return createStore<AppState>()(
    immer((set, get, api) => ({
      ...merged,
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
      ...createGitHubSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createGitLabSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createJiraSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createLinearSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createOfficeSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createFeaturesSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSystemSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createUISlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createAutomationsSlice(set as any, get as any, api as any),
      // Re-assert merged initial state so caller-supplied values win over slice defaults.
      ...buildStateOverrides(merged),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      hydrate: (state, options) => set((draft) => hydrateState(draft as any, state, options)),
    })),
  );
}

export type StoreProviderProps = {
  children: React.ReactNode;
  initialState?: Partial<AppState>;
};
