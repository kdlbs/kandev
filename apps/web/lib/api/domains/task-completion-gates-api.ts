import { fetchJson, type ApiRequestOptions } from "../client";

export type TaskCompletionEvidenceSubject = {
  kind: "task_revision" | "execution" | "artifact_revision" | "github_pr_head";
  id: string;
  revision?: string;
};

export type TaskCompletionEvidence = {
  subject: TaskCompletionEvidenceSubject;
  summary?: string;
  reference?: string;
};

export type TaskCompletionCriterion = {
  id: string;
  description: string;
  evidence_subject: TaskCompletionEvidenceSubject;
  criterion_revision: number;
  verified_revision?: number;
  evidence?: TaskCompletionEvidence | null;
  verifier_kind?: string;
  verifier_id?: string;
  verified_at?: string | null;
};

export type TaskCompletionBlocker = {
  criterion_id: string;
  reason: string;
};

export type TaskCompletionGateSnapshot = {
  task_id: string;
  workspace_id: string;
  revision: number;
  criteria: TaskCompletionCriterion[];
  blockers?: TaskCompletionBlocker[];
  blocked: boolean;
};

export type TaskCompletionGateHistory = {
  id: string;
  task_id: string;
  workspace_id: string;
  revision: number;
  action: string;
  actor_kind: string;
  actor_id: string;
  reason?: string;
  details?: string;
  created_at: string;
};

export type TaskCompletionCriterionInput = {
  id: string;
  description: string;
  evidence_subject: Pick<TaskCompletionEvidenceSubject, "kind" | "id">;
};

export type SetTaskCompletionCriteriaRequest = {
  expected_revision: number;
  criteria: TaskCompletionCriterionInput[];
  human_confirmation?: { expected_revision: number; reason: string };
};

export type VerifyTaskCompletionCriterionRequest = {
  expected_revision: number;
  evidence: TaskCompletionEvidence;
};

export function getTaskCompletionGate(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskCompletionGateSnapshot>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/completion-gate`,
    options,
  );
}

export async function listTaskCompletionGateHistory(taskId: string, options?: ApiRequestOptions) {
  const response = await fetchJson<{ items: TaskCompletionGateHistory[] }>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/completion-gate/history`,
    options,
  );
  return response.items ?? [];
}

export function setTaskCompletionCriteria(
  taskId: string,
  request: SetTaskCompletionCriteriaRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskCompletionGateSnapshot>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/completion-gate/criteria`,
    {
      ...options,
      init: { method: "PUT", body: JSON.stringify(request), ...(options?.init ?? {}) },
    },
  );
}

export function verifyTaskCompletionCriterion(
  taskId: string,
  criterionId: string,
  request: VerifyTaskCompletionCriterionRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskCompletionGateSnapshot>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/completion-gate/criteria/${encodeURIComponent(criterionId)}/evidence`,
    {
      ...options,
      init: { method: "PUT", body: JSON.stringify(request), ...(options?.init ?? {}) },
    },
  );
}
