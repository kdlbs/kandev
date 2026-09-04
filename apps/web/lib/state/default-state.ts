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
import { mergeHydratedQuickChatSessions } from "@/lib/state/slices/ui/quick-chat-sync";
import type { AgentRuntimeAvailability } from "@/lib/types/agent-runtime";
import type { HydrationState } from "./store";
import { seedSettledSessionBoundaries } from "@/lib/state/slices/session/turn-actions";
import { migrateSidebarViewDraft, migrateView } from "./slices/ui/ui-slice";
import { mergeAgentProfileRecentUseState } from "@/lib/agent-profile-recent-use";
import { normalizeThreadViews } from "./slices/ui/thread-view-builtins";

export const defaultState = {
  kanban: defaultKanbanState.kanban,
  kanbanMulti: defaultKanbanState.kanbanMulti,
  sidebarArchivedTasks: defaultKanbanState.sidebarArchivedTasks,
  workflows: defaultKanbanState.workflows,
  workspaceContextGeneration: defaultKanbanState.workspaceContextGeneration,
  tasks: defaultKanbanState.tasks,
  workspaces: defaultWorkspaceState.workspaces,
  repositories: defaultWorkspaceState.repositories,
  repositorySets: defaultWorkspaceState.repositorySets,
  repositoryBranchPolicies: defaultWorkspaceState.repositoryBranchPolicies,
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
  agentProfileRecentUse: defaultSettingsState.agentProfileRecentUse,
  messages: defaultSessionState.messages,
  turns: defaultSessionState.turns,
  taskSessions: defaultSessionState.taskSessions,
  taskSessionsByTask: defaultSessionState.taskSessionsByTask,
  pendingActionProjectionsBySessionId: defaultSessionState.pendingActionProjectionsBySessionId,
  sessionAgentctl: defaultSessionState.sessionAgentctl,
  worktrees: defaultSessionState.worktrees,
  sessionWorktreesBySessionId: defaultSessionState.sessionWorktreesBySessionId,
  pendingModel: defaultSessionState.pendingModel,
  activeModel: defaultSessionState.activeModel,
  messagePrompts: defaultSessionState.messagePrompts,
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
  threadViews: defaultUIState.threadViews,
  collapsedSubtaskParents: defaultUIState.collapsedSubtaskParents,
  kanbanPreviewedTaskId: defaultUIState.kanbanPreviewedTaskId,
  sidebarTaskPrefs: defaultUIState.sidebarTaskPrefs,
};

export type DefaultState = typeof defaultState;

/** Merge the code-host slice fields (MRs, watches, presets, stats, status, Azure DevOps PRs/work items) from hydration state over defaults. */
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

/** Merge quick-chat state from hydration over defaults, applying locally stored chat names to the SSR-provided sessions. */
function mergeQuickChatState(initialState: HydrationState): DefaultState["quickChat"] {
  const { sessions, ...hydratedQuickChat } = initialState.quickChat ?? {};
  const quickChat = {
    ...defaultState.quickChat,
    ...hydratedQuickChat,
    unseenIdleByWorkspace: {},
    lastSettledAtBySession: {},
    sessionOwnership: {},
    syncRevisionByWorkspace: {},
    tombstonedSessions: {},
  };
  return sessions ? mergeHydratedQuickChatSessions(quickChat, sessions) : quickChat;
}

/** Merge sidebar view state, preferring the server-provided views, active view, and draft from user settings when present. */
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

/** Merge Threads saved views from user settings over the UI defaults. */
function mergeThreadViewState(initialState: HydrationState): DefaultState["threadViews"] {
  const threadViews = { ...defaultState.threadViews, ...initialState.threadViews };
  const serverViews = initialState.userSettings?.threadViews;
  if (serverViews) threadViews.views = normalizeThreadViews(serverViews);

  const activeViewId = initialState.userSettings?.threadActiveViewId;
  if (activeViewId && threadViews.views.some((view) => view.id === activeViewId)) {
    threadViews.activeViewId = activeViewId;
  } else if (!threadViews.views.some((view) => view.id === threadViews.activeViewId)) {
    threadViews.activeViewId = threadViews.views[0].id;
  }
  if (initialState.userSettings?.threadViewDraft !== undefined) {
    threadViews.draft = initialState.userSettings.threadViewDraft;
  }
  return threadViews;
}

/** Merge sidebar task prefs, copying the server-provided pinned, ordered, and subtask-order lists from user settings over defaults. */
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

/** Merge review-PR selection state, deep-merging the per-task selected keys from hydration over defaults. */
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

/** Return the hydrated session-failure notification, or the default when none is provided. */
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

/** Merges the independently hydrated Prompt History projection. */
function mergePromptHistoryState(initialState: HydrationState) {
  return {
    ...defaultState.messagePrompts,
    ...initialState.messagePrompts,
    generationBySession: {
      ...defaultState.messagePrompts.generationBySession,
      ...initialState.messagePrompts?.generationBySession,
    },
  };
}

/** Merges the GitHub slices for initial (SSR/boot) hydration. */
/** Merges the GitHub slices for initial (SSR/boot) hydration. */
function mergeGitHubState(initialState: HydrationState) {
  return {
    githubStatus: { ...defaultState.githubStatus, ...initialState.githubStatus },
    githubAppRegistrations: {
      ...defaultState.githubAppRegistrations,
      ...initialState.githubAppRegistrations,
    },
    taskPRs: mergeTaskPRState(initialState),
  };
}

