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
import type { AgentUpdateJob, InstallJob } from "./slices/settings/types";
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
  createAzureDevOpsSlice,
  createJiraSlice,
  createLinearSlice,
  createOfficeSlice,
  createFeaturesSlice,
  createAuthSlice,
  createAutomationsSlice,
  createSystemSlice,
  createPluginsSlice,
  createReviewSlice,
  defaultWorkspaceState,
  defaultSettingsState,
  defaultSessionState,
  defaultSessionRuntimeState,
  defaultUIState,
  defaultGitHubState,
  defaultGitLabState,
  defaultAzureDevOpsState,
  defaultJiraState,
  defaultLinearState,
  defaultOfficeState,
  defaultFeaturesState,
  defaultAuthState,
  defaultAutomationsState,
  defaultSystemState,
  defaultPluginsState,
  defaultReviewState,
  type WorkspaceState,
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
  type AuthSliceActions,
  type GitHubSliceActions,
  type GitLabSliceActions,
  type AzureDevOpsSliceActions,
  type JiraSliceActions,
  type LinearSliceActions,
  type OfficeSliceActions,
  type PluginsSliceActions,
  type ReviewSliceActions,
  type KanbanSlice,
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

// Combined AppState type
export type AppState = KanbanSlice & {
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
  updateJobs: (typeof defaultSettingsState)["updateJobs"];
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
  sessionMcpStatus: (typeof defaultSessionRuntimeState)["sessionMcpStatus"];
  promptUsage: (typeof defaultSessionRuntimeState)["promptUsage"];
  sessionPollMode: (typeof defaultSessionRuntimeState)["sessionPollMode"];
  embeddedVscodeSupport: (typeof defaultSessionRuntimeState)["embeddedVscodeSupport"];

  // GitHub slice
  githubStatus: (typeof defaultGitHubState)["githubStatus"];
  githubAppRegistrations: (typeof defaultGitHubState)["githubAppRegistrations"];
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

  // Azure DevOps slice
  azureDevOpsTaskPullRequests: (typeof defaultAzureDevOpsState)["azureDevOpsTaskPullRequests"];
  azureDevOpsTaskWorkItems: (typeof defaultAzureDevOpsState)["azureDevOpsTaskWorkItems"];

  // JIRA slice
  jiraIssueWatches: (typeof defaultJiraState)["jiraIssueWatches"];

  // Linear slice
  linearIssueWatches: (typeof defaultLinearState)["linearIssueWatches"];

  // Office slice
  office: (typeof defaultOfficeState)["office"];

  // Feature flags slice
  features: (typeof defaultFeaturesState)["features"];

  // Auth slice (actions merged via AuthSliceActions intersection on AppState)
  auth: (typeof defaultAuthState)["auth"];

  // Automations slice
  automations: (typeof defaultAutomationsState)["automations"];
  automationRuns: (typeof defaultAutomationsState)["automationRuns"];

  // System slice (actions merged via SystemSliceActions intersection on AppState)
  system: (typeof defaultSystemState)["system"];

  // Plugins slice (actions merged via PluginsSliceActions intersection on AppState)
  plugins: (typeof defaultPluginsState)["plugins"];

  // Review slice (actions merged via ReviewSliceActions intersection on AppState)
  taskReview: (typeof defaultReviewState)["taskReview"];

  // UI slice
  previewPanel: (typeof defaultUIState)["previewPanel"];
  rightPanel: (typeof defaultUIState)["rightPanel"];
  diffs: (typeof defaultUIState)["diffs"];
  connection: (typeof defaultUIState)["connection"];
  mobileKanban: (typeof defaultUIState)["mobileKanban"];
  mobileSession: (typeof defaultUIState)["mobileSession"];
  chatInput: (typeof defaultUIState)["chatInput"];
  transcriptAutoScroll: (typeof defaultUIState)["transcriptAutoScroll"];
  reviewPRSelection: (typeof defaultUIState)["reviewPRSelection"];
  documentPanel: (typeof defaultUIState)["documentPanel"];
  systemHealth: (typeof defaultUIState)["systemHealth"];
  quickChat: (typeof defaultUIState)["quickChat"];
  sessionFailureNotification: (typeof defaultUIState)["sessionFailureNotification"];
  taskDeletedNotification: (typeof defaultUIState)["taskDeletedNotification"];
  updateAvailableNotification: (typeof defaultUIState)["updateAvailableNotification"];
  bottomTerminal: (typeof defaultUIState)["bottomTerminal"];
  sidebarViews: (typeof defaultUIState)["sidebarViews"];
  collapsedSubtaskParents: (typeof defaultUIState)["collapsedSubtaskParents"];
  kanbanPreviewedTaskId: (typeof defaultUIState)["kanbanPreviewedTaskId"];
  sidebarTaskPrefs: (typeof defaultUIState)["sidebarTaskPrefs"];
  appSidebar: (typeof defaultUIState)["appSidebar"];
  acknowledgedAgentErrors: (typeof defaultUIState)["acknowledgedAgentErrors"];
  dismissedAgentErrors: (typeof defaultUIState)["dismissedAgentErrors"];

  // Actions from all slices
  hydrate: (state: HydrationState, options?: HydrationOptions) => void;
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState["items"]) => void;
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
  setInstallJobs: (jobs: InstallJob[]) => void;
  upsertInstallJob: (job: InstallJob) => void;
  appendInstallOutput: (agentName: string, chunk: string) => void;
  clearInstallJob: (agentName: string) => void;
  setAgentUpdateJobs: (jobs: AgentUpdateJob[]) => void;
  upsertAgentUpdateJob: (job: AgentUpdateJob) => void;
  appendAgentUpdateOutput: (agentName: string, jobId: string, chunk: string) => void;
  clearAgentUpdateJob: (agentName: string) => void;
  setRepositories: (workspaceId: string, repositories: Repository[]) => void;
  upsertRepository: (workspaceId: string, repository: Repository) => void;
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
  setSessionMCPStatus: (
    sessionId: string,
    history: import("./slices/session-runtime/types").MCPAttachmentHistory,
  ) => void;
  setPreviewOpen: (sessionId: string, open: boolean) => void;
  togglePreviewOpen: (sessionId: string) => void;
  setPreviewView: (sessionId: string, view: PreviewViewMode) => void;
  setPreviewDevice: (sessionId: string, device: PreviewDevicePreset) => void;
  setPreviewStage: (sessionId: string, stage: PreviewStage) => void;
  setPreviewUrl: (sessionId: string, url: string) => void;
  setPreviewUrlDraft: (sessionId: string, url: string) => void;
  setRightPanelActiveTab: (sessionId: string, tab: string) => void;
  setConnectionStatus: (status: ConnectionState["status"], error?: string | null) => void;
  setConnectionIssueSeverity: (
    severity: import("@/lib/types/connection").ConnectionIssueSeverity,
  ) => void;
  setMobileKanbanActiveStep: (workflowId: string, stepId: string) => void;
  setMobileKanbanMenuOpen: (open: boolean) => void;
  setMobileKanbanSearchOpen: (open: boolean) => void;
  setMobileSessionPanel: (sessionId: string, panel: UISliceTypes.MobileSessionPanel) => void;
  setMobileSessionReview: (sessionId: string, mrKey: string | null) => void;
  setMobileSessionTaskSwitcherOpen: (open: boolean) => void;
  setPlanMode: (sessionId: string, enabled: boolean) => void;
  setCancelTurnPending: UIA["setCancelTurnPending"];
  setTranscriptAutoScrollEnabled: UIA["setTranscriptAutoScrollEnabled"];
  setTranscriptScrollTop: UIA["setTranscriptScrollTop"];
  setTranscriptVirtuosoState: UIA["setTranscriptVirtuosoState"];
  setReviewPRSelection: UIA["setReviewPRSelection"];
  setActiveDocument: (sessionId: string, doc: UISliceTypes.ActiveDocument | null) => void;
  setSystemHealth: (response: SystemHealthResponse) => void;
  setSystemHealthLoading: (loading: boolean) => void;
  invalidateSystemHealth: () => void;
  openQuickChat: UIA["openQuickChat"];
  addQuickChatSession: UIA["addQuickChatSession"];
  syncQuickChatSessions: UIA["syncQuickChatSessions"];
  upsertQuickChatSessionFromEvent: UIA["upsertQuickChatSessionFromEvent"];
  removeQuickChatSessionsForTask: UIA["removeQuickChatSessionsForTask"];
  closeQuickChat: () => void;
  closeQuickChatSession: (sessionId: string) => void;
  setActiveQuickChatSession: (sessionId: string, workspaceId: string) => void;
  renameQuickChatSession: (sessionId: string, name: string) => void;
  setQuickChatInitialPrompt: UIA["setQuickChatInitialPrompt"];
  setSessionFailureNotification: (n: UISliceTypes.SessionFailureNotification | null) => void;
  setTaskDeletedNotification: (n: UISliceTypes.TaskDeletedNotification | null) => void;
  setUpdateAvailableNotification: (n: UISliceTypes.UpdateAvailableNotification | null) => void;
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
  setTaskSession: (session: TaskSession) => void;
  updateSessionReadCursor: (sessionId: string, lastReadMessageId: string) => void;
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
      configBaseline?: Record<string, string>;
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
  setEmbeddedVscodeSupport: (sessionId: string, supported: boolean) => void;
  /* prettier-ignore */ setSidebarActiveView: UIA["setSidebarActiveView"];
  createSidebarView: UIA["createSidebarView"];
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
} & GitHubSliceActions &
  GitLabSliceActions &
  JiraSliceActions &
  LinearSliceActions &
  OfficeSliceActions &
  import("./store-reexports").WorkspaceSourceStoreState &
  AzureDevOpsSliceActions &
  SystemSliceActions &
  FeaturesSliceActions &
  AuthSliceActions &
  AutomationsSliceActions &
  PluginsSliceActions &
  ReviewSliceActions;

