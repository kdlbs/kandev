import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type {
  TaskPendingAction,
  TaskSessionState,
  TaskState,
  WorkflowReviewStatus,
} from "@/lib/types/http";

type KanbanTask = KanbanState["tasks"][number];

/**
 * One live agent conversation, projected into the shape a Threads column needs.
 *
 * Derived entirely from the workflow snapshots the board already keeps in the
 * store, so opening the Threads view costs no extra request.
 */
export type ActiveThread = {
  taskId: string;
  title: string;
  workflowId: string;
  workflowName: string;
  /** Null when the task sits in a step the snapshot no longer lists. */
  stepTitle: string | null;
  sessionId: string;
  sessionState: TaskSessionState;
  pendingAction: TaskPendingAction | null;
  /** Aggregate action across the task, never attributed to `sessionId`. */
  taskPendingAction?: TaskPendingAction | null;
  taskState?: TaskState | null;
  reviewStatus?: WorkflowReviewStatus | null;
  activeSubagentCount: number;
  queuedPromptCount: number;
  lastActivityAt: string | null;
};

/**
 * Attention buckets. A thread blocked on a person is the one the user came to
 * this view to find, so it sorts ahead of threads the agent is still driving.
 */
const NEEDS_HUMAN = 0;
const WORKING = 1;
const WAITING = 2;

const WORKING_STATES: TaskSessionState[] = ["RUNNING", "STARTING"];

type ThreadSession = {
  id: string | null;
  state: TaskSessionState | null;
  pendingAction: TaskPendingAction | null;
  lastActivityAt: string | null;
};

function resolvePendingAction(task: KanbanTask): TaskPendingAction | null {
  // The bounded status summary and taskPendingAction are aggregates across
  // every session. Threads renders the primary session only, so using either
  // aggregate here can attribute a child session's request to the wrong
  // conversation.
  return task.primarySessionPendingAction ?? null;
}

export function resolveTaskPendingAction(task: KanbanTask): TaskPendingAction | null {
  const summaryAction = task.statusSummary?.pending_action;
  return summaryAction !== undefined ? summaryAction : (task.taskPendingAction ?? null);
}

function isReviewOutcome(task: KanbanTask): boolean {
  return task.state === "REVIEW" || task.reviewStatus === "pending";
}

function resolveThreadBucket(
  task: KanbanTask,
  session: ThreadSession,
  taskPendingAction: TaskPendingAction | null,
): number | null {
  const sessionBucket = attentionBucket(session);
  if (sessionBucket !== null) return sessionBucket;
  if (taskPendingAction) return NEEDS_HUMAN;
  if (isReviewOutcome(task)) return WORKING;
  return null;
}

function resolveLastActivityAt(task: KanbanTask): string | null {
  const summary = task.statusSummary;
  return summary?.last_activity_at ?? summary?.updated_at ?? task.updatedAt ?? null;
}

/**
 * The status summary is the bounded live projection the backend pushes over
 * WebSockets, so it leads; the cached primary-session fields only fill gaps a
 * summary-less task would otherwise leave blank.
 */
function resolveThreadSession(task: KanbanTask): ThreadSession {
  const summarySession = task.statusSummary?.primary_session ?? null;
  return {
    id: summarySession?.id ?? task.primarySessionId ?? null,
    state: summarySession?.state ?? (task.primarySessionState as TaskSessionState | null) ?? null,
    pendingAction: resolvePendingAction(task),
    lastActivityAt: resolveLastActivityAt(task),
  };
}

function attentionBucket(session: {
  state: TaskSessionState | null | undefined;
  pendingAction?: TaskPendingAction | null;
}): number | null {
  if (session.pendingAction) return NEEDS_HUMAN;
  if (session.state && WORKING_STATES.includes(session.state)) return WORKING;
  return session.state === "WAITING_FOR_INPUT" ? WAITING : null;
}

export type ThreadTaskEligibilityInput = {
  taskState?: TaskState | null;
  reviewStatus?: WorkflowReviewStatus | null;
  taskPendingAction?: TaskPendingAction | null;
  primarySession?: {
    state: TaskSessionState | null | undefined;
    pendingAction?: TaskPendingAction | null;
  } | null;
};

/**
 * Whether a task has a column in the Threads deck, independent of which
 * conversation a task-detail page currently has selected.
 */
export function isThreadTaskEligible({
  taskState,
  reviewStatus,
  taskPendingAction,
  primarySession,
}: ThreadTaskEligibilityInput): boolean {
  if (taskPendingAction || taskState === "REVIEW" || reviewStatus === "pending") return true;
  return primarySession
    ? isActiveThreadSession({
        isPrimary: true,
        state: primarySession.state,
        pendingAction: primarySession.pendingAction,
      })
    : false;
}

