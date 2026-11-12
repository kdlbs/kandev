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
import { getTaskMoveErrorMessage } from "@/components/task/task-move-error-message";
import type { OfficeTaskStatus } from "@/lib/state/slices/office/types";
import type { TaskStatus } from "@/app/office/tasks/[id]/types";

export type PendingApprover = { agent_profile_id?: string; name?: string };

type ApprovalGateReason = "approvals" | "workflow_step" | "completion_gate" | "workflow_move";

const GATE_REASONS: ReadonlySet<string> = new Set([
  "approvals",
  "workflow_step",
  "completion_gate",
  "workflow_move",
]);

/** What the backend did with a status change that it accepted. */
export type StatusUpdateOutcome = {
  status: OfficeTaskStatus | TaskStatus;
  workflowStepName?: string;
  /** The move is waiting for room under the final step's WIP limit. */
  queued?: boolean;
};

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

function extractRedirectedStatus(err: unknown): (OfficeTaskStatus | TaskStatus) | null {
  if (!(err instanceof ApiError)) return null;
  const body = err.body;
  if (!body || typeof body !== "object") return null;
  const status = (body as { status?: unknown }).status;
  return typeof status === "string" && status.length > 0
    ? (status as OfficeTaskStatus | TaskStatus)
    : null;
}

function extractGateReason(err: unknown): ApprovalGateReason | null {
  if (!(err instanceof ApiError)) return null;
  const body = err.body;
  if (!body || typeof body !== "object") return null;
  const reason = (body as { reason?: unknown }).reason;
  return typeof reason === "string" && GATE_REASONS.has(reason)
    ? (reason as ApprovalGateReason)
    : null;
}

// Thrown instead of a plain Error when the approver gate fires. The backend
// does not reject a gated "done" move: it redirects the task to in_review,
// persists and broadcasts that, and only then returns 409 (see
// respondStatusUpdateError in the backend handler). A caller that rolls back
// to its pre-mutation snapshot on any thrown error would show a status the
// server no longer holds, so this carries the server's actual redirected
// status through instead of discarding it.
export class ApprovalGateError extends Error {
  readonly redirectedStatus: OfficeTaskStatus | TaskStatus;

  constructor(message: string, redirectedStatus: OfficeTaskStatus | TaskStatus) {
    super(message);
    this.name = "ApprovalGateError";
    this.redirectedStatus = redirectedStatus;
  }
}

// Thrown when the backend refused a status change without changing anything,
// such as a Done request whose move to the final workflow step was refused.
// Callers roll back to their pre-mutation snapshot.
export class StatusChangeRefusedError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "StatusChangeRefusedError";
  }
}

function refusalMessage(err: ApiError, reason: ApprovalGateReason): string {
  if (reason === "approvals") {
    return formatPendingApproversMessage(extractPendingApprovers(err) ?? []);
  }
  if (reason === "completion_gate") return t("task:cannotMarkDoneCompletionCriteria");
  const code = (err.body as { code?: unknown }).code;
  return typeof code === "string" && code
    ? getTaskMoveErrorMessage(err, "", (key) => t(key))
    : t("task:cannotMarkDoneMoveRefused");
}

function toOutcome(
  requested: OfficeTaskStatus | TaskStatus,
  response: { status?: string; queued?: boolean; workflow_step?: { name?: string } } | undefined,
): StatusUpdateOutcome {
  const outcome: StatusUpdateOutcome = {
    status: (response?.status as OfficeTaskStatus | TaskStatus | undefined) || requested,
  };
  const stepName = response?.workflow_step?.name;
  if (stepName) outcome.workflowStepName = stepName;
  if (response?.queued) outcome.queued = true;
  return outcome;
}

// The info message for a Done request that moved the task to the final step
// without completing it (or queued it behind the step's WIP limit), or null
// when the task reached the requested status.
export function finalStepMoveNotice(
  requested: OfficeTaskStatus | TaskStatus,
  outcome: StatusUpdateOutcome,
): string | null {
  if (outcome.status === requested || !outcome.workflowStepName) return null;
  if (outcome.queued) {
    return t("task:movedToFinalStepQueued", { step: outcome.workflowStepName });
  }
  return t("task:movedToFinalStepNotCompleted", { step: outcome.workflowStepName });
}

// Sets a task's status and returns what the backend did with it. A gate that
// redirected the task is re-thrown as an ApprovalGateError so every caller
// surfaces the same sentence and settles on the status the server actually
// holds; a refusal that changed nothing is re-thrown as a
// StatusChangeRefusedError so callers roll back.
export async function updateTaskStatusOrTranslateGate(
  taskId: string,
  status: OfficeTaskStatus | TaskStatus,
): Promise<StatusUpdateOutcome> {
  try {
    return toOutcome(status, await updateTask(taskId, { status }));
  } catch (err) {
    const reason = extractGateReason(err);
    if (err instanceof ApiError && err.status === 409 && reason && !extractRedirectedStatus(err)) {
      throw new StatusChangeRefusedError(refusalMessage(err, reason));
    }
    if (reason === "workflow_step") {
      const redirectedStatus = extractRedirectedStatus(err) ?? "in_review";
      throw new ApprovalGateError(
        t("task:cannotMarkDoneBeforeFinalWorkflowStep"),
        redirectedStatus,
      );
    }
    const pending = extractPendingApprovers(err);
    if (pending) {
      // The gate always redirects to in_review today (see applyApprovalGate
      // in the backend); reading it from the response body rather than
      // hardcoding it here keeps this in sync if the backend ever changes.
      const redirectedStatus = extractRedirectedStatus(err) ?? "in_review";
      throw new ApprovalGateError(formatPendingApproversMessage(pending), redirectedStatus);
    }
    throw err;
  }
}
