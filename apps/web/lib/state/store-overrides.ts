import type { DefaultState } from "./default-state";

// buildStateOverrides picks the plain-state fields out of `merged` and re-asserts them
// after all slice spreads so that caller-supplied initialState wins over slice defaults.
// Note: collapsedSubtaskParents is intentionally omitted — createUISlice hydrates it
// from sessionStorage and we want that to win.
export function buildStateOverrides(m: DefaultState) {
  return {
    workflows: m.workflows,
    tasks: m.tasks,
    workspaces: m.workspaces,
    userSettings: m.userSettings,
    messages: m.messages,
    turns: m.turns,
    taskSessions: m.taskSessions,
    taskSessionsByTask: m.taskSessionsByTask,
    sessionAgentctl: m.sessionAgentctl,
    worktrees: m.worktrees,
    sessionWorktreesBySessionId: m.sessionWorktreesBySessionId,
    activeModel: m.activeModel,
    shell: m.shell,
    processes: m.processes,
    gitStatus: m.gitStatus,
    contextWindow: m.contextWindow,
    userShells: m.userShells,
    prepareProgress: m.prepareProgress,
    sessionModels: m.sessionModels,
    pendingPrUrlByTaskId: m.pendingPrUrlByTaskId,
    prFeedbackCache: m.prFeedbackCache,
    office: m.office,
    features: m.features,
    previewPanel: m.previewPanel,
    rightPanel: m.rightPanel,
    connection: m.connection,
    mobileKanban: m.mobileKanban,
    mobileSession: m.mobileSession,
    chatInput: m.chatInput,
    documentPanel: m.documentPanel,
    quickChat: m.quickChat,
    sessionFailureNotification: m.sessionFailureNotification,
    bottomTerminal: m.bottomTerminal,
  };
}