function mergeTaskPRState(initialState: HydrationState) {
  const incoming = initialState.taskPRs;
  return {
    ...defaultState.taskPRs,
    ...incoming,
    byTaskId: { ...defaultState.taskPRs.byTaskId, ...incoming?.byTaskId },
    deletedAssociationIdsByTaskId: {
      ...defaultState.taskPRs.deletedAssociationIdsByTaskId,
      ...incoming?.deletedAssociationIdsByTaskId,
    },
    workspaceId: incoming?.workspaceId ?? initialState.workspaces?.activeId ?? null,
    workspaceContextGeneration:
      incoming?.workspaceContextGeneration ?? initialState.workspaceContextGeneration ?? 0,
  };
}

/**
 * Merges the turns slice for initial (SSR/boot) hydration. The server-side
 * turn lists are complete per-session snapshots, so every session the payload
 * installs is marked `loadedBySession`; without the marker, turn-derived UI
 * would mistake WS-seeded live turns for the full history (the debug
 * metadata dialog's `turn_metadata` regression). Settled sessions also seed
 * their settled boundary from `updated_at` (monotonically) so an
 * old/unknown delayed WS start arriving before any session-list refresh is
 * rejected.
 */
function mergeTurnsState(
  current: DefaultState["turns"],
  incoming: HydrationState["turns"],
  sessions: HydrationState["taskSessions"],
): DefaultState["turns"] {
  const merged = { ...current, ...incoming };
  const settledBoundaryBySession = { ...merged.settledBoundaryBySession };
  // One shared seeding invariant (turn-actions) with the production
  // StateHydrator path, so the SSR and hydration boundary rules cannot drift.
  seedSettledSessionBoundaries(settledBoundaryBySession, Object.values(sessions?.items ?? {}));
  if (!incoming?.bySession) return { ...merged, settledBoundaryBySession };
  const loadedBySession: Record<string, boolean> = {};
  for (const sessionId of Object.keys(incoming.bySession)) {
    loadedBySession[sessionId] = true;
  }
  return { ...merged, loadedBySession, settledBoundaryBySession };
}

function mergeTaskSessionState(initialState: HydrationState) {
  return {
    taskSessions: { ...defaultState.taskSessions, ...initialState.taskSessions },
    taskSessionsByTask: {
      ...defaultState.taskSessionsByTask,
      ...initialState.taskSessionsByTask,
      itemsByTaskId: {
        ...defaultState.taskSessionsByTask.itemsByTaskId,
        ...initialState.taskSessionsByTask?.itemsByTaskId,
      },
      loadingByTaskId: {
        ...defaultState.taskSessionsByTask.loadingByTaskId,
        ...initialState.taskSessionsByTask?.loadingByTaskId,
      },
      loadedByTaskId: {
        ...defaultState.taskSessionsByTask.loadedByTaskId,
        ...initialState.taskSessionsByTask?.loadedByTaskId,
      },
      errorByTaskId: {
        ...defaultState.taskSessionsByTask.errorByTaskId,
        ...initialState.taskSessionsByTask?.errorByTaskId,
      },
    },
    pendingActionProjectionsBySessionId: {
      ...defaultState.pendingActionProjectionsBySessionId,
      ...initialState.pendingActionProjectionsBySessionId,
    },
  };
}

/**
 * Builds the full default state from the SSR/boot hydration payload, merging
 * per-slice (kanban, turns, settings, ...) so partial payloads never clobber
 * the client's live defaults.
 */
// eslint-disable-next-line max-lines-per-function -- merges every hydrated state slice in one place.
export function mergeInitialState(initialState?: HydrationState): DefaultState {
  if (!initialState) return defaultState;
  return {
    ...defaultState,
    ...initialState,
    kanban: { ...defaultState.kanban, ...initialState.kanban },
    kanbanMulti: { ...defaultState.kanbanMulti, ...initialState.kanbanMulti },
    workflows: { ...defaultState.workflows, ...initialState.workflows },
    workspaceContextGeneration: mergeWorkspaceContextGeneration(initialState),
    tasks: { ...defaultState.tasks, ...initialState.tasks },
    workspaces: { ...defaultState.workspaces, ...initialState.workspaces },
    repositories: { ...defaultState.repositories, ...initialState.repositories },
    repositorySets: { ...defaultState.repositorySets, ...initialState.repositorySets },
    repositoryBranchPolicies: {
      ...defaultState.repositoryBranchPolicies,
      ...initialState.repositoryBranchPolicies,
    },
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
    agentProfileRecentUse: mergeAgentProfileRecentUseState(
      defaultState.agentProfileRecentUse,
      initialState.agentProfileRecentUse ?? {},
    ),
    messages: { ...defaultState.messages, ...initialState.messages },
    messagePrompts: mergePromptHistoryState(initialState),
    turns: mergeTurnsState(defaultState.turns, initialState.turns, initialState.taskSessions),
    ...mergeTaskSessionState(initialState),
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

function mergeWorkspaceContextGeneration(initialState: HydrationState) {
  return initialState.workspaceContextGeneration ?? defaultState.workspaceContextGeneration;
}

// Split out of mergeInitialState to stay under the per-function line limit —
// these fields are the UI/panel slice of the merge and have no cross-field
// dependencies on the rest of DefaultState.
/** Merge the UI/panel slice fields (panels, quick chat, sidebar views/task prefs, review selection, notification) from hydration state over defaults. */
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
    threadViews: mergeThreadViewState(initialState),
    sidebarTaskPrefs: mergeSidebarTaskPrefsState(initialState),
    collapsedSubtaskParents:
      initialState.collapsedSubtaskParents ?? defaultState.collapsedSubtaskParents,
  };
}
