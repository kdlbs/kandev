import type {
  Message,
  TaskPendingAction,
  TaskPendingActionRevision,
  TaskSession,
  Turn,
  TaskPlan,
  TaskPlanRevision,
  TaskWalkthrough,
} from "@/lib/types/http";
import type { EntityReference } from "@/lib/types/entity-reference";

export type MessagesState = {
  bySession: Record<string, Message[]>;
  metaBySession: Record<
    string,
    {
      /** Initial/refetch loading (never touched by older-page merges). */
      isLoading: boolean;
      /** Older-page request in flight (set by the shared pagination coordinator). */
      isLoadingMore: boolean;
      /** True after an authoritative newest-window response or boot payload. */
      historyInitialized: boolean;
      hasMore: boolean;
      oldestCursor: string | null;
    }
  >;
};

/** Prompts are fetched independently from the transcript with their own page metadata. */
export type PromptsState = MessagesState & {
  /** Incremented when a session is removed to reject stale prompt requests. */
  generationBySession: Record<string, number>;
};

export type TurnsState = {
  bySession: Record<string, Turn[]>;
  activeBySession: Record<string, string | null>; // sessionId -> active turnId
  /**
   * Sessions whose FULL persisted turn history has entered the store (SSR
   * hydration or a complete REST fetch). Distinct from `bySession` presence:
   * WS `session.turn.*` events seed individual live turns without the history,
   * so `bySession[sessionId]` being non-empty is NOT proof the history is
   * loaded. The debug metadata dialog and turn-derived UI resolve turns only
   * when this marker is set.
   */
  loadedBySession: Record<string, boolean>;
  /**
   * Per-session generation counter bumped by authoritative active-marker
   * clears (source adoption). A REST hydration started before the bump must
   * not resurrect the marker from a stale snapshot.
   */
  reconcileEpochBySession: Record<string, number>;
  /**
   * Per-session settled-boundary timestamp (RFC3339, compared with nanosecond
   * precision). Set by authoritative boundaries (source adoption,
   * settled-session clears). Any turn that STARTED at or before the boundary
   * must never become active again — a delayed WS `session.turn.started`, a
   * stale hydration, or a force-merged snapshot naming it are all rejected.
   * Turns started after the boundary (genuine resumes) are unaffected.
   */
  settledBoundaryBySession: Record<string, string>;
};

export type TaskSessionsState = {
  items: Record<string, TaskSession>;
  /** Monotonic client event generation used to order live activity against REST refreshes. */
  activityEpochBySession?: Record<string, number>;
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

/** Ordered slot pair for the compare-revisions feature. Either slot may be
 * null. Reducers enforce a 2-slot cap and reject duplicates. */
export type ComparePair = [string | null, string | null];

export type TaskPlansState = {
  byTaskId: Record<string, TaskPlan | null>;
  loadingByTaskId: Record<string, boolean>;
  loadedByTaskId: Record<string, boolean>;
  savingByTaskId: Record<string, boolean>;
  revisionsByTaskId: Record<string, TaskPlanRevision[]>;
  revisionsLoadingByTaskId: Record<string, boolean>;
  revisionsLoadedByTaskId: Record<string, boolean>;
  revisionContentCache: Record<string, string>; // revisionId -> content
  // Phase 6: preview + compare state
  previewRevisionIdByTaskId: Record<string, string | null>;
  comparePairByTaskId: Record<string, ComparePair>;
  // From main: tracks the last `updated_at` the user has seen, so the panel
  // can flag unseen-changes after agent writes between visits.
  lastSeenUpdatedAtByTaskId: Record<string, string>;
};

export type WalkthroughsState = {
  /** The current walkthrough per task (null = explicitly none). */
  byTaskId: Record<string, TaskWalkthrough | null>;
  /** The active step index per task (drives the popover position). */
  activeStepByTaskId: Record<string, number>;
  /** The last `updated_at` the user has opened, for the unseen-dot indicator. */
  lastSeenUpdatedAtByTaskId: Record<string, string>;
};

export type QueuedMessageMetadata = Record<string, unknown> & {
  entity_references?: EntityReference[];
  workflow_message?: boolean;
  workflow_auto_start?: boolean;
  workflow_step_id?: string;
  workflow_step_name?: string;
  workflow_step_color?: string;
  sender_task_id?: string;
  sender_task_title?: string;
  sender_session_id?: string;
  sender_session_name?: string;
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
    data?: string;
    attachment_id?: string;
    mime_type: string;
    name?: string;
    size_bytes?: number;
    delivery_mode?: "prompt" | "path";
  }>;
  metadata?: QueuedMessageMetadata;
  queued_at: string;
  queued_by?: string;
};

