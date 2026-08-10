import {
  defaultKanbanState,
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
  defaultReviewState,
} from "./slices";
import { applyStoredQuickChatNames } from "@/lib/state/slices/ui/quick-chat-sync";
import type { AgentRuntimeAvailability } from "@/lib/types/agent-runtime";
import type { HydrationState } from "./store";
import { migrateSidebarViewDraft, migrateView } from "./slices/ui/ui-slice";

export const defaultState = {
  kanban: defaultKanbanState.kanban,
  kanbanMulti: defaultKanbanState.kanbanMulti,
  sidebarArchivedTasks: defaultKanbanState.sidebarArchivedTasks,
  workflows: defaultKanbanState.workflows,
  tasks: defaultKanbanState.tasks,
  workspaces: defaultWorkspaceState.workspaces,
  repositories: defaultWorkspaceState.repositories,
  repositoryBranches: defaultWorkspaceState.repositoryBranches,
  repositoryScripts: defaultWorkspaceState.repositoryScripts,
  executors: defaultSettingsState.executors,
  settingsAgents: defaultSettingsState.settingsAgents,
  agentDiscovery: defaultSettingsState.agentDiscovery,
  availableAgents: defaultSettingsState.availableAgents,
  agentProfiles: defaultSettingsState.agentProfiles,
  editors: defaultSettingsState.editors,
  prompts: defaultSettingsState.prompts,
  secrets: defaultSettingsState.secrets,
  notificationProviders: defaultSettingsState.notificationProviders,
  settingsData: defaultSettingsState.settingsData,
  sleepInhibition: defaultSettingsState.sleepInhibition,
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
  walkthroughs: defaultSessionState.walkthroughs,
  taskReview: defaultReviewState.taskReview,
  queue: defaultSessionState.queue,
  terminal: defaultSessionRuntimeState.terminal,
  shell: defaultSessionRuntimeState.shell,
  processes: defaultSessionRuntimeState.processes,
  gitStatus: defaultSessionRuntimeState.gitStatus,
  environmentIdBySessionId: defaultSessionRuntimeState.environmentIdBySessionId,
  sessionCommits: defaultSessionRuntimeState.sessionCommits,
  contextWindow: defaultSessionRuntimeState.contextWindow,
  agents: defaultSessionRuntimeState.agents,
  availableCommands: defaultSessionRuntimeState.availableCommands,
  sessionMode: defaultSessionRuntimeState.sessionMode,
  userShells: defaultSessionRuntimeState.userShells,
  prepareProgress: defaultSessionRuntimeState.prepareProgress,
  sessionTodos: defaultSessionRuntimeState.sessionTodos,
  agentCapabilities: defaultSessionRuntimeState.agentCapabilities,
  sessionModels: defaultSessionRuntimeState.sessionModels,
  sessionMcpStatus: defaultSessionRuntimeState.sessionMcpStatus,
  promptUsage: defaultSessionRuntimeState.promptUsage,
  sessionPollMode: defaultSessionRuntimeState.sessionPollMode,
  embeddedVscodeSupport: defaultSessionRuntimeState.embeddedVscodeSupport,
  githubStatus: defaultGitHubState.githubStatus,
  githubAppRegistrations: defaultGitHubState.githubAppRegistrations,
  taskPRs: defaultGitHubState.taskPRs,
  taskIssues: defaultGitHubState.taskIssues,
  pendingPrUrlByTaskId: defaultGitHubState.pendingPrUrlByTaskId,
  prWatches: defaultGitHubState.prWatches,
  reviewWatches: defaultGitHubState.reviewWatches,
  issueWatches: defaultGitHubState.issueWatches,
  actionPresets: defaultGitHubState.actionPresets,
  prFeedbackCache: defaultGitHubState.prFeedbackCache,
  taskCIAutomation: defaultGitHubState.taskCIAutomation,
  taskMRs: defaultGitLabState.taskMRs,
  azureDevOpsTaskPullRequests: defaultAzureDevOpsState.azureDevOpsTaskPullRequests,
  azureDevOpsTaskWorkItems: defaultAzureDevOpsState.azureDevOpsTaskWorkItems,
  gitlabReviewWatches: defaultGitLabState.gitlabReviewWatches,
  gitlabIssueWatches: defaultGitLabState.gitlabIssueWatches,
  gitlabMRWatches: defaultGitLabState.gitlabMRWatches,
  gitlabActionPresets: defaultGitLabState.gitlabActionPresets,
  gitlabStats: defaultGitLabState.gitlabStats,
  gitlabStatus: defaultGitLabState.gitlabStatus,
  taskMRAutomation: defaultGitLabState.taskMRAutomation,
  jiraIssueWatches: defaultJiraState.jiraIssueWatches,
  linearIssueWatches: defaultLinearState.linearIssueWatches,
  office: defaultOfficeState.office,
  features: defaultFeaturesState.features,
  auth: defaultAuthState.auth,
  automations: defaultAutomationsState.automations,
  automationRuns: defaultAutomationsState.automationRuns,
  system: defaultSystemState.system,
  agentRuntime: null as AgentRuntimeAvailability | null,
  previewPanel: defaultUIState.previewPanel,
  rightPanel: defaultUIState.rightPanel,
  diffs: defaultUIState.diffs,
  connection: defaultUIState.connection,
  mobileKanban: defaultUIState.mobileKanban,
  mobileSession: defaultUIState.mobileSession,
  chatInput: defaultUIState.chatInput,
  reviewPRSelection: defaultUIState.reviewPRSelection,
  documentPanel: defaultUIState.documentPanel,
  systemHealth: defaultUIState.systemHealth,
  quickChat: defaultUIState.quickChat,
  sessionFailureNotification: defaultUIState.sessionFailureNotification,
  bottomTerminal: defaultUIState.bottomTerminal,
  sidebarViews: defaultUIState.sidebarViews,
  collapsedSubtaskParents: defaultUIState.collapsedSubtaskParents,
  kanbanPreviewedTaskId: defaultUIState.kanbanPreviewedTaskId,
  sidebarTaskPrefs: defaultUIState.sidebarTaskPrefs,
};

