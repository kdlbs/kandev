// Export slice creators
export { createKanbanSlice, defaultKanbanState } from "./kanban/kanban-slice";
export { createWorkspaceSlice, defaultWorkspaceState } from "./workspace/workspace-slice";
export { createSettingsSlice, defaultSettingsState } from "./settings/settings-slice";
export { createSessionSlice, defaultSessionState } from "./session/session-slice";
export {
  createSessionRuntimeSlice,
  defaultSessionRuntimeState,
} from "./session-runtime/session-runtime-slice";
export { createUISlice, defaultUIState } from "./ui/ui-slice";
export { createGitHubSlice, defaultGitHubState } from "./github/github-slice";
export { createOfficeSlice, defaultOfficeState } from "./office/office-slice";
export { defaultFeaturesState } from "./features/features-slice";

// Export types
export type { KanbanSlice, KanbanSliceState, KanbanSliceActions } from "./kanban/types";
export type { WorkspaceSlice, WorkspaceSliceState, WorkspaceSliceActions } from "./workspace/types";
export type { SettingsSlice, SettingsSliceState, SettingsSliceActions } from "./settings/types";
export type { SessionSlice, SessionSliceState, SessionSliceActions } from "./session/types";
export type {
  SessionRuntimeSlice,
  SessionRuntimeSliceState,
  SessionRuntimeSliceActions,
} from "./session-runtime/types";
export type { UISlice, UISliceState, UISliceActions } from "./ui/types";
export type { GitHubSlice, GitHubSliceState, GitHubSliceActions } from "./github/types";
export type { OfficeSlice, OfficeSliceState, OfficeSliceActions } from "./office/types";
export type { FeatureFlags, FeatureName } from "./features/types";

// Re-export commonly used types from each domain
export type {
  KanbanState,
  KanbanMultiState,
  WorkflowSnapshotData,
  WorkflowItem,
  WorkflowsState,
  TaskState,
} from "./kanban/types";
export type { WorkspaceState } from "./workspace/types";
export type { AgentProfileOption, UserSettingsState } from "./settings/types";
export type {
  MessagesState,
  TurnsState,
  TaskSessionsState,
  TaskSessionsByTaskState,
  SessionAgentctlStatus,
  SessionAgentctlState,
  Worktree,
  ActiveModelState,
  TaskPlansState,
  QueueStatus,
  QueuedMessage,
} from "./session/types";
export type {
  ShellState,
  ProcessStatusEntry,
  ProcessState,
  FileInfo,
  GitStatusEntry,
  GitStatusState,
  SessionCommit,
  CumulativeDiff,
  SessionCommitsState,
  ContextWindowEntry,
  ContextWindowState,
  AvailableCommand,
  UserShellInfo,
  UserShellKind,
  UserShellState,
  UserShellPTYStatus,
  UserShellsState,
  PrepareStepInfo,
  SessionPrepareState,
  PrepareProgressState,
} from "./session-runtime/types";
export type {
  PreviewStage,
  PreviewViewMode,
  PreviewDevicePreset,
  PreviewPanelState,
  RightPanelState,
  ConnectionState,
  MobileKanbanState,
  MobileSessionPanel,
  MobileSessionState,
  ActiveDocument,
  DocumentPanelState,
} from "./ui/types";
export type {
  AgentProfile,
  Skill,
  Project,
  Approval,
  ActivityEntry,
  CostSummary,
  BudgetPolicy,
  Routine,
  InboxItem,
  Run,
  DashboardData,
} from "./office/types";
export type { Repository, Branch } from "@/lib/types/http";
