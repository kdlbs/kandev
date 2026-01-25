export type BackendMessageType =
  | 'kanban.update'
  | 'task.created'
  | 'task.updated'
  | 'task.deleted'
  | 'task.state_changed'
  | 'agent.updated'
  | 'agent.available.updated'
  | 'terminal.output'
  | 'diff.update'
  | 'session.git.event'
  | 'system.error'
  | 'workspace.created'
  | 'workspace.updated'
  | 'workspace.deleted'
  | 'board.created'
  | 'board.updated'
  | 'board.deleted'
  | 'column.created'
  | 'column.updated'
  | 'column.deleted'
  | 'session.message.added'
  | 'session.message.updated'
  | 'session.state_changed'
  | 'session.waiting_for_input'
  | 'session.agentctl_starting'
  | 'session.agentctl_ready'
  | 'session.agentctl_error'
  | 'session.turn.started'
  | 'session.turn.completed'
  | 'executor.created'
  | 'executor.updated'
  | 'executor.deleted'
  | 'environment.created'
  | 'environment.updated'
  | 'environment.deleted'
  | 'agent.profile.deleted'
  | 'agent.profile.created'
  | 'agent.profile.updated'
  | 'user.settings.updated'
  | 'session.workspace.file.changes'
  | 'session.shell.output'
  | 'session.process.output'
  | 'session.process.status';

export type BackendMessage<T extends BackendMessageType, P> = {
  id?: string;
  type: 'request' | 'response' | 'notification' | 'error';
  action: T;
  payload: P;
  timestamp?: string;
};

import type { AvailableAgent, TaskState } from '@/lib/types/http';
import type { GitEventPayload } from '@/lib/types/git-events';

export type KanbanUpdatePayload = {
  boardId: string;
  columns: Array<{ id: string; title: string; color?: string; position?: number }>;
  tasks: Array<{
    id: string;
    columnId: string;
    title: string;
    position?: number;
    description?: string;
    state?: TaskState;
  }>;
};

export type TaskEventPayload = {
  task_id: string;
  board_id: string;
  column_id: string;
  title: string;
  description?: string;
  state?: TaskState;
  priority?: number;
  position?: number;
  repository_id?: string;
};

export type AgentUpdatePayload = {
  agentId: string;
  status: 'idle' | 'running' | 'error';
  message?: string;
};

export type AgentAvailableUpdatedPayload = {
  agents: AvailableAgent[];
};

export type TerminalOutputPayload = {
  terminalId: string;
  data: string;
  stream?: 'stdout' | 'stderr';
};

export type DiffUpdatePayload = {
  taskId: string;
  files: Array<{
    path: string;
    status: 'A' | 'M' | 'D';
    plus: number;
    minus: number;
  }>;
};

export type SystemErrorPayload = {
  message: string;
  code?: string;
};

export type WorkspacePayload = {
  id: string;
  name: string;
  description?: string;
  owner_id?: string;
  default_executor_id?: string | null;
  default_environment_id?: string | null;
  default_agent_profile_id?: string | null;
  created_at?: string;
  updated_at?: string;
};

export type BoardPayload = {
  id: string;
  workspace_id: string;
  name: string;
  description?: string;
  created_at?: string;
  updated_at?: string;
};

export type ColumnPayload = {
  id: string;
  board_id: string;
  name: string;
  position: number;
  state: string;
  color: string;
  created_at?: string;
  updated_at?: string;
};

export type MessageAddedPayload = {
  task_id: string;
  message_id: string;
  session_id: string;
  author_type: 'user' | 'agent';
  author_id?: string;
  content: string;
  type?: string;
  metadata?: Record<string, unknown>;
  requests_input?: boolean;
  created_at: string;
};

export type TaskSessionStateChangedPayload = {
  task_id: string;
  session_id: string;
  old_state?: string;
  new_state: string;
  agent_profile_id?: string;
  agent_profile_snapshot?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
};

