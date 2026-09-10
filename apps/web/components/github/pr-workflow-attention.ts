import type { TaskPR, WorkflowAttention, WorkflowAttentionState } from "@/lib/types/github";

type WorkflowAttentionCarrier = {
  state: string;
  head_sha?: string | null;
  workflow_attention?: WorkflowAttention | null;
};

function isAttentionState(state: WorkflowAttentionState): boolean {
  return state === "approval_required" || state === "action_required";
}

function currentWorkflowAttention(
  carrier: WorkflowAttentionCarrier,
  attention: WorkflowAttention | null | undefined,
): WorkflowAttention | null {
  if (
    carrier.state !== "open" ||
    !carrier.head_sha ||
    !attention ||
    attention.head_sha !== carrier.head_sha
  ) {
    return null;
  }
  return attention;
}

function normalizeWorkflowAttention(attention: WorkflowAttention | null): WorkflowAttention | null {
  if (!attention || attention.state === "none") return null;
  return { ...attention, runs: attention.runs ?? [] };
}

export function getCurrentWorkflowAttention(
  carrier: WorkflowAttentionCarrier,
): WorkflowAttention | null {
  return currentWorkflowAttention(carrier, carrier.workflow_attention);
}

/**
 * Chooses the best observation for a surface that has both stored TaskPR data
 * and an optional live feedback response. A failed live read must not hide a
 * stored same-head positive observation, while an authoritative empty result
 * must clear it.
 */
export function getWorkflowAttentionForDisplay(
  carrier: WorkflowAttentionCarrier,
  suppliedAttention?: WorkflowAttention | null,
): WorkflowAttention | null {
  const stored = getCurrentWorkflowAttention(carrier);
  if (suppliedAttention == null) return normalizeWorkflowAttention(stored);
  if (
    carrier.head_sha &&
    suppliedAttention.head_sha &&
    suppliedAttention.head_sha !== carrier.head_sha
  ) {
    return null;
  }
  const incoming = currentWorkflowAttention(carrier, suppliedAttention);
  if (!incoming) return null;
  if (incoming.state === "unknown" && stored && isAttentionState(stored.state)) {
    return normalizeWorkflowAttention({ ...stored, stale: true });
  }
  return normalizeWorkflowAttention(incoming);
}

export function getActiveWorkflowAttention(
  carrier: WorkflowAttentionCarrier,
  attention: WorkflowAttention | null | undefined = carrier.workflow_attention,
): WorkflowAttention | null {
  const current = currentWorkflowAttention(carrier, attention);
  return current && isAttentionState(current.state) ? current : null;
}

export function getTaskPRWorkflowAttention(pr: TaskPR): WorkflowAttention | null {
  return getActiveWorkflowAttention(pr);
}

export function workflowAttentionWorkflowNames(attention: WorkflowAttention): string {
  const names = attention.runs.map((run) => run.name.trim()).filter(Boolean);
  return names.join(", ");
}

export function isWorkflowApprovalRequired(attention: WorkflowAttention): boolean {
  return attention.state === "approval_required";
}

export function workflowAttentionReasonKey(reason: string): string {
  return reason === "approval_required"
    ? "github:workflowApprovalRequired"
    : "github:workflowNeedsAttention";
}