export type DefaultState = typeof defaultState;

function mergeCodeHostFields(
  d: DefaultState,
  s: HydrationState,
): Pick<
  DefaultState,
  | "taskMRs"
  | "gitlabReviewWatches"
  | "gitlabIssueWatches"
  | "gitlabMRWatches"
  | "gitlabActionPresets"
  | "gitlabStats"
  | "gitlabStatus"
  | "azureDevOpsTaskPullRequests"
  | "azureDevOpsTaskWorkItems"
> {
  return {
    taskMRs: { ...d.taskMRs, ...s.taskMRs },
    gitlabReviewWatches: { ...d.gitlabReviewWatches, ...s.gitlabReviewWatches },
    gitlabIssueWatches: { ...d.gitlabIssueWatches, ...s.gitlabIssueWatches },
    gitlabMRWatches: { ...d.gitlabMRWatches, ...s.gitlabMRWatches },
    gitlabActionPresets: { ...d.gitlabActionPresets, ...s.gitlabActionPresets },
    gitlabStats: { ...d.gitlabStats, ...s.gitlabStats },
    gitlabStatus: { ...d.gitlabStatus, ...s.gitlabStatus },
    azureDevOpsTaskPullRequests: {
      ...d.azureDevOpsTaskPullRequests,
      ...s.azureDevOpsTaskPullRequests,
    },
    azureDevOpsTaskWorkItems: {
      ...d.azureDevOpsTaskWorkItems,
      ...s.azureDevOpsTaskWorkItems,
    },
  };
}

function mergeQuickChatState(initialState: HydrationState): DefaultState["quickChat"] {
  const quickChat = { ...defaultState.quickChat, ...initialState.quickChat };
  if (!initialState.quickChat?.sessions) return quickChat;

  return {
    ...quickChat,
    sessions: applyStoredQuickChatNames(initialState.quickChat.sessions),
  };
}

function mergeSidebarViewState(initialState: HydrationState): DefaultState["sidebarViews"] {
  const sidebarViews = { ...defaultState.sidebarViews, ...initialState.sidebarViews };
  const userSettings = initialState.userSettings;
  const serverViews = userSettings?.sidebarViews?.map(migrateView) ?? [];
  if (serverViews.length > 0) sidebarViews.views = serverViews;

  const activeViewId = userSettings?.sidebarActiveViewId;
  if (activeViewId && sidebarViews.views.some((view) => view.id === activeViewId)) {
    sidebarViews.activeViewId = activeViewId;
  } else if (!sidebarViews.views.some((view) => view.id === sidebarViews.activeViewId)) {
    sidebarViews.activeViewId = sidebarViews.views[0].id;
  }
  if (userSettings?.sidebarDraft !== undefined) {
    sidebarViews.draft = userSettings.sidebarDraft
      ? migrateSidebarViewDraft(userSettings.sidebarDraft)
      : null;
  }
  return sidebarViews;
}

