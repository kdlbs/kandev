import type { Message, TaskSession, Turn } from "@/lib/types/http";

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
  status: "starting" | "ready" | "error";
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

export type ActiveModelState = {
  bySessionId: Record<string, string>;
};

/** Ordered slot pair for the compare-revisions feature. Either slot may be
 * null. Reducers enforce a 2-slot cap and reject duplicates. */
export type ComparePair = [string | null, string | null];

export type TaskPlansState = {
  // Phase 6: preview + compare state
  previewRevisionIdByTaskId: Record<string, string | null>;
  comparePairByTaskId: Record<string, ComparePair>;
  // From main: tracks the last `updated_at` the user has seen, so the panel
  // can flag unseen-changes after agent writes between visits.
  lastSeenUpdatedAtByTaskId: Record<string, string>;
};

export type QueuedMessageMetadata = Record<string, unknown> & {
  workflow_message?: boolean;
  workflow_auto_start?: boolean;
  workflow_step_id?: string;
  workflow_step_name?: string;
  workflow_step_color?: string;
  sender_task_id?: string;
  sender_task_title?: string;
  sender_session_id?: string;
};

export type QueuedMessage = {
  id: string;
  session_id: string;
  task_id: string;
  position?: number;
  content: string;
  model?: string;
  plan_mode: boolean;
  attachments?: Array<{
    type: string;
    data: string;
    mime_type: string;
    name?: string;
    delivery_mode?: "prompt" | "path";
  }>;
  metadata?: QueuedMessageMetadata;
  queued_at: string;
  queued_by?: string;
};

export type QueueStatus = {
  entries: QueuedMessage[];
  count: number;
  max: number;
};

export type SessionSliceState = {
  messages: MessagesState;
  turns: TurnsState;
  taskSessions: TaskSessionsState;
  taskSessionsByTask: TaskSessionsByTaskState;
  sessionAgentctl: SessionAgentctlState;
  activeModel: ActiveModelState;
  taskPlans: TaskPlansState;
};

export type SessionSliceActions = {
  setMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
  addMessage: (message: Message) => void;
  updateMessage: (message: Message) => void;
  /**
   * Idempotent full-snapshot merge: reconciles `messages` against the current
   * stored array, preserving object identity for unchanged messages and the
   * array reference itself when nothing changed (see `reconcileMessages`). Used
   * by periodic refetches so a no-op tick triggers zero re-renders.
   */
  mergeMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
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
  addTurn: (turn: Turn) => void;
  completeTurn: (
    sessionId: string,
    turnId: string,
    completedAt: string,
    metadata?: Record<string, unknown>,
  ) => void;
  setActiveTurn: (sessionId: string, turnId: string | null) => void;
  setTaskSession: (session: TaskSession) => void;
  removeTaskSession: (taskId: string, sessionId: string) => void;
  setTaskSessionsForTask: (taskId: string, sessions: TaskSession[]) => void;
  upsertTaskSessionFromEvent: (taskId: string, session: TaskSession) => void;
  setTaskSessionsLoading: (taskId: string, loading: boolean) => void;
  setSessionAgentctlStatus: (sessionId: string, status: SessionAgentctlStatus) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  // Task plan UI actions. The plan DTO itself is TanStack Query-owned.
  hydrateTaskPlanLastSeen: (taskId: string) => void;
  markTaskPlanSeen: (taskId: string, updatedAt?: string | null) => void;
  // Phase 6: preview + compare actions
  setPreviewRevision: (taskId: string, revisionId: string | null) => void;
  toggleComparePair: (taskId: string, revisionId: string) => void;
  clearComparePair: (taskId: string) => void;
};

export type SessionSlice = SessionSliceState & SessionSliceActions;
