import { createStore } from "zustand/vanilla";
import { immer } from "zustand/middleware/immer";
import { hydrateState, type HydrationOptions } from "./hydration/hydrator";
import type { Message, Turn } from "@/lib/types/http";
import type { SystemHealthResponse } from "@/lib/types/health";
import type { SsrInitialState } from "@/lib/ssr/initial-state";
import type { UISliceActions as UIA } from "./slices/ui/types";
import type * as UISliceTypes from "./slices/ui/types";
import { mergeInitialState } from "./default-state";
import {
  createKanbanSlice,
  createWorkspaceSlice,
  createSessionSlice,
  createSessionRuntimeSlice,
  createUISlice,
  createGitHubSlice,
  createGitLabSlice,
  createLinearSlice,
  createSystemSlice,
  defaultKanbanState,
  defaultWorkspaceState,
  defaultSessionState,
  defaultSessionRuntimeState,
  defaultUIState,
  defaultGitHubState,
  defaultGitLabState,
  defaultLinearState,
  defaultSystemState,
  type ProcessStatusEntry,
  type SessionCommit,
  type ContextWindowEntry,
  type PreviewStage,
  type PreviewViewMode,
  type PreviewDevicePreset,
  type ConnectionState,
  type SystemSliceActions,
  type GitHubSliceActions,
} from "./slices";
import type {
  AvailableCommand,
  AgentCapabilitiesEntry,
  PromptUsageEntry,
  UserShellInfo,
} from "./slices/session-runtime/types";

// Re-export all types from slices for backwards compatibility.
export type * from "./store-reexports";
import type { TaskMR } from "@/lib/types/gitlab";
import type { LinearIssueWatch } from "@/lib/types/linear";

