// Export slice creators
export { createKanbanSlice, defaultKanbanState } from './kanban/kanban-slice';
export { createWorkspaceSlice, defaultWorkspaceState } from './workspace/workspace-slice';
export { createSettingsSlice, defaultSettingsState } from './settings/settings-slice';
export { createSessionSlice, defaultSessionState } from './session/session-slice';
export {
  createSessionRuntimeSlice,
  defaultSessionRuntimeState,
} from './session-runtime/session-runtime-slice';
export { createUISlice, defaultUIState } from './ui/ui-slice';

// Export types
export type { KanbanSlice, KanbanSliceState, KanbanSliceActions } from './kanban/types';
export type {
  WorkspaceSlice,
  WorkspaceSliceState,
  WorkspaceSliceActions,
} from './workspace/types';
export type { SettingsSlice, SettingsSliceState, SettingsSliceActions } from './settings/types';
export type { SessionSlice, SessionSliceState, SessionSliceActions } from './session/types';
export type {
  SessionRuntimeSlice,
  SessionRuntimeSliceState,
  SessionRuntimeSliceActions,
} from './session-runtime/types';
export type { UISlice, UISliceState, UISliceActions } from './ui/types';

// Re-export commonly used types from each domain
export type { KanbanState, BoardState, TaskState } from './kanban/types';
export type { WorkspaceState, RepositoriesState, RepositoryBranchesState } from './workspace/types';
export type {
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
} from './settings/types';
export type {
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
  TaskPlansState,
} from './session/types';
export type {
  TerminalState,
  ShellState,
  ProcessStatusEntry,
  ProcessState,
  FileInfo,
  GitStatusEntry,
  GitStatusState,
  SnapshotType,
  GitSnapshot,
  SessionCommit,
  CumulativeDiff,
  GitSnapshotsState,
  SessionCommitsState,
  ContextWindowEntry,
  ContextWindowState,
  AgentState,
  AvailableCommand,
  AvailableCommandsState,
} from './session-runtime/types';
export type {
  PreviewStage,
  PreviewViewMode,
  PreviewDevicePreset,
  PreviewPanelState,
  RightPanelState,
  DiffState,
  ConnectionState,
} from './ui/types';
export type { Repository, Branch } from '@/lib/types/http';
