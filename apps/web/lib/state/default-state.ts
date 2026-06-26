import {
  defaultKanbanState,
  defaultWorkspaceState,
  defaultSettingsState,
  defaultSessionState,
  defaultSessionRuntimeState,
  defaultUIState,
  defaultGitHubState,
  defaultOfficeState,
} from "./slices";

export const defaultState = {
  workflows: defaultKanbanState.workflows,
  tasks: defaultKanbanState.tasks,
  workspaces: defaultWorkspaceState.workspaces,
  userSettings: defaultSettingsState.userSettings,
  messages: defaultSessionState.messages,
  turns: defaultSessionState.turns,
  taskSessions: defaultSessionState.taskSessions,
  taskSessionsByTask: defaultSessionState.taskSessionsByTask,
  sessionAgentctl: defaultSessionState.sessionAgentctl,
  activeModel: defaultSessionState.activeModel,
  taskPlans: defaultSessionState.taskPlans,
  shell: defaultSessionRuntimeState.shell,
  processes: defaultSessionRuntimeState.processes,
  gitStatus: defaultSessionRuntimeState.gitStatus,
  environmentIdBySessionId: defaultSessionRuntimeState.environmentIdBySessionId,
  sessionCommits: defaultSessionRuntimeState.sessionCommits,
  contextWindow: defaultSessionRuntimeState.contextWindow,
  userShells: defaultSessionRuntimeState.userShells,
  prepareProgress: defaultSessionRuntimeState.prepareProgress,
  sessionModels: defaultSessionRuntimeState.sessionModels,
  pendingPrUrlByTaskId: defaultGitHubState.pendingPrUrlByTaskId,
  prFeedbackCache: defaultGitHubState.prFeedbackCache,
  office: defaultOfficeState.office,
  previewPanel: defaultUIState.previewPanel,
  rightPanel: defaultUIState.rightPanel,
  connection: defaultUIState.connection,
  mobileKanban: defaultUIState.mobileKanban,
  mobileSession: defaultUIState.mobileSession,
  chatInput: defaultUIState.chatInput,
  documentPanel: defaultUIState.documentPanel,
  quickChat: defaultUIState.quickChat,
  sessionFailureNotification: defaultUIState.sessionFailureNotification,
  bottomTerminal: defaultUIState.bottomTerminal,
  sidebarViews: defaultUIState.sidebarViews,
  collapsedSubtaskParents: defaultUIState.collapsedSubtaskParents,
  kanbanPreviewedTaskId: defaultUIState.kanbanPreviewedTaskId,
  sidebarTaskPrefs: defaultUIState.sidebarTaskPrefs,
};

export type DefaultState = typeof defaultState;

export function mergeInitialState(initialState?: Partial<DefaultState>): DefaultState {
  if (!initialState) return defaultState;
  const knownInitialState = pickDefaultStateFields(initialState);

  return {
    ...defaultState,
    ...knownInitialState,
    workflows: {
      ...defaultState.workflows,
      activeId: initialState.workflows?.activeId ?? defaultState.workflows.activeId,
    },
    tasks: { ...defaultState.tasks, ...initialState.tasks },
    workspaces: {
      ...defaultState.workspaces,
      activeId: initialState.workspaces?.activeId ?? defaultState.workspaces.activeId,
    },
    userSettings: { ...defaultState.userSettings, ...initialState.userSettings },
    messages: { ...defaultState.messages, ...initialState.messages },
    turns: { ...defaultState.turns, ...initialState.turns },
    taskSessions: { ...defaultState.taskSessions, ...initialState.taskSessions },
    taskSessionsByTask: { ...defaultState.taskSessionsByTask, ...initialState.taskSessionsByTask },
    sessionAgentctl: { ...defaultState.sessionAgentctl, ...initialState.sessionAgentctl },
    activeModel: { ...defaultState.activeModel, ...initialState.activeModel },
    taskPlans: { ...defaultState.taskPlans, ...initialState.taskPlans },
    shell: { ...defaultState.shell, ...initialState.shell },
    processes: { ...defaultState.processes, ...initialState.processes },
    gitStatus: { ...defaultState.gitStatus, ...initialState.gitStatus },
    sessionCommits: { ...defaultState.sessionCommits, ...initialState.sessionCommits },
    contextWindow: { ...defaultState.contextWindow, ...initialState.contextWindow },
    userShells: { ...defaultState.userShells, ...initialState.userShells },
    prepareProgress: { ...defaultState.prepareProgress, ...initialState.prepareProgress },
    sessionModels: { ...defaultState.sessionModels, ...initialState.sessionModels },
    pendingPrUrlByTaskId: {
      ...defaultState.pendingPrUrlByTaskId,
      ...initialState.pendingPrUrlByTaskId,
    },
    prFeedbackCache: { ...defaultState.prFeedbackCache, ...initialState.prFeedbackCache },
    office: { ...defaultState.office, ...initialState.office },
    previewPanel: { ...defaultState.previewPanel, ...initialState.previewPanel },
    rightPanel: { ...defaultState.rightPanel, ...initialState.rightPanel },
    connection: { ...defaultState.connection, ...initialState.connection },
    mobileKanban: { ...defaultState.mobileKanban, ...initialState.mobileKanban },
    mobileSession: { ...defaultState.mobileSession, ...initialState.mobileSession },
    chatInput: { ...defaultState.chatInput, ...initialState.chatInput },
    documentPanel: { ...defaultState.documentPanel, ...initialState.documentPanel },
    quickChat: { ...defaultState.quickChat, ...initialState.quickChat },
    sessionFailureNotification:
      initialState.sessionFailureNotification ?? defaultState.sessionFailureNotification,
    bottomTerminal: { ...defaultState.bottomTerminal, ...initialState.bottomTerminal },
    sidebarViews: { ...defaultState.sidebarViews, ...initialState.sidebarViews },
    collapsedSubtaskParents:
      initialState.collapsedSubtaskParents ?? defaultState.collapsedSubtaskParents,
  };
}

function pickDefaultStateFields(initialState: Partial<DefaultState>): Partial<DefaultState> {
  const picked: Record<string, unknown> = {};
  const source = initialState as Record<string, unknown>;
  for (const key of Object.keys(defaultState)) {
    if (Object.prototype.hasOwnProperty.call(source, key)) {
      picked[key] = source[key];
    }
  }
  return picked as Partial<DefaultState>;
}
