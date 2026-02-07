import type { Message, TaskSession, Turn, TaskPlan } from '@/lib/types/http';

export type MessagesState = {
  bySession: Record<string, Message[]>;
  metaBySession: Record<
    string,
    {
      isLoading: boolean;
      hasMore: boolean;
      oldestCursor: string | null;
    }
  >;
};

export type TurnsState = {
  bySession: Record<string, Turn[]>;
  activeBySession: Record<string, string | null>; // sessionId -> active turnId
};

export type TaskSessionsState = {
  items: Record<string, TaskSession>;
};

export type TaskSessionsByTaskState = {
  itemsByTaskId: Record<string, TaskSession[]>;
  loadingByTaskId: Record<string, boolean>;
  loadedByTaskId: Record<string, boolean>;
};

export type SessionAgentctlStatus = {
  status: 'starting' | 'ready' | 'error';
  errorMessage?: string;
  agentExecutionId?: string;
  updatedAt?: string;
};

export type SessionAgentctlState = {
  itemsBySessionId: Record<string, SessionAgentctlStatus>;
};

export type Worktree = {
  id: string;
  sessionId: string;
  repositoryId?: string;
  path?: string;
  branch?: string;
};

export type WorktreesState = {
  items: Record<string, Worktree>;
};

export type SessionWorktreesState = {
  itemsBySessionId: Record<string, string[]>;
};

export type PendingModelState = {
  bySessionId: Record<string, string>;
};

export type ActiveModelState = {
  bySessionId: Record<string, string>;
};

export type TaskPlansState = {
  byTaskId: Record<string, TaskPlan | null>;
  loadingByTaskId: Record<string, boolean>;
  loadedByTaskId: Record<string, boolean>;
  savingByTaskId: Record<string, boolean>;
};

export type QueuedMessage = {
  id: string;
  session_id: string;
  task_id: string;
  content: string;
  model?: string;
  plan_mode: boolean;
  attachments?: Array<{ type: string; data: string; mime_type: string }>;
  queued_at: string;
  queued_by?: string;
};

export type QueueStatus = {
  is_queued: boolean;
  message?: QueuedMessage | null;
};

export type QueueState = {
  bySessionId: Record<string, QueueStatus>;
  isLoading: Record<string, boolean>;
};

export type SessionSliceState = {
  messages: MessagesState;
  turns: TurnsState;
  taskSessions: TaskSessionsState;
  taskSessionsByTask: TaskSessionsByTaskState;
  sessionAgentctl: SessionAgentctlState;
  worktrees: WorktreesState;
  sessionWorktreesBySessionId: SessionWorktreesState;
  pendingModel: PendingModelState;
  activeModel: ActiveModelState;
  taskPlans: TaskPlansState;
  queue: QueueState;
};

export type SessionSliceActions = {
  setMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null }
  ) => void;
  addMessage: (message: Message) => void;
  updateMessage: (message: Message) => void;
  prependMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null }
  ) => void;
  setMessagesMetadata: (
    sessionId: string,
    meta: { hasMore?: boolean; isLoading?: boolean; oldestCursor?: string | null }
  ) => void;
  setMessagesLoading: (sessionId: string, loading: boolean) => void;
  addTurn: (turn: Turn) => void;
  completeTurn: (sessionId: string, turnId: string, completedAt: string) => void;
  setActiveTurn: (sessionId: string, turnId: string | null) => void;
  setTaskSession: (session: TaskSession) => void;
  setTaskSessionsForTask: (taskId: string, sessions: TaskSession[]) => void;
  setTaskSessionsLoading: (taskId: string, loading: boolean) => void;
  setSessionAgentctlStatus: (sessionId: string, status: SessionAgentctlStatus) => void;
  setWorktree: (worktree: Worktree) => void;
  setSessionWorktrees: (sessionId: string, worktreeIds: string[]) => void;
  setPendingModel: (sessionId: string, modelId: string) => void;
  clearPendingModel: (sessionId: string) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  // Task plan actions
  setTaskPlan: (taskId: string, plan: TaskPlan | null) => void;
  setTaskPlanLoading: (taskId: string, loading: boolean) => void;
  setTaskPlanSaving: (taskId: string, saving: boolean) => void;
  clearTaskPlan: (taskId: string) => void;
  // Queue actions
  setQueueStatus: (sessionId: string, status: QueueStatus) => void;
  setQueueLoading: (sessionId: string, loading: boolean) => void;
  clearQueueStatus: (sessionId: string) => void;
};

export type SessionSlice = SessionSliceState & SessionSliceActions;