function mergeSidebarTaskPrefsState(
  initialState: HydrationState,
): DefaultState["sidebarTaskPrefs"] {
  const sidebarTaskPrefs = {
    ...defaultState.sidebarTaskPrefs,
    ...initialState.sidebarTaskPrefs,
  };
  const serverPrefs = initialState.userSettings?.sidebarTaskPrefs;
  if (!serverPrefs) return sidebarTaskPrefs;

  return {
    ...sidebarTaskPrefs,
    pinnedTaskIds: [...serverPrefs.pinnedTaskIds],
    orderedTaskIds: [...serverPrefs.orderedTaskIds],
    subtaskOrderByParentId: Object.fromEntries(
      Object.entries(serverPrefs.subtaskOrderByParentId).map(([parentId, taskIds]) => [
        parentId,
        [...taskIds],
      ]),
    ),
  };
}

function mergeReviewPRSelectionState(
  initialState: HydrationState,
): DefaultState["reviewPRSelection"] {
  return {
    ...defaultState.reviewPRSelection,
    ...initialState.reviewPRSelection,
    selectedKeyByTaskId: {
      ...defaultState.reviewPRSelection.selectedKeyByTaskId,
      ...initialState.reviewPRSelection?.selectedKeyByTaskId,
    },
  };
}

function mergeSessionFailureNotification(initialState: HydrationState) {
  return initialState.sessionFailureNotification ?? defaultState.sessionFailureNotification;
}

/**
 * Merges the agent-authored review artifacts (walkthroughs and code-review
 * findings). Extracted so mergeInitialState stays under the function line cap.
 */
function mergeAgentReviewArtifacts(initialState: HydrationState) {
  return {
    walkthroughs: { ...defaultState.walkthroughs, ...initialState.walkthroughs },
    taskReview: { ...defaultState.taskReview, ...initialState.taskReview },
  };
}

function mergeGitHubState(initialState: HydrationState) {
  return {
    githubStatus: { ...defaultState.githubStatus, ...initialState.githubStatus },
    githubAppRegistrations: {
      ...defaultState.githubAppRegistrations,
      ...initialState.githubAppRegistrations,
    },
  };
}

