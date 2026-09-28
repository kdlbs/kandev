import type { ApiClient } from "../../helpers/api-client";

export type CompletionGateSeed = {
  workflowId: string;
  taskId: string;
  completionStepId: string;
};

type TaskSnapshot = {
  id: string;
  updated_at: string;
  state: string;
};

export async function seedStaleCompletionEvidence(
  apiClient: ApiClient,
  workspaceId: string,
  title: string,
): Promise<CompletionGateSeed> {
  const workflow = await apiClient.createWorkflow(workspaceId, `${title} Workflow`);
  const inbox = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0, {
    is_start_step: true,
  });
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 1, {
    complete_task_on_enter: true,
  });
  const seededTask = await apiClient.seedTask(workspaceId, title, {
    workflow_id: workflow.id,
    workflow_step_id: inbox.id,
  });
  const taskId = seededTask.task_id;
  const taskResponse = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`);
  if (!taskResponse.ok)
    throw new Error(`Could not read completion gate task: ${taskResponse.status}`);
  const task = (await taskResponse.json()) as TaskSnapshot;

  const criteriaResponse = await apiClient.rawRequest(
    "PUT",
    `/api/v1/tasks/${taskId}/completion-gate/criteria`,
    {
      expected_revision: 0,
      criteria: [
        {
          id: "task-review",
          description: "Review the current task revision",
          evidence_subject: { kind: "task_revision", id: taskId },
        },
      ],
    },
  );
  if (!criteriaResponse.ok) {
    throw new Error(`Could not set completion criteria: ${criteriaResponse.status}`);
  }

  const evidenceResponse = await apiClient.rawRequest(
    "PUT",
    `/api/v1/tasks/${taskId}/completion-gate/criteria/task-review/evidence`,
    {
      expected_revision: 1,
      evidence: {
        subject: { kind: "task_revision", id: taskId, revision: task.updated_at },
        summary: "The current task revision passed review.",
      },
    },
  );
  if (!evidenceResponse.ok) {
    throw new Error(`Could not record completion evidence: ${evidenceResponse.status}`);
  }

  return { workflowId: workflow.id, taskId, completionStepId: done.id };
}

export async function makeCompletionEvidenceStale(apiClient: ApiClient, taskId: string) {
  await apiClient.updateTaskTitle(taskId, "Task changed after evidence verification");
  const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`);
  if (!response.ok) throw new Error(`Could not read changed task: ${response.status}`);
  return (await response.json()) as TaskSnapshot;
}

export async function attemptBlockedCompletion(
  apiClient: ApiClient,
  taskId: string,
  workflowId: string,
  completionStepId: string,
) {
  return apiClient.rawRequest("POST", `/api/v1/tasks/${taskId}/move`, {
    workflow_id: workflowId,
    workflow_step_id: completionStepId,
  });
}

export async function readCompletionGate(apiClient: ApiClient, taskId: string) {
  const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}/completion-gate`);
  if (!response.ok) throw new Error(`Could not read completion gate: ${response.status}`);
  return (await response.json()) as {
    revision: number;
    blocked: boolean;
    blockers: Array<{ criterion_id: string; reason: string }>;
  };
}

export async function readTaskState(apiClient: ApiClient, taskId: string) {
  const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`);
  if (!response.ok) throw new Error(`Could not read task state: ${response.status}`);
  return ((await response.json()) as TaskSnapshot).state;
}

export async function readCompletionGateHistory(apiClient: ApiClient, taskId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/tasks/${taskId}/completion-gate/history`,
  );
  if (!response.ok) throw new Error(`Could not read completion history: ${response.status}`);
  return (await response.json()) as {
    items: Array<{ action: string; reason?: string }>;
  };
}
