import {
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
} from "./slices";
import { getStoredQuickChatNames } from "@/lib/local-storage";
import { migrateView } from "./slices/ui/ui-slice";

export const defaultState = {
  kanban: defaultKanbanState.kanban,
  kanbanMulti: defaultKanbanState.kanbanMulti,
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
  promptUsage: defaultSessionRuntimeState.promptUsage,
  sessionPollMode: defaultSessionRuntimeState.sessionPollMode,
  githubStatus: defaultGitHubState.githubStatus,
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
  gitlabReviewWatches: defaultGitLabState.gitlabReviewWatches,
  gitlabIssueWatches: defaultGitLabState.gitlabIssueWatches,
  gitlabMRWatches: defaultGitLabState.gitlabMRWatches,
  gitlabActionPresets: defaultGitLabState.gitlabActionPresets,
  gitlabStats: defaultGitLabState.gitlabStats,
  gitlabStatus: defaultGitLabState.gitlabStatus,
  jiraIssueWatches: defaultJiraState.jiraIssueWatches,
  linearIssueWatches: defaultLinearState.linearIssueWatches,
  office: defaultOfficeState.office,
  features: defaultFeaturesState.features,
  automations: defaultAutomationsState.automations,
  automationRuns: defaultAutomationsState.automationRuns,
  system: defaultSystemState.system,
  previewPanel: defaultUIState.previewPanel,
  rightPanel: defaultUIState.rightPanel,
  diffs: defaultUIState.diffs,
  connection: defaultUIState.connection,
  mobileKanban: defaultUIState.mobileKanban,
  mobileSession: defaultUIState.mobileSession,
  chatInput: defaultUIState.chatInput,
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

function mergeGitLabFields(
  d: DefaultState,
  s: Partial<DefaultState>,
): Pick<
  DefaultState,
  | "taskMRs"
  | "gitlabReviewWatches"
  | "gitlabIssueWatches"
  | "gitlabMRWatches"
  | "gitlabActionPresets"
  | "gitlabStats"
  | "gitlabStatus"
> {
  return {
    taskMRs: { ...d.taskMRs, ...s.taskMRs },
    gitlabReviewWatches: { ...d.gitlabReviewWatches, ...s.gitlabReviewWatches },
    gitlabIssueWatches: { ...d.gitlabIssueWatches, ...s.gitlabIssueWatches },
    gitlabMRWatches: { ...d.gitlabMRWatches, ...s.gitlabMRWatches },
    gitlabActionPresets: { ...d.gitlabActionPresets, ...s.gitlabActionPresets },
    gitlabStats: { ...d.gitlabStats, ...s.gitlabStats },
    gitlabStatus: { ...d.gitlabStatus, ...s.gitlabStatus },
  };
}

function mergeQuickChatState(initialState: Partial<DefaultState>): DefaultState["quickChat"] {
  const quickChat = { ...defaultState.quickChat, ...initialState.quickChat };
  if (!initialState.quickChat?.sessions) return quickChat;

  const storedNames = getStoredQuickChatNames();
  return {
    ...quickChat,
    sessions: initialState.quickChat.sessions.map((session) => {
      const localName = storedNames[session.sessionId];
      return {
        ...session,
        kind: session.kind ?? "chat",
        ...(localName ? { name: localName } : {}),
      };
    }),
  };
}

function mergeSidebarViewState(initialState: Partial<DefaultState>): DefaultState["sidebarViews"] {
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
  if (userSettings?.sidebarDraft !== undefined) sidebarViews.draft = userSettings.sidebarDraft;
  return sidebarViews;
}

function mergeSidebarTaskPrefsState(
  initialState: Partial<DefaultState>,
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

export function mergeInitialState(initialState?: Partial<DefaultState>): DefaultState {
  if (!initialState) return defaultState;

  return {
    ...defaultState,
    ...initialState,
    // Ensure nested objects are properly merged
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
    walkthroughs: { ...defaultState.walkthroughs, ...initialState.walkthroughs },
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
    promptUsage: { ...defaultState.promptUsage, ...initialState.promptUsage },
    sessionPollMode: { ...defaultState.sessionPollMode, ...initialState.sessionPollMode },
    githubStatus: { ...defaultState.githubStatus, ...initialState.githubStatus },
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
    ...mergeGitLabFields(defaultState, initialState),
    jiraIssueWatches: { ...defaultState.jiraIssueWatches, ...initialState.jiraIssueWatches },
    linearIssueWatches: {
      ...defaultState.linearIssueWatches,
      ...initialState.linearIssueWatches,
    },
    office: { ...defaultState.office, ...initialState.office },
    features: { ...defaultState.features, ...initialState.features },
    automations: { ...defaultState.automations, ...initialState.automations },
    automationRuns: { ...defaultState.automationRuns, ...initialState.automationRuns },
    system: { ...defaultState.system, ...initialState.system },
    previewPanel: { ...defaultState.previewPanel, ...initialState.previewPanel },
    rightPanel: { ...defaultState.rightPanel, ...initialState.rightPanel },
    diffs: { ...defaultState.diffs, ...initialState.diffs },
    connection: { ...defaultState.connection, ...initialState.connection },
    mobileKanban: { ...defaultState.mobileKanban, ...initialState.mobileKanban },
    mobileSession: { ...defaultState.mobileSession, ...initialState.mobileSession },
    chatInput: { ...defaultState.chatInput, ...initialState.chatInput },
    documentPanel: { ...defaultState.documentPanel, ...initialState.documentPanel },
    systemHealth: { ...defaultState.systemHealth, ...initialState.systemHealth },
    quickChat: mergeQuickChatState(initialState),
    sessionFailureNotification:
      initialState.sessionFailureNotification ?? defaultState.sessionFailureNotification,
    bottomTerminal: { ...defaultState.bottomTerminal, ...initialState.bottomTerminal },
    sidebarViews: mergeSidebarViewState(initialState),
    sidebarTaskPrefs: mergeSidebarTaskPrefsState(initialState),
    collapsedSubtaskParents:
      initialState.collapsedSubtaskParents ?? defaultState.collapsedSubtaskParents,
  };
}