export function mergeInitialState(initialState?: HydrationState): DefaultState {
  if (!initialState) return defaultState;
  return {
    ...defaultState,
    ...initialState,
    kanban: { ...defaultState.kanban, ...initialState.kanban },
    kanbanMulti: { ...defaultState.kanbanMulti, ...initialState.kanbanMulti },
    workflows: { ...defaultState.workflows, ...initialState.workflows },
    tasks: { ...defaultState.tasks, ...initialState.tasks },
    workspaces: { ...defaultState.workspaces, ...initialState.workspaces },
    repositories: { ...defaultState.repositories, ...initialState.repositories },
    repositoryBranches: { ...defaultState.repositoryBranches, ...initialState.repositoryBranches },
    repositoryScripts: { ...defaultState.repositoryScripts, ...initialState.repositoryScripts },
    executors: { ...defaultState.executors, ...initialState.executors },
    settingsAgents: { ...defaultState.settingsAgents, ...initialState.settingsAgents },
    agentDiscovery: { ...defaultState.agentDiscovery, ...initialState.agentDiscovery },
    availableAgents: { ...defaultState.availableAgents, ...initialState.availableAgents },
    agentProfiles: { ...defaultState.agentProfiles, ...initialState.agentProfiles },
    editors: { ...defaultState.editors, ...initialState.editors },
    prompts: { ...defaultState.prompts, ...initialState.prompts },
    secrets: { ...defaultState.secrets, ...initialState.secrets },
    notificationProviders: {
      ...defaultState.notificationProviders,
      ...initialState.notificationProviders,
    },
    settingsData: { ...defaultState.settingsData, ...initialState.settingsData },
    sleepInhibition: { ...defaultState.sleepInhibition, ...initialState.sleepInhibition },
    userSettings: { ...defaultState.userSettings, ...initialState.userSettings },
    messages: { ...defaultState.messages, ...initialState.messages },
    turns: { ...defaultState.turns, ...initialState.turns },
    taskSessions: { ...defaultState.taskSessions, ...initialState.taskSessions },
    taskSessionsByTask: { ...defaultState.taskSessionsByTask, ...initialState.taskSessionsByTask },
    sessionAgentctl: { ...defaultState.sessionAgentctl, ...initialState.sessionAgentctl },
    worktrees: { ...defaultState.worktrees, ...initialState.worktrees },
    sessionWorktreesBySessionId: {
      ...defaultState.sessionWorktreesBySessionId,
      ...initialState.sessionWorktreesBySessionId,
    },
    pendingModel: { ...defaultState.pendingModel, ...initialState.pendingModel },
    activeModel: { ...defaultState.activeModel, ...initialState.activeModel },
    taskPlans: { ...defaultState.taskPlans, ...initialState.taskPlans },
    ...mergeAgentReviewArtifacts(initialState),
    queue: { ...defaultState.queue, ...initialState.queue },
    terminal: { ...defaultState.terminal, ...initialState.terminal },
    shell: { ...defaultState.shell, ...initialState.shell },
    processes: { ...defaultState.processes, ...initialState.processes },
    gitStatus: { ...defaultState.gitStatus, ...initialState.gitStatus },
    sessionCommits: { ...defaultState.sessionCommits, ...initialState.sessionCommits },
    contextWindow: { ...defaultState.contextWindow, ...initialState.contextWindow },
    agents: { ...defaultState.agents, ...initialState.agents },
    sessionMode: { ...defaultState.sessionMode, ...initialState.sessionMode },
    userShells: { ...defaultState.userShells, ...initialState.userShells },
    prepareProgress: { ...defaultState.prepareProgress, ...initialState.prepareProgress },
    sessionTodos: { ...defaultState.sessionTodos, ...initialState.sessionTodos },
    agentCapabilities: { ...defaultState.agentCapabilities, ...initialState.agentCapabilities },
    sessionModels: { ...defaultState.sessionModels, ...initialState.sessionModels },
    sessionMcpStatus: { ...defaultState.sessionMcpStatus, ...initialState.sessionMcpStatus },
    promptUsage: { ...defaultState.promptUsage, ...initialState.promptUsage },
    sessionPollMode: { ...defaultState.sessionPollMode, ...initialState.sessionPollMode },
    embeddedVscodeSupport: {
      ...defaultState.embeddedVscodeSupport,
      ...initialState.embeddedVscodeSupport,
    },
    ...mergeGitHubState(initialState),
    taskPRs: { ...defaultState.taskPRs, ...initialState.taskPRs },
    taskIssues: { ...defaultState.taskIssues, ...initialState.taskIssues },
    pendingPrUrlByTaskId: {
      ...defaultState.pendingPrUrlByTaskId,
      ...initialState.pendingPrUrlByTaskId,
    },
    prWatches: { ...defaultState.prWatches, ...initialState.prWatches },
    reviewWatches: { ...defaultState.reviewWatches, ...initialState.reviewWatches },
    issueWatches: { ...defaultState.issueWatches, ...initialState.issueWatches },
    actionPresets: { ...defaultState.actionPresets, ...initialState.actionPresets },
    prFeedbackCache: { ...defaultState.prFeedbackCache, ...initialState.prFeedbackCache },
    taskCIAutomation: { ...defaultState.taskCIAutomation, ...initialState.taskCIAutomation },
    taskMRAutomation: { ...defaultState.taskMRAutomation, ...initialState.taskMRAutomation },
    ...mergeCodeHostFields(defaultState, initialState),
    jiraIssueWatches: { ...defaultState.jiraIssueWatches, ...initialState.jiraIssueWatches },
    linearIssueWatches: {
      ...defaultState.linearIssueWatches,
      ...initialState.linearIssueWatches,
    },
    office: { ...defaultState.office, ...initialState.office },
    features: { ...defaultState.features, ...initialState.features },
    auth: { ...defaultState.auth, ...initialState.auth },
    automations: { ...defaultState.automations, ...initialState.automations },
    automationRuns: { ...defaultState.automationRuns, ...initialState.automationRuns },
    system: { ...defaultState.system, ...initialState.system },
    ...mergeUIPanelState(initialState),
  };
}

// Split out of mergeInitialState to stay under the per-function line limit —
// these fields are the UI/panel slice of the merge and have no cross-field
// dependencies on the rest of DefaultState.
function mergeUIPanelState(initialState: HydrationState) {
  return {
    previewPanel: { ...defaultState.previewPanel, ...initialState.previewPanel },
    rightPanel: { ...defaultState.rightPanel, ...initialState.rightPanel },
    diffs: { ...defaultState.diffs, ...initialState.diffs },
    connection: { ...defaultState.connection, ...initialState.connection },
    mobileKanban: { ...defaultState.mobileKanban, ...initialState.mobileKanban },
    mobileSession: { ...defaultState.mobileSession, ...initialState.mobileSession },
    chatInput: { ...defaultState.chatInput, ...initialState.chatInput },
    reviewPRSelection: mergeReviewPRSelectionState(initialState),
    documentPanel: { ...defaultState.documentPanel, ...initialState.documentPanel },
    systemHealth: { ...defaultState.systemHealth, ...initialState.systemHealth },
    quickChat: mergeQuickChatState(initialState),
    sessionFailureNotification: mergeSessionFailureNotification(initialState),
    bottomTerminal: { ...defaultState.bottomTerminal, ...initialState.bottomTerminal },
    sidebarViews: mergeSidebarViewState(initialState),
    sidebarTaskPrefs: mergeSidebarTaskPrefsState(initialState),
    collapsedSubtaskParents:
      initialState.collapsedSubtaskParents ?? defaultState.collapsedSubtaskParents,
  };
}