// Combined AppState type
export type AppState = {
  // Kanban slice (client-only: active workflow + active task/session selection)
  workflows: (typeof defaultKanbanState)["workflows"];
  tasks: (typeof defaultKanbanState)["tasks"];

  // Workspace slice (client-only: active workspace selection)
  workspaces: (typeof defaultWorkspaceState)["workspaces"];

  // Session slice
  messages: (typeof defaultSessionState)["messages"];
  turns: (typeof defaultSessionState)["turns"];
  pendingModel: (typeof defaultSessionState)["pendingModel"];
  activeModel: (typeof defaultSessionState)["activeModel"];
  taskPlans: (typeof defaultSessionState)["taskPlans"];
  queue: (typeof defaultSessionState)["queue"];

  // Session Runtime slice
  terminal: (typeof defaultSessionRuntimeState)["terminal"];
  shell: (typeof defaultSessionRuntimeState)["shell"];
  processes: (typeof defaultSessionRuntimeState)["processes"];
  // gitStatus / sessionMode / sessionModels / sessionTodos / sessionPollMode
  // removed — these server fields now live in the TanStack Query cache.
  environmentIdBySessionId: (typeof defaultSessionRuntimeState)["environmentIdBySessionId"];
  sessionCommits: (typeof defaultSessionRuntimeState)["sessionCommits"];
  contextWindow: (typeof defaultSessionRuntimeState)["contextWindow"];
  agents: (typeof defaultSessionRuntimeState)["agents"];
  availableCommands: (typeof defaultSessionRuntimeState)["availableCommands"];
  userShells: (typeof defaultSessionRuntimeState)["userShells"];
  agentCapabilities: (typeof defaultSessionRuntimeState)["agentCapabilities"];
  promptUsage: (typeof defaultSessionRuntimeState)["promptUsage"];

  // GitHub slice (client-only — server state lives in TanStack Query)
  pendingPrUrlByTaskId: (typeof defaultGitHubState)["pendingPrUrlByTaskId"];

  // GitLab slice
  taskMRs: (typeof defaultGitLabState)["taskMRs"];

  // Linear slice
  linearIssueWatches: (typeof defaultLinearState)["linearIssueWatches"];

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
  bottomTerminal: (typeof defaultUIState)["bottomTerminal"];
  sidebarViews: (typeof defaultUIState)["sidebarViews"];
  collapsedSubtaskParents: (typeof defaultUIState)["collapsedSubtaskParents"];
  kanbanPreviewedTaskId: (typeof defaultUIState)["kanbanPreviewedTaskId"];
  sidebarTaskPrefs: (typeof defaultUIState)["sidebarTaskPrefs"];

  // GitLab actions
  setTaskMRs: (mrs: Record<string, TaskMR[]>) => void;
  setTaskMR: (taskId: string, mr: TaskMR) => void;
  resetTaskMRs: () => void;

  // Linear actions
  setLinearIssueWatches: (watches: LinearIssueWatch[]) => void;
  setLinearIssueWatchesLoading: (loading: boolean) => void;
  addLinearIssueWatch: (watch: LinearIssueWatch) => void;
  updateLinearIssueWatch: (watch: LinearIssueWatch) => void;
  removeLinearIssueWatch: (id: string) => void;
  resetLinearIssueWatches: () => void;

  // Actions from all slices
  hydrate: (state: SsrInitialState, options?: HydrationOptions) => void;
  setActiveWorkspace: (workspaceId: string | null) => void;
  setActiveWorkflow: (workflowId: string | null) => void;
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
  toggleBottomTerminal: () => void;
  openBottomTerminalWithCommand: (command: string) => void;
  clearBottomTerminalCommand: () => void;
  setMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
  addMessage: (message: Message) => void;
  addTurn: (turn: Turn) => void;
  completeTurn: (sessionId: string, turnId: string, completedAt: string) => void;
  setActiveTurn: (sessionId: string, turnId: string | null) => void;
  updateMessage: (message: Message) => void;
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
  setPendingModel: (sessionId: string, modelId: string) => void;
  clearPendingModel: (sessionId: string) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  // Task plan client-state actions (server data lives in TanStack Query)
  setTaskPlanSaving: (taskId: string, saving: boolean) => void;
  clearTaskPlan: (taskId: string) => void;
  markTaskPlanSeen: (taskId: string, updatedAt?: string | null) => void;
  // Plan revision client-state actions
  cachePlanRevisionContent: (revisionId: string, content: string) => void;
  // Plan revision preview + compare actions
  setPreviewRevision: (taskId: string, revisionId: string | null) => void;
  toggleComparePair: (taskId: string, revisionId: string) => void;
  clearComparePair: (taskId: string) => void;
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
  // Agent capabilities actions
  setAgentCapabilities: (sessionId: string, caps: AgentCapabilitiesEntry) => void;
  // Prompt usage actions
  setPromptUsage: (sessionId: string, usage: PromptUsageEntry) => void;
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
  migrateLocalViewsToBackend: UIA["migrateLocalViewsToBackend"];
  setKanbanPreviewedTaskId: UIA["setKanbanPreviewedTaskId"];
  togglePinnedTask: UIA["togglePinnedTask"];
  setSidebarTaskOrder: UIA["setSidebarTaskOrder"];
  setSubtaskOrder: UIA["setSubtaskOrder"];
  removeTaskFromSidebarPrefs: UIA["removeTaskFromSidebarPrefs"];
} & GitHubSliceActions &
  SystemSliceActions;

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
      ...createSessionSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSessionRuntimeSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createGitHubSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createGitLabSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createLinearSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSystemSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createUISlice(set as any, get as any, api as any),
      // Override state with merged initial state
      workflows: merged.workflows,
      tasks: merged.tasks,
      workspaces: merged.workspaces,
      messages: merged.messages,
      turns: merged.turns,
      pendingModel: merged.pendingModel,
      activeModel: merged.activeModel,
      queue: merged.queue,
      terminal: merged.terminal,
      shell: merged.shell,
      processes: merged.processes,
      contextWindow: merged.contextWindow,
      agents: merged.agents,
      userShells: merged.userShells,
      agentCapabilities: merged.agentCapabilities,
      promptUsage: merged.promptUsage,
      pendingPrUrlByTaskId: merged.pendingPrUrlByTaskId,
      taskMRs: merged.taskMRs,
      linearIssueWatches: merged.linearIssueWatches,
      system: merged.system,
      previewPanel: merged.previewPanel,
      rightPanel: merged.rightPanel,
      diffs: merged.diffs,
      connection: merged.connection,
      mobileKanban: merged.mobileKanban,
      mobileSession: merged.mobileSession,
      chatInput: merged.chatInput,
      documentPanel: merged.documentPanel,
      systemHealth: merged.systemHealth,
      quickChat: merged.quickChat,
      sessionFailureNotification: merged.sessionFailureNotification,
      bottomTerminal: merged.bottomTerminal,
      // Note: collapsedSubtaskParents is intentionally not overridden here —
      // createUISlice hydrates it from sessionStorage and we want that to win.
      // Add hydrate method
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      hydrate: (state, options) => set((draft) => hydrateState(draft as any, state, options)),
    })),
  );
}

export type StoreProviderProps = {
  children: React.ReactNode;
  initialState?: Partial<AppState>;
};
