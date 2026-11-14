import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import {
  createWorkflowAgentProfiles,
  waitForWorkflowProfileSession,
} from "../workflow/workflow-agent-switch-helpers";

export async function createUnavailableExecutorMoveFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const { profileA, profileB } = await createWorkflowAgentProfiles(
    apiClient,
    seedData.agentProfileId,
  );
  let workflowId = "";
  let taskId = "";
  let executorProfileId = "";
  try {
    const { executors } = await apiClient.listExecutors();
    const worktreeExecutor = executors.find(
      (executor) =>
        executor.type === "worktree" &&
        executor.profiles?.some((profile) => profile.id === seedData.worktreeExecutorProfileId),
    );
    if (!worktreeExecutor) throw new Error("the seeded worktree executor is unavailable");

    const executorProfile = await apiClient.createExecutorProfile(
      worktreeExecutor.id,
      `Unavailable executor ${Date.now()}`,
    );
    executorProfileId = executorProfile.id;

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${title} workflow`);
    workflowId = workflow.id;
    const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Plan", 0, {
      is_start_step: true,
    });
    const targetStep = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    await apiClient.updateWorkflowStep(sourceStep.id, {
      agent_profile_id: profileA.id,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(targetStep.id, {
      agent_profile_id: profileB.id,
      profile_session_start_policy: "new",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profileA.id, {
      description: "/e2e:simple-message",
      workflow_id: workflow.id,
      workflow_step_id: sourceStep.id,
      repository_ids: [seedData.repositoryId],
      executor_id: worktreeExecutor.id,
      executor_profile_id: executorProfile.id,
    });
    taskId = task.id;
    await waitForWorkflowProfileSession(apiClient, task.id, profileA.id);
    await apiClient.deleteExecutorProfile(executorProfile.id);

    return {
      workflowId: workflow.id,
      sourceStep,
      targetStep,
      task,
      deletedExecutorProfileId: executorProfile.id,
      async cleanup() {
        await apiClient.deleteTask(task.id).catch(() => undefined);
        await apiClient.deleteWorkflow(workflow.id).catch(() => undefined);
        await apiClient.deleteExecutorProfile(executorProfile.id).catch(() => undefined);
        await apiClient.deleteAgentProfile(profileA.id).catch(() => undefined);
        await apiClient.deleteAgentProfile(profileB.id).catch(() => undefined);
      },
    };
  } catch (error) {
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    if (workflowId) await apiClient.deleteWorkflow(workflowId).catch(() => undefined);
    if (executorProfileId)
      await apiClient.deleteExecutorProfile(executorProfileId).catch(() => undefined);
    await apiClient.deleteAgentProfile(profileA.id).catch(() => undefined);
    await apiClient.deleteAgentProfile(profileB.id).catch(() => undefined);
    throw error;
  }
}
