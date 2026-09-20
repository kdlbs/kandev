import { expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  createWorkflowAgentProfiles,
  waitForWorkflowProfileSession,
} from "./workflow-agent-switch-helpers";

export type WorkflowSessionFocusScenario = {
  workflow: { id: string };
  source: { id: string; name: string };
  destination: { id: string; name: string };
  task: { id: string };
  sourceSessionId: string;
  secondarySessionId: string;
};

function destinationProfileOptions(target: "new" | "initial" | "profile", profileBId: string) {
  if (target === "new") {
    return {
      session_target: { kind: "initial" as const },
      profile_session_start_policy: "new" as const,
    };
  }
  if (target === "initial") return { session_target: { kind: "initial" as const } };
  return { agent_profile_id: profileBId, profile_session_start_policy: "new" as const };
}

export async function createWorkflowSessionFocusScenario(
  apiClient: ApiClient,
  seedData: SeedData,
  target: "new" | "initial" | "profile",
): Promise<WorkflowSessionFocusScenario> {
  const { profileA, profileB } = await createWorkflowAgentProfiles(apiClient);
  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    `Workflow session focus ${target}`,
  );
  const source = await apiClient.createWorkflowStep(workflow.id, "Source", 0, {
    is_start_step: true,
  });
  const destination = await apiClient.createWorkflowStep(workflow.id, "Destination", 1);

  await apiClient.updateWorkflowStep(source.id, {
    agent_profile_id: profileA.id,
    profile_session_end_policy: "park",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.updateWorkflowStep(destination.id, {
    ...destinationProfileOptions(target, profileB.id),
    prompt: `e2e:message("workflow-focus-${target}")`,
    profile_session_end_policy: "park",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `Workflow session focus ${target} task`,
    profileA.id,
    {
      workflow_id: workflow.id,
      workflow_step_id: source.id,
      repository_ids: [seedData.repositoryId],
    },
  );
  const sourceSessionId = await waitForWorkflowProfileSession(apiClient, task.id, profileA.id);

  const secondary = await apiClient.launchSession({
    task_id: task.id,
    agent_profile_id: profileA.id,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    workflow_step_id: source.id,
    prompt: 'e2e:message("workflow-focus-secondary")',
  });
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((session) => session.id === secondary.session_id)?.state;
      },
      { timeout: 30_000, message: "secondary focus session never became visible" },
    )
    .not.toBe("CREATED");
  await apiClient.setPrimarySession(sourceSessionId);

  return {
    workflow,
    source: { ...source, name: "Source" },
    destination: { ...destination, name: "Destination" },
    task,
    sourceSessionId,
    secondarySessionId: secondary.session_id,
  };
}

export async function waitForNewWorkflowSession(
  apiClient: ApiClient,
  taskId: string,
  excludedSessionIds: readonly string[],
): Promise<string> {
  let sessionId = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        sessionId =
          sessions.find((session) => !excludedSessionIds.includes(session.id) && session.is_primary)
            ?.id ?? "";
        return sessionId;
      },
      { timeout: 30_000, message: "workflow move did not create its destination session" },
    )
    .not.toBe("");
  return sessionId;
}
