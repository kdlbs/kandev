import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

export type WorkflowAgentOverrideFixture = {
  agentId: string;
  profileA: Awaited<ReturnType<ApiClient["createAgentProfile"]>>;
  profileB: Awaited<ReturnType<ApiClient["createAgentProfile"]>>;
  profileC: Awaited<ReturnType<ApiClient["createAgentProfile"]>>;
  workflow: Awaited<ReturnType<ApiClient["createWorkflow"]>>;
  analysisStep: { id: string };
  implementStep: { id: string };
  reviewStep: { id: string };
  prStep: { id: string };
};

type CreateOverrideTaskOptions = {
  title: string;
  replacementProfileId?: string;
  startAgent?: boolean;
};

export async function seedWorkflowAgentOverrideFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  suffix: string,
): Promise<WorkflowAgentOverrideFixture> {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.id !== "dynamic") ?? agents[0];
  if (!agent) throw new Error("the mock-agent fixture has no launchable agent family");

  const [profileA, profileB, profileC] = await Promise.all([
    apiClient.createAgentProfile(agent.id, `${suffix} Initial A`, { model: "mock-fast" }),
    apiClient.createAgentProfile(agent.id, `${suffix} Replacement B with a long name`, {
      model: "mock-slow",
    }),
    apiClient.createAgentProfile(agent.id, `${suffix} Replacement C with a long name`, {
      model: "mock-fast",
    }),
  ]);
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${suffix} Workflow`);
  const analysisStep = await apiClient.createWorkflowStep(workflow.id, "Analysis", 0, {
    is_start_step: true,
  });
  const implementStep = await apiClient.createWorkflowStep(workflow.id, "Implement", 1, {
    agent_profile_id: profileA.id,
    profile_session_start_policy: "new",
    profile_session_end_policy: "park",
  });
  const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 2, {
    session_target: { kind: "initial" },
    profile_session_end_policy: "park",
  });
  const prStep = await apiClient.createWorkflowStep(workflow.id, "PR", 3, {
    session_target: { kind: "step", step_id: implementStep.id },
    profile_session_end_policy: "park",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  const persistedSteps = await apiClient.listWorkflowSteps(workflow.id);
  const persistedPR = persistedSteps.steps.find((step) => step.id === prStep.id);
  if (persistedPR?.session_target?.kind !== "step") {
    throw new Error(`fixture PR session target was not persisted: ${JSON.stringify(persistedPR)}`);
  }

  return {
    agentId: agent.id,
    profileA,
    profileB,
    profileC,
    workflow,
    analysisStep,
    implementStep,
    reviewStep,
    prStep,
  };
}

export async function createOverrideTask(
  apiClient: ApiClient,
  seedData: SeedData,
  fixture: WorkflowAgentOverrideFixture,
  options: CreateOverrideTaskOptions,
) {
  const { title, replacementProfileId, startAgent = true } = options;
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, fixture.profileA.id, {
    description: `e2e workflow override ${title}`,
    workflow_id: fixture.workflow.id,
    workflow_step_id: fixture.analysisStep.id,
    repository_ids: [seedData.repositoryId],
    executor_profile_id: seedData.worktreeExecutorProfileId,
    workflow_agent_overrides: replacementProfileId
      ? { [fixture.profileA.id]: replacementProfileId }
      : undefined,
    start_agent: startAgent,
  });
}

export async function waitForNewWorkflowProfileSession(
  apiClient: ApiClient,
  taskId: string,
  profileId: string,
  excludedSessionIds: string[] = [],
) {
  let sessionId = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = sessions.find(
          (candidate) =>
            candidate.agent_profile_id === profileId &&
            !excludedSessionIds.includes(candidate.id) &&
            candidate.state === "WAITING_FOR_INPUT",
        );
        sessionId = session?.id ?? "";
        return sessionId;
      },
      { timeout: 45_000, message: `profile ${profileId} did not create a new answerable session` },
    )
    .not.toBe("");
  return sessionId;
}

export async function waitForWorkflowStep(apiClient: ApiClient, taskId: string, stepId: string) {
  await expect
    .poll(() => apiClient.getTask(taskId).then((task) => task.workflow_step_id), {
      timeout: 30_000,
      message: `task ${taskId} did not enter workflow step ${stepId}`,
    })
    .toBe(stepId);
}

export async function waitForWorkflowMoveLifecycle(apiClient: ApiClient, taskId: string) {
  await expect
    .poll(
      async () => {
        const task = await apiClient.getTask(taskId);
        const metadata = task.metadata ?? {};
        return Object.hasOwn(metadata, "manual_move_lifecycle_pending") ? "busy" : "idle";
      },
      { timeout: 30_000, message: `task ${taskId} did not finish its workflow move lifecycle` },
    )
    .toBe("idle");
}

export async function previewWorkflowMove(
  apiClient: ApiClient,
  taskId: string,
  workflowId: string,
  workflowStepId: string,
) {
  const response = await apiClient.rawRequest("POST", `/api/v1/tasks/${taskId}/move-preview`, {
    workflow_id: workflowId,
    workflow_step_id: workflowStepId,
  });
  if (!response.ok) {
    throw new Error(
      `workflow move preview failed with ${response.status}: ${await response.text()}`,
    );
  }
  return response.json() as Promise<{
    outcome: string;
    recipient?: { session_id?: string; profile_id?: string };
    model: {
      before: { known: boolean; label?: string };
      after: { known: boolean; label?: string };
      after_source?: string;
    };
  }>;
}

export async function deleteFixtureTasks(apiClient: ApiClient, taskIds: string[]) {
  await Promise.all(taskIds.map((taskId) => apiClient.deleteTask(taskId).catch(() => undefined)));
}
