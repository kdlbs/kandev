export type BackendMessageType =
  | 'kanban.update'
  | 'task.created'
  | 'task.updated'
  | 'task.deleted'
  | 'task.state_changed'
  | 'agent.updated'
  | 'terminal.output'
  | 'diff.update'
  | 'git.status'
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
  | 'message.added'
  | 'message.updated'
  | 'task_session.state_changed'
  | 'task_session.waiting_for_input'
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
  | 'workspace.file.changes'
  | 'shell.output';

export type BackendMessage<T extends BackendMessageType, P> = {
  id?: string;
  type: 'request' | 'response' | 'notification' | 'error';
  action: T;
  payload: P;
  timestamp?: string;
};

import type { TaskState } from '@/lib/types/http';

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
  task_session_id: string;
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
  task_session_id: string;
  old_state?: string;
  new_state: string;
};

export type TaskSessionWaitingForInputPayload = {
  task_id: string;
  task_session_id: string;
  title: string;
  body: string;
};

export type FileInfo = {
  path: string;
  status: 'modified' | 'added' | 'deleted' | 'untracked' | 'renamed';
  additions?: number;
  deletions?: number;
  old_path?: string;
  diff?: string;
};

export type GitStatusPayload = {
  task_id: string;
  branch: string;
  remote_branch?: string;
  modified: string[];
  added: string[];
  deleted: string[];
  untracked: string[];
  renamed: string[];
  ahead: number;
  behind: number;
  files: Record<string, FileInfo>;
  timestamp: string;
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
  model: string;
  auto_approve: boolean;
  dangerously_skip_permissions: boolean;
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
  updated_at?: string;
};

export type ShellOutputPayload = {
  task_id: string;
  type: 'output' | 'exit';
  data?: string;
  code?: number;
};

export type BackendMessageMap = {
  'kanban.update': BackendMessage<'kanban.update', KanbanUpdatePayload>;
  'task.created': BackendMessage<'task.created', TaskEventPayload>;
  'task.updated': BackendMessage<'task.updated', TaskEventPayload>;
  'task.deleted': BackendMessage<'task.deleted', TaskEventPayload>;
  'task.state_changed': BackendMessage<'task.state_changed', TaskEventPayload>;
  'agent.updated': BackendMessage<'agent.updated', AgentUpdatePayload>;
  'terminal.output': BackendMessage<'terminal.output', TerminalOutputPayload>;
  'diff.update': BackendMessage<'diff.update', DiffUpdatePayload>;
  'git.status': BackendMessage<'git.status', GitStatusPayload>;
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
  'message.added': BackendMessage<'message.added', MessageAddedPayload>;
  'message.updated': BackendMessage<'message.updated', MessageAddedPayload>;
  'task_session.state_changed': BackendMessage<'task_session.state_changed', TaskSessionStateChangedPayload>;
  'task_session.waiting_for_input': BackendMessage<'task_session.waiting_for_input', TaskSessionWaitingForInputPayload>;
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
  'workspace.file.changes': BackendMessage<'workspace.file.changes', FileChangeNotificationPayload>;
  'shell.output': BackendMessage<'shell.output', ShellOutputPayload>;
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

export type FileChangeNotificationPayload = {
  timestamp: string;
  path: string;
  operation: 'create' | 'write' | 'remove' | 'rename' | 'chmod' | 'refresh';
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
