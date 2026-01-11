export type BackendMessageType =
  | 'kanban.update'
  | 'task.update'
  | 'agent.update'
  | 'terminal.output'
  | 'diff.update'
  | 'system.error';

export type BackendMessage<T extends BackendMessageType, P> = {
  type: T;
  payload: P;
  requestId?: string;
  timestamp?: string;
};

export type KanbanUpdatePayload = {
  boardId: string;
  columns: Array<{ id: string; title: string }>;
  tasks: Array<{ id: string; columnId: string; title: string }>;
};

export type TaskUpdatePayload = {
  taskId: string;
  status: string;
  title?: string;
  description?: string;
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

export type BackendMessageMap = {
  'kanban.update': BackendMessage<'kanban.update', KanbanUpdatePayload>;
  'task.update': BackendMessage<'task.update', TaskUpdatePayload>;
  'agent.update': BackendMessage<'agent.update', AgentUpdatePayload>;
  'terminal.output': BackendMessage<'terminal.output', TerminalOutputPayload>;
  'diff.update': BackendMessage<'diff.update', DiffUpdatePayload>;
  'system.error': BackendMessage<'system.error', SystemErrorPayload>;
};
