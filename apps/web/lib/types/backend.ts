export type BackendMessageType =
  | 'kanban.update'
  | 'task.created'
  | 'task.updated'
  | 'task.deleted'
  | 'task.state_changed'
  | 'agent.updated'
  | 'terminal.output'
  | 'diff.update'
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
  | 'comment.added';

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

export type CommentAddedPayload = {
  task_id: string;
  comment_id: string;
  author_type: 'user' | 'agent';
  author_id?: string;
  content: string;
  type?: string;
  metadata?: Record<string, unknown>;
  requests_input?: boolean;
  created_at: string;
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
  'comment.added': BackendMessage<'comment.added', CommentAddedPayload>;
};
