import { t } from "@/lib/i18n";
import { workflowMessageInfoFromMetadata } from "@/components/task/chat/messages/workflow-step-message-badge";
import type { SenderTaskInfo } from "@/hooks/domains/session/use-sender-task-badge-model";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import type { WorkflowStepMessageInfo } from "@/components/task/chat/messages/workflow-step-message-badge";

export type SenderKind = "user" | "agent" | "workflow" | "system";

export function isWorkflowQueuedMessage(entry: QueuedMessage): boolean {
  return (
    entry.queued_by === "workflow" ||
    entry.queued_by === "workflow-auto-start" ||
    workflowMessageInfoFromMetadata(entry.metadata) !== null
  );
}

export function senderKindOf(entry: QueuedMessage): SenderKind {
  if (isWorkflowQueuedMessage(entry)) return "workflow";
  if (!entry.queued_by) return "system";
  if (entry.queued_by === "agent") return "agent";
  return "user";
}

/** A message may serve as a merge source or target only when its sender kind is
 * "user" or "agent". Server-owned rows (queued_by="server") are reserved
 * backend rows and are never mergeable. */
export function canMergeEntry(entry: QueuedMessage): boolean {
  if (entry.queued_by === "server") return false;
  if (
    entry.metadata &&
    ("plan_comment_refs" in entry.metadata ||
      "plan_comment_request_fingerprint" in entry.metadata ||
      "client_queue_id" in entry.metadata)
  ) {
    return false;
  }
  const kind = senderKindOf(entry);
  return kind === "user" || kind === "agent";
}

/** Two messages may merge when both are mergeable and share the same sender
 * kind. Agent entries additionally require an identical non-empty
 * sender_task_id; user entries require both rows to be owned by the caller
 * identity (callerId defaults to the backend's QueuedByUser, which is what the
 * SPA sends when it omits user_id). Mirrors the backend's mergeAllowed
 * predicate so the button never appears for merges the server would reject. */
export function canMergeWithAbove(
  entry: QueuedMessage,
  above: QueuedMessage | undefined,
  callerId = "user",
): boolean {
  if (!above) return false;
  if (!canMergeEntry(entry) || !canMergeEntry(above)) return false;
  if (senderKindOf(entry) !== senderKindOf(above)) return false;
  if (senderKindOf(entry) === "agent") {
    const senderTaskId = getSenderTaskInfo(entry)?.id;
    return senderTaskId !== undefined && senderTaskId === getSenderTaskInfo(above)?.id;
  }
  if (senderKindOf(entry) === "user") {
    return callerId !== "" && entry.queued_by === callerId && above.queued_by === callerId;
  }
  return false;
}

export function senderLabel(entry: QueuedMessage): string {
  const kind = senderKindOf(entry);
  if (kind === "agent") {
    const title = entry.metadata?.sender_task_title;
    const sessionName = entry.metadata?.sender_session_name;
    const base =
      typeof title === "string" && title.length > 0
        ? t("task:fromTask", { title })
        : t("task:fromAgent");
    return typeof sessionName === "string" && sessionName.length > 0
      ? t("task:senderWithSession", { base, sessionName })
      : base;
  }
  if (kind === "workflow") return t("task:workflowSender");
  if (kind === "system") return t("task:systemSender");
  return t("task:you");
}

export function getSenderTaskInfo(entry: QueuedMessage): SenderTaskInfo | null {
  if (senderKindOf(entry) !== "agent") return null;
  const meta = entry.metadata as
    | {
        sender_task_id?: string;
        sender_task_title?: string;
        sender_session_id?: string;
        sender_session_name?: string;
      }
    | undefined;
  const id = meta?.sender_task_id;
  if (typeof id !== "string" || id.length === 0) return null;
  return {
    id,
    snapshotTitle: meta?.sender_task_title ?? "",
    sessionId: meta?.sender_session_id,
    sessionName: meta?.sender_session_name,
  };
}

export function getWorkflowMessageInfo(entry: QueuedMessage): WorkflowStepMessageInfo | null {
  const info = workflowMessageInfoFromMetadata(entry.metadata);
  if (info) return info;
  return entry.queued_by === "workflow" || entry.queued_by === "workflow-auto-start" ? {} : null;
}