/**
 * Whether one session is the thread the deck would give a column to.
 *
 * The deck keys columns by task and renders the task's primary session, so a
 * non-primary session has no column of its own however busy it is. Surfaces
 * that offer to jump into the deck ask this first, so they never point at a
 * column that is not there.
 */
export function isActiveThreadSession(session: {
  isPrimary: boolean;
  state: TaskSessionState | null | undefined;
  pendingAction?: TaskPendingAction | null;
}): boolean {
  if (!session.isPrimary) return false;
  return attentionBucket(session) !== null;
}

/**
 * The column a deep link asked to focus, or null when that task is not in the
 * deck — it may have settled between the link being offered and followed.
 */
export function resolveFocusedThreadId(
  threads: readonly ActiveThread[],
  requestedTaskId: string | null | undefined,
): string | null {
  if (!requestedTaskId) return null;
  return threads.some((thread) => thread.taskId === requestedTaskId) ? requestedTaskId : null;
}

type ThreadBuildInput = {
  task: KanbanTask;
  snapshot: WorkflowSnapshotData;
  stepTitles: Map<string, string>;
  session: ThreadSession;
  taskPendingAction: TaskPendingAction | null;
  bucket: number;
};

function buildThread({
  task,
  snapshot,
  stepTitles,
  session,
  taskPendingAction,
  bucket,
}: ThreadBuildInput): ActiveThread & { bucket: number } {
  return {
    bucket,
    taskId: task.id,
    title: task.title,
    workflowId: snapshot.workflowId,
    workflowName: snapshot.workflowName,
    stepTitle: stepTitles.get(task.workflowStepId) ?? null,
    sessionId: session.id as string,
    sessionState: session.state as TaskSessionState,
    pendingAction: session.pendingAction,
    taskPendingAction,
    taskState: task.state ?? null,
    reviewStatus: task.reviewStatus ?? null,
    activeSubagentCount: task.statusSummary?.active_subagent_count ?? task.activeSubagentCount ?? 0,
    queuedPromptCount: task.statusSummary?.queued_prompt_count ?? 0,
    lastActivityAt: session.lastActivityAt,
  };
}

function toThread(
  task: KanbanTask,
  snapshot: WorkflowSnapshotData,
  stepTitles: Map<string, string>,
): (ActiveThread & { bucket: number }) | null {
  if (task.isArchived) return null;
  const session = resolveThreadSession(task);
  const taskPendingAction = resolveTaskPendingAction(task);
  if (
    !isThreadTaskEligible({
      taskState: task.state ?? null,
      reviewStatus: task.reviewStatus ?? null,
      taskPendingAction,
      primarySession: session,
    })
  ) {
    return null;
  }
  const bucket = resolveThreadBucket(task, session, taskPendingAction);
  // A thread with no session id has no conversation to render, so a column for
  // it would be an empty promise rather than a view of live work.
  if (bucket === null || !session.id || !session.state) return null;
  return buildThread({ task, snapshot, stepTitles, session, taskPendingAction, bucket });
}

function activityRank(thread: ActiveThread): number {
  const parsed = thread.lastActivityAt ? Date.parse(thread.lastActivityAt) : Number.NaN;
  return Number.isNaN(parsed) ? 0 : parsed;
}

/**
 * Columns must not reshuffle under the reader between renders, so ordering is
 * total: bucket, then recency, then a task-id tiebreak.
 */
function compareThreads(
  a: ActiveThread & { bucket: number },
  b: ActiveThread & { bucket: number },
): number {
  if (a.bucket !== b.bucket) return a.bucket - b.bucket;
  const recency = activityRank(b) - activityRank(a);
  if (recency !== 0) return recency;
  return a.taskId.localeCompare(b.taskId);
}

export function selectActiveThreads(
  snapshots: Record<string, WorkflowSnapshotData>,
  options: { workflowId?: string | null } = {},
): ActiveThread[] {
  const scoped = Object.values(snapshots).filter(
    (snapshot) => !options.workflowId || snapshot.workflowId === options.workflowId,
  );

  const threads = scoped.flatMap((snapshot) => {
    const stepTitles = new Map(snapshot.steps.map((step) => [step.id, step.title]));
    return snapshot.tasks
      .map((task) => toThread(task, snapshot, stepTitles))
      .filter((thread): thread is ActiveThread & { bucket: number } => thread !== null);
  });

  return threads.sort(compareThreads).map(({ bucket: _bucket, ...thread }) => thread);
}
