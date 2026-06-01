import type { Message, Turn } from "@/lib/types/http";

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

/**
 * Worktree view-model derived from a TaskSession's worktree_* fields. No longer
 * a Zustand slice — built on demand by useSessionWorktrees / useWorktree from
 * the canonical TaskSession TQ cache (qk.taskSession.byId / .byTask).
 */
export type Worktree = {
  id: string;
  sessionId: string;
  repositoryId?: string;
  path?: string;
  branch?: string;
};

export type PendingModelState = {
  bySessionId: Record<string, string>;
};

export type ActiveModelState = {
  bySessionId: Record<string, string>;
};

/** Ordered slot pair for the compare-revisions feature. Either slot may be
 * null. Reducers enforce a 2-slot cap and reject duplicates. */
export type ComparePair = [string | null, string | null];

// CLIENT-only task-plan state. The plan + revisions list (server data) now
// live in the TanStack Query cache (qk.taskSession.plans / plansRevisions);
// only these client-owned fields remain in Zustand.
export type TaskPlansState = {
  savingByTaskId: Record<string, boolean>;
  revisionContentCache: Record<string, string>; // revisionId -> content
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
  attachments?: Array<{ type: string; data: string; mime_type: string }>;
  metadata?: QueuedMessageMetadata;
  queued_at: string;
  queued_by?: string;
};

/** Capacity info kept alongside the entry list. */
export type QueueMeta = {
  count: number;
  max: number;
};

export type QueueStatus = {
  entries: QueuedMessage[];
  count: number;
  max: number;
};

export type QueueState = {
  /** Ordered list of pending entries per session (FIFO; head at index 0). */
  bySessionId: Record<string, QueuedMessage[]>;
  /** Per-session capacity snapshot from the latest server response. */
  metaBySessionId: Record<string, QueueMeta>;
  isLoading: Record<string, boolean>;
};

export type SessionSliceState = {
  messages: MessagesState;
  turns: TurnsState;
  pendingModel: PendingModelState;
  activeModel: ActiveModelState;
  taskPlans: TaskPlansState;
  queue: QueueState;
};

export type SessionSliceActions = {
  setMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
  addMessage: (message: Message) => void;
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
  addTurn: (turn: Turn) => void;
  completeTurn: (sessionId: string, turnId: string, completedAt: string) => void;
  setActiveTurn: (sessionId: string, turnId: string | null) => void;
  setPendingModel: (sessionId: string, modelId: string) => void;
  clearPendingModel: (sessionId: string) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  // Task plan client-state actions (server data lives in TanStack Query)
  setTaskPlanSaving: (taskId: string, saving: boolean) => void;
  clearTaskPlan: (taskId: string) => void;
  markTaskPlanSeen: (taskId: string, updatedAt?: string | null) => void;
  // Revision client-state actions
  cachePlanRevisionContent: (revisionId: string, content: string) => void;
  // Phase 6: preview + compare actions
  setPreviewRevision: (taskId: string, revisionId: string | null) => void;
  toggleComparePair: (taskId: string, revisionId: string) => void;
  clearComparePair: (taskId: string) => void;
  // Queue actions
  setQueueEntries: (sessionId: string, entries: QueuedMessage[], meta: QueueMeta) => void;
  removeQueueEntry: (sessionId: string, entryId: string) => void;
  setQueueLoading: (sessionId: string, loading: boolean) => void;
  clearQueueStatus: (sessionId: string) => void;
};

export type SessionSlice = SessionSliceState & SessionSliceActions;
