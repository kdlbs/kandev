import type {
  TaskPreviewFeedback,
  TaskPreviewFeedbackRef,
  TaskPreviewFeedbackSnapshot,
} from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/request-error";

/** Freeze the persisted versions visible when a task composer submits. */
export function toTaskPreviewFeedbackRefs(items: TaskPreviewFeedback[]): TaskPreviewFeedbackRef[] {
  return items.map((item) => ({ id: item.id, version: item.version }));
}

function parseSnapshot(value: unknown): TaskPreviewFeedbackSnapshot | undefined {
  if (
    typeof value !== "object" ||
    value === null ||
    !("task_id" in value) ||
    !("revision" in value) ||
    !("items" in value)
  ) {
    return undefined;
  }
  return value as TaskPreviewFeedbackSnapshot;
}

/** Read the authoritative collection returned with a stale delivery request. */
export function previewFeedbackAdmissionConflict(
  error: unknown,
): TaskPreviewFeedbackSnapshot | undefined {
  if (!(error instanceof WebSocketRequestError) || error.code !== "preview_feedback_changed") {
    return undefined;
  }
  return parseSnapshot(error.details?.snapshot);
}
