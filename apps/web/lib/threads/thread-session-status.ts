import type {
  ForegroundActivity,
  TaskPendingAction,
  TaskSession,
  TaskState,
  WorkflowReviewStatus,
} from "@/lib/types/http";

export type ThreadSessionStatusKind =
  | "permission"
  | "clarification"
  | "starting"
  | "working"
  | "failed"
  | "cancelled"
  | "finished"
  | "waiting"
  | "completed"
  | "created";

export type ThreadColumnStatusKind = ThreadSessionStatusKind | "needs-you" | "review-ready";

export type ThreadStatus = {
  kind: ThreadColumnStatusKind;
  labelKey: string;
  hasAttention: boolean;
};

type ThreadSessionStatusInput = Pick<TaskSession, "state"> & {
  pending_action?: TaskPendingAction | null;
  foreground_activity?: ForegroundActivity | null;
};

const STATUS = {
  permission: {
    kind: "permission",
    labelKey: "threads:statusPermissionNeeded",
    hasAttention: true,
  },
  clarification: {
    kind: "clarification",
    labelKey: "threads:statusQuestionFromAgent",
    hasAttention: true,
  },
  starting: { kind: "starting", labelKey: "threads:statusStarting", hasAttention: false },
  working: { kind: "working", labelKey: "threads:statusWorking", hasAttention: false },
  failed: { kind: "failed", labelKey: "threads:statusFailed", hasAttention: false },
  cancelled: { kind: "cancelled", labelKey: "threads:statusCancelled", hasAttention: false },
  finished: { kind: "finished", labelKey: "threads:statusTurnFinished", hasAttention: false },
  waiting: { kind: "waiting", labelKey: "threads:statusWaiting", hasAttention: false },
  completed: { kind: "completed", labelKey: "threads:statusCompleted", hasAttention: false },
  created: { kind: "created", labelKey: "threads:statusNotStarted", hasAttention: false },
} satisfies Record<ThreadSessionStatusKind, ThreadStatus>;

const REVIEW_READY: ThreadStatus = {
  kind: "review-ready",
  labelKey: "threads:statusReadyForReview",
  hasAttention: false,
};

const NEEDS_YOU: ThreadStatus = {
  kind: "needs-you",
  labelKey: "threads:statusNeedsYou",
  hasAttention: true,
};

export function resolveThreadSessionStatus(session: ThreadSessionStatusInput): ThreadStatus {
  if (session.pending_action === "permission") return STATUS.permission;
  if (session.pending_action === "clarification") return STATUS.clarification;
  if (session.state === "STARTING") return STATUS.starting;
  if (session.state === "RUNNING" || session.foreground_activity) return STATUS.working;
  if (session.state === "FAILED") return STATUS.failed;
  if (session.state === "CANCELLED") return STATUS.cancelled;
  if (session.state === "IDLE") return STATUS.finished;
  if (session.state === "WAITING_FOR_INPUT") return STATUS.waiting;
  if (session.state === "COMPLETED") return STATUS.completed;
  return STATUS.created;
}

export function resolveThreadColumnStatus({
  taskState,
  reviewStatus,
  taskPendingAction,
  session,
}: {
  taskState?: TaskState | null;
  reviewStatus?: WorkflowReviewStatus | null;
  taskPendingAction?: TaskPendingAction | null;
  session?: ThreadSessionStatusInput | null;
}): ThreadStatus {
  const sessionStatus = session ? resolveThreadSessionStatus(session) : null;
  if (sessionStatus?.hasAttention) return sessionStatus;
  if (taskPendingAction) return NEEDS_YOU;
  if (sessionStatus?.kind === "starting" || sessionStatus?.kind === "working") {
    return sessionStatus;
  }
  if (taskState === "REVIEW" || reviewStatus === "pending") return REVIEW_READY;
  return sessionStatus ?? STATUS.created;
}
