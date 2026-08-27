/**
 * Extraction for the reason a task move was refused.
 *
 * Deliberately free of React and of the alert primitives: the banner renders
 * it, but so does the proceed-to-next-step toast in a chat hook, and a plain
 * helper keeps that from dragging component modules into unrelated bundles.
 */
import { ApiError } from "@/lib/api/client";

type Translate = (key: string) => string;

const MOVE_ERROR_TRANSLATIONS: Record<string, string> = {
  task_move_active_session: "task:taskMoveErrorActiveSession",
  task_move_archived: "task:taskMoveErrorArchived",
  task_move_different_workspace: "task:taskMoveErrorDifferentWorkspace",
  task_move_workflow_step: "task:taskMoveErrorWorkflowStep",
  task_move_wip_limit: "task:taskMoveErrorWipLimit",
};

const GENERIC_MOVE_ERROR_TRANSLATION = "task:taskMoveErrorGeneric";

function getTaskMoveErrorCode(error: unknown): string | null {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") return null;
  const code = (error.body as { code?: unknown }).code;
  return typeof code === "string" && code.trim() ? code : null;
}

export function getTaskMoveErrorMessage(
  error: unknown,
  fallback: string,
  translate?: Translate,
): string {
  if (translate && error instanceof ApiError) {
    const code = getTaskMoveErrorCode(error);
    return translate((code && MOVE_ERROR_TRANSLATIONS[code]) || GENERIC_MOVE_ERROR_TRANSLATION);
  }
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error;
  return fallback;
}

/**
 * The detail worth rendering beneath the headline, or null when extraction fell
 * back to the headline itself. Repeating the same sentence twice reads as a
 * rendering bug and tells the user nothing the headline has not already said.
 */
export function getTaskMoveErrorDetail(
  error: unknown,
  title: string,
  translate?: Translate,
): string | null {
  const detail = getTaskMoveErrorMessage(error, title, translate);
  return detail === title ? null : detail;
}
