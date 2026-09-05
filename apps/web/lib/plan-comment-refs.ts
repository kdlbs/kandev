import type { PlanComment } from "@/lib/state/slices/comments";
import type {
  TaskPlanCommentRef,
  TaskPlanCommentSnapshot,
  TaskSessionState,
} from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/request-error";

/** Freeze the task-plan comment versions visible when a user submits. */
export function toTaskPlanCommentRefs(comments: PlanComment[]): TaskPlanCommentRef[] {
  return comments.map((comment) => {
    if (!comment.version || comment.version < 1) {
      // i18n-exempt: internal invariant; legacy rows are migrated before delivery.
      throw new Error("Task plan comment is missing a persisted version");
    }
    return { id: comment.id, version: comment.version };
  });
}

export type PlanCommentAdmissionConflict = {
  code: "plan_comments_changed" | "primary_session_changed";
  snapshot?: TaskPlanCommentSnapshot;
  primarySessionId?: string;
  primarySessionState?: TaskSessionState;
};

/** Read the stable plan-comment conflict fields from a rejected WS request. */
export function planCommentAdmissionConflict(
  error: unknown,
): PlanCommentAdmissionConflict | undefined {
  if (!(error instanceof WebSocketRequestError)) return undefined;
  if (error.code !== "plan_comments_changed" && error.code !== "primary_session_changed") {
    return undefined;
  }
  const candidate = error.details?.snapshot;
  const snapshot =
    typeof candidate === "object" &&
    candidate !== null &&
    "task_id" in candidate &&
    "plan_id" in candidate &&
    "revision" in candidate &&
    "comments" in candidate
      ? (candidate as TaskPlanCommentSnapshot)
      : undefined;
  const primarySessionId =
    typeof error.details?.primary_session_id === "string"
      ? error.details.primary_session_id
      : undefined;
  const primarySessionState =
    typeof error.details?.primary_session_state === "string"
      ? (error.details.primary_session_state as TaskSessionState)
      : undefined;
  return { code: error.code, snapshot, primarySessionId, primarySessionState };
}