export type TaskSessionWaitingForInputPayload = {
  task_id: string;
  session_id: string;
  title: string;
  body: string;
};

export type TaskSessionAgentctlPayload = {
  task_id: string;
  session_id: string;
  agent_execution_id?: string;
  error_message?: string;
};

export type FileInfo = {
  path: string;
  status: 'modified' | 'added' | 'deleted' | 'untracked' | 'renamed';
  staged: boolean;
  additions?: number;
  deletions?: number;
  old_path?: string;
  diff?: string;
};

export type ProcessOutputPayload = {
  session_id: string;
  process_id: string;
  kind: string;
  stream: 'stdout' | 'stderr';
  data: string;
  timestamp?: string;
};

export type ProcessStatusPayload = {
  session_id: string;
  process_id: string;
  kind: string;
  script_name?: string;
  status: string;
  command?: string;
  working_dir?: string;
  exit_code?: number | null;
  timestamp?: string;
};

export type ExecutorPayload = {
  id: string;
  name: string;
  type: string;
  status: string;
  is_system: boolean;
  config?: Record<string, string>;
  created_at?: string;
  updated_at?: string;
};

export type EnvironmentPayload = {
  id: string;
  name: string;
  kind: string;
  is_system: boolean;
  worktree_root?: string;
  image_tag?: string;
  dockerfile?: string;
  build_config?: Record<string, string>;
  created_at?: string;
  updated_at?: string;
};

export type AgentProfilePayload = {
  id: string;
  agent_id: string;
  name: string;
  agent_display_name: string;
  model: string;
  auto_approve: boolean;
  dangerously_skip_permissions: boolean;
  allow_indexing: boolean;
  cli_passthrough?: boolean;
  plan: string;
  created_at?: string;
  updated_at?: string;
};

export type AgentProfileDeletedPayload = {
  profile: AgentProfilePayload;
};

export type AgentProfileChangedPayload = {
  profile: AgentProfilePayload;
};

export type UserSettingsUpdatedPayload = {
  user_id: string;
  workspace_id: string;
  board_id: string;
  repository_ids: string[];
  initial_setup_complete?: boolean;
  preferred_shell?: string;
  default_editor_id?: string;
  enable_preview_on_click?: boolean;
  updated_at?: string;
};

export type ShellOutputPayload = {
  task_id: string;
  session_id: string;
  type: 'output' | 'exit';
  data?: string;
  code?: number;
};