/** Capacity info kept alongside the entry list. */
export type QueueMeta = {
  count: number;
  max: number;
  /** Backend-owned queue motion policy. Missing server state defaults to on. */
  autoRun: boolean;
  /** Mirrors the server's message queue merge_enabled setting; hides the
   * "Merge with above" affordance without a separate settings fetch. */
  mergeEnabled: boolean;
};

export type QueueStatus = {
  entries: QueuedMessage[];
  count: number;
  max: number;
  merge_enabled: boolean;
  auto_run?: boolean;
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
  messagePrompts: PromptsState;
  turns: TurnsState;
  taskSessions: TaskSessionsState;
  taskSessionsByTask: TaskSessionsByTaskState;
  sessionAgentctl: SessionAgentctlState;
  worktrees: WorktreesState;
  sessionWorktreesBySessionId: SessionWorktreesState;
  pendingModel: PendingModelState;
  activeModel: ActiveModelState;
  taskPlans: TaskPlansState;
  walkthroughs: WalkthroughsState;
  queue: QueueState;
};

export type SessionSliceActions = {
  setMessages: (
    sessionId: string,
    messages: Message[],
    meta?: {
      historyInitialized?: boolean;
      hasMore?: boolean;
      oldestCursor?: string | null;
    },
  ) => void;
  addMessage: (message: Message) => void;
  updateMessage: (message: Message) => void;
  updateMessages: (messages: Message[]) => void;
  removeMessage: (sessionId: string, messageId: string) => void;
  /**
   * Idempotent full-snapshot merge: reconciles `messages` against the current
   * stored array, preserving object identity for unchanged messages and the
   * array reference itself when nothing changed (see `reconcileMessages`). Used
   * by periodic refetches so a no-op tick triggers zero re-renders.
   */
  mergeMessages: (
    sessionId: string,
    messages: Message[],
    meta?: {
      historyInitialized?: boolean;
      hasMore?: boolean;
      oldestCursor?: string | null;
    },
  ) => void;
  prependMessages: (
    sessionId: string,
    messages: Message[],
    meta?: {
      historyInitialized?: boolean;
      hasMore?: boolean;
      oldestCursor?: string | null;
    },
  ) => void;
  setMessagesMetadata: (
    sessionId: string,
    meta: {
      historyInitialized?: boolean;
      hasMore?: boolean;
      isLoading?: boolean;
      isLoadingMore?: boolean;
      oldestCursor?: string | null;
    },
  ) => void;
  /** Sets the session's message-loading flag. */
  setMessagesLoading: (sessionId: string, loading: boolean) => void;
  replacePromptMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
  prependPromptMessages: (
    sessionId: string,
    messages: Message[],
    meta?: { hasMore?: boolean; oldestCursor?: string | null },
  ) => void;
  setPromptMessagesLoading: (sessionId: string, loading: boolean) => void;
  setPromptMessagesLoadingMore: (sessionId: string, loading: boolean) => void;
  /** Upserts a turn row, rejecting stale updates (see shouldApplyTurnUpdate). */
  addTurn: (turn: Turn) => void;
  /** Merges a complete REST snapshot and reconciles its marker atomically. */
  mergeTurnsSnapshot: (sessionId: string, turns: Turn[], hydrationEpoch: number) => void;
  completeTurn: (
    sessionId: string,
    turnId: string,
    completedAt: string,
    metadata?: Record<string, unknown> | null,
    /** updated_at from the event payload; guards stale re-deliveries. */
    updatedAt?: string,
  ) => void;
  /** Marks a turn as the session's active turn (or null to clear it). */
  setActiveTurn: (sessionId: string, turnId: string | null) => void;
  /**
   * Establishes (or clears) the active-turn marker after a full REST
   * hydration, applying the same settled-session rule as
   * reconcileActiveTurnForIdleSession and rejecting hydrations that started
   * before an authoritative clear (epoch mismatch).
   */
  reconcileActiveTurnAfterHydration: (sessionId: string, hydrationEpoch: number) => void;
  /** Records that the session's full persisted turn history is in the store. */
  markTurnsLoaded: (sessionId: string) => void;
  /**
   * Source adoption is an authoritative idle boundary for the listed
   * sessions. `boundaryTimestamp` MUST be server-issued (the WS envelope
   * timestamp) so the boundary stays on the backend clock — a client-clock
   * fallback would retire legitimate turns when the browser clock runs
   * ahead of the backend. Absent a server timestamp, only the marker clear
   * and epoch bump apply; the server-published adoption event records the
   * boundary on arrival.
   */
  reconcileWorkspaceSourcesAdopted: (sessionIds: string[], boundaryTimestamp?: string) => void;
  setTaskSession: (session: TaskSession) => void;
  /**
   * Narrowly updates only a session's Slack-style read cursor
   * (last_read_message_id) — never the full session object. Used for the
   * mark-read HTTP response, whose full-session snapshot is frozen at
   * write time and can otherwise clobber a newer WS session.state_changed
   * update that arrives while the request is still in flight. No-ops if
   * the session isn't in the store (never creates a bare session record).
   */
  updateSessionReadCursor: (sessionId: string, lastReadMessageId: string) => void;
  setTaskSessionPendingAction: (
    sessionId: string,
    pendingAction: TaskPendingAction | null,
    revision?: TaskPendingActionRevision,
  ) => void;
  removeTaskSession: (taskId: string, sessionId: string) => void;
  setTaskSessionsForTask: (
    taskId: string,
    sessions: TaskSession[],
    activityEpochsAtRequestStart: Readonly<Record<string, number>>,
  ) => void;
  upsertTaskSessionFromEvent: (taskId: string, session: TaskSession) => void;
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
  markTaskPlanSeen: (taskId: string) => void;
  // Revision actions
  setPlanRevisions: (taskId: string, revisions: TaskPlanRevision[]) => void;
  upsertPlanRevision: (taskId: string, revision: TaskPlanRevision) => void;
  setPlanRevisionsLoading: (taskId: string, loading: boolean) => void;
  cachePlanRevisionContent: (revisionId: string, content: string) => void;
  // Phase 6: preview + compare actions
  setPreviewRevision: (taskId: string, revisionId: string | null) => void;
  toggleComparePair: (taskId: string, revisionId: string) => void;
  clearComparePair: (taskId: string) => void;
  // Walkthrough actions
  setWalkthrough: (taskId: string, walkthrough: TaskWalkthrough | null) => void;
  setWalkthroughActiveStep: (taskId: string, stepIndex: number) => void;
  markWalkthroughSeen: (taskId: string) => void;
  // Queue actions
  setQueueEntries: (sessionId: string, entries: QueuedMessage[], meta: QueueMeta) => void;
  removeQueueEntry: (sessionId: string, entryId: string) => void;
  setQueueLoading: (sessionId: string, loading: boolean) => void;
  clearQueueStatus: (sessionId: string) => void;
};

export type SessionSlice = SessionSliceState & SessionSliceActions;
