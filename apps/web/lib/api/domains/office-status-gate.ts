// Shared status mutation for office tasks, plus the translation of the one
// gate it can trip.
//
// The backend refuses an `in_review -> done` transition with 409 and a
// `pending_approvers` body whenever a reviewer has not signed off. A caller
// that does not translate that shows a bare "Update failed" and the user has
// no idea who they are waiting on. This lived inside status-picker.tsx while
// the picker was the only way to change a status; the task board's drag
// handler is the second, so it moved here rather than being copied.
import { ApiError } from "../client";
import { updateTask } from "./office-extended-api";
import { t } from "@/lib/i18n";

export type PendingApprover = { agent_profile_id?: string; name?: string };

// Builds the message the user sees when the approver gate rejects the move.
// Names render in the order the backend echoed them.
export function formatPendingApproversMessage(pending: PendingApprover[]): string {
  const names = pending.map((p) => p.name?.trim() || p.agent_profile_id || "").filter(Boolean);
  if (names.length === 0) return t("task:cannotMarkDoneAwaitingApprovals");
  return t("task:cannotMarkDoneAwaitingApprovalFrom", { names: names.join(", ") });
}

export function extractPendingApprovers(err: unknown): PendingApprover[] | null {
  if (!(err instanceof ApiError)) return null;
  if (err.status !== 409) return null;
  const body = err.body;
  if (!body || typeof body !== "object") return null;
  const pending = (body as { pending_approvers?: unknown }).pending_approvers;
  if (!Array.isArray(pending)) return null;
  return pending.filter((p): p is PendingApprover => !!p && typeof p === "object");
}

// Sets a task's status, re-throwing the approver gate as a readable Error so
// every caller surfaces the same sentence.
export async function updateTaskStatusOrTranslateGate(
  taskId: string,
  status: string,
): Promise<void> {
  try {
    await updateTask(taskId, { status });
  } catch (err) {
    const pending = extractPendingApprovers(err);
    if (pending) {
      throw new Error(formatPendingApproversMessage(pending));
    }
    throw err;
  }
}