export type TurnEventPayload = {
  id: string;
  session_id: string;
  task_id: string;
  started_at: string;
  completed_at?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type BackendMessageMap = {
  'kanban.update': BackendMessage<'kanban.update', KanbanUpdatePayload>;
  'task.created': BackendMessage<'task.created', TaskEventPayload>;
  'task.updated': BackendMessage<'task.updated', TaskEventPayload>;
  'task.deleted': BackendMessage<'task.deleted', TaskEventPayload>;
  'task.state_changed': BackendMessage<'task.state_changed', TaskEventPayload>;
  'agent.updated': BackendMessage<'agent.updated', AgentUpdatePayload>;
  'agent.available.updated': BackendMessage<'agent.available.updated', AgentAvailableUpdatedPayload>;
  'terminal.output': BackendMessage<'terminal.output', TerminalOutputPayload>;
  'diff.update': BackendMessage<'diff.update', DiffUpdatePayload>;
  'session.git.event': BackendMessage<'session.git.event', GitEventPayload>;
  'system.error': BackendMessage<'system.error', SystemErrorPayload>;
  'workspace.created': BackendMessage<'workspace.created', WorkspacePayload>;
  'workspace.updated': BackendMessage<'workspace.updated', WorkspacePayload>;
  'workspace.deleted': BackendMessage<'workspace.deleted', WorkspacePayload>;
  'board.created': BackendMessage<'board.created', BoardPayload>;
  'board.updated': BackendMessage<'board.updated', BoardPayload>;
  'board.deleted': BackendMessage<'board.deleted', BoardPayload>;
  'column.created': BackendMessage<'column.created', ColumnPayload>;
  'column.updated': BackendMessage<'column.updated', ColumnPayload>;
  'column.deleted': BackendMessage<'column.deleted', ColumnPayload>;
  'session.message.added': BackendMessage<'session.message.added', MessageAddedPayload>;
  'session.message.updated': BackendMessage<'session.message.updated', MessageAddedPayload>;
  'session.state_changed': BackendMessage<'session.state_changed', TaskSessionStateChangedPayload>;
  'session.waiting_for_input': BackendMessage<'session.waiting_for_input', TaskSessionWaitingForInputPayload>;
  'session.agentctl_starting': BackendMessage<'session.agentctl_starting', TaskSessionAgentctlPayload>;
  'session.agentctl_ready': BackendMessage<'session.agentctl_ready', TaskSessionAgentctlPayload>;
  'session.agentctl_error': BackendMessage<'session.agentctl_error', TaskSessionAgentctlPayload>;
  'session.turn.started': BackendMessage<'session.turn.started', TurnEventPayload>;
  'session.turn.completed': BackendMessage<'session.turn.completed', TurnEventPayload>;
  'executor.created': BackendMessage<'executor.created', ExecutorPayload>;
  'executor.updated': BackendMessage<'executor.updated', ExecutorPayload>;
  'executor.deleted': BackendMessage<'executor.deleted', ExecutorPayload>;
  'environment.created': BackendMessage<'environment.created', EnvironmentPayload>;
  'environment.updated': BackendMessage<'environment.updated', EnvironmentPayload>;
  'environment.deleted': BackendMessage<'environment.deleted', EnvironmentPayload>;
  'agent.profile.deleted': BackendMessage<'agent.profile.deleted', AgentProfileDeletedPayload>;
  'agent.profile.created': BackendMessage<'agent.profile.created', AgentProfileChangedPayload>;
  'agent.profile.updated': BackendMessage<'agent.profile.updated', AgentProfileChangedPayload>;
  'user.settings.updated': BackendMessage<'user.settings.updated', UserSettingsUpdatedPayload>;
  'session.workspace.file.changes': BackendMessage<'session.workspace.file.changes', FileChangeNotificationPayload>;
  'session.shell.output': BackendMessage<'session.shell.output', ShellOutputPayload>;
  'session.process.output': BackendMessage<'session.process.output', ProcessOutputPayload>;
  'session.process.status': BackendMessage<'session.process.status', ProcessStatusPayload>;
};

// Workspace file types
export type FileTreeNode = {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
  children?: FileTreeNode[];
};

export type FileTreeResponse = {
  request_id?: string;
  root: FileTreeNode;
  error?: string;
};

export type FileContentResponse = {
  request_id?: string;
  path: string;
  content: string;
  size: number;
  error?: string;
};

// Single file change event (used within batched notifications)
export type FileChangeEvent = {
  timestamp: string;
  path: string;
  operation: 'create' | 'write' | 'remove' | 'rename' | 'chmod' | 'refresh';
  session_id: string;
  task_id: string;
  agent_id: string;
};

// Batched file change notification payload (multiple changes batched for efficiency)
export type FileChangeNotificationPayload = {
  session_id: string;
  changes: FileChangeEvent[];
};

// Open file tab for file viewer
export type OpenFileTab = {
  path: string;
  name: string;
  content: string;
};

// File extension to color mapping for file type indicators
export const FILE_EXTENSION_COLORS: Record<string, string> = {
  ts: 'bg-blue-500',
  tsx: 'bg-blue-400',
  js: 'bg-yellow-500',
  jsx: 'bg-yellow-400',
  go: 'bg-cyan-500',
  py: 'bg-green-500',
  rs: 'bg-orange-500',
  json: 'bg-amber-400',
  css: 'bg-purple-500',
  html: 'bg-red-500',
  md: 'bg-gray-400',
};