// Most callers hydrate a fully-shaped slice per top-level key (see
// mergeInitialState / hydrateState), but `system` is a grab-bag of many
// independently-fetched fields (info, diskUsage, updates, ...). Callers that
// only have one piece of it (e.g. update notification settings from the
// settings boot payload) must be able to pass a partial `system` object
// without fabricating placeholder values for the rest.
export type HydrationState = Omit<Partial<AppState>, "system"> & {
  system?: Partial<AppState["system"]>;
};

/** Creates the Zustand app store, hydrating from `initialState` and
 * composing every domain slice (kanban, ui, workspace, settings, ...). */
export function createAppStore(initialState?: HydrationState) {
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
      ...createAzureDevOpsSlice(set as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createJiraSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createLinearSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createOfficeSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createFeaturesSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createAuthSlice(set as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSystemSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createUISlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createAutomationsSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createPluginsSlice(set as any, get as any, api as any),
      // createReviewSlice only needs `set`; passing get/api would be superfluous
      // arguments (CodeQL js/superfluous-trailing-arguments).
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createReviewSlice(set as any),
      // Re-assert merged initial state so caller-supplied values win over slice defaults.
      ...buildStateOverrides(merged),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      hydrate: (state, options) => set((draft) => hydrateState(draft as any, state, options)),
    })),
  );
}

export type StoreProviderProps = {
  children: React.ReactNode;
  initialState?: HydrationState;
};
