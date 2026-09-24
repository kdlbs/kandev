import { expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { TaskStatusSummaryLaunchQueue } from "../../../lib/types/task-status-summary";
import {
  createWorkflowAgentProfiles,
  waitForWorkflowProfileSession,
} from "./workflow-agent-switch-helpers";

export type QueuedSessionOwnershipScenario = {
  taskId: string;
  taskTitle: string;
  workflowId: string;
  sourceSessionId: string;
  destinationSessionId: string;
  fillerSessionId: string;
  queue: TaskStatusSummaryLaunchQueue;
  destinationProfileName: string;
};

export const QUEUED_DESTINATION_MARKER = "queued destination delivery";

export async function waitForLaunchQueue(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
  timeoutMs = 30_000,
): Promise<TaskStatusSummaryLaunchQueue> {
  let latest: TaskStatusSummaryLaunchQueue | null | undefined;
  await expect
    .poll(
      async () => {
        const { tasks } = await apiClient.listTasks(workspaceId);
        latest = tasks.find((task) => task.id === taskId)?.status_summary?.launch_queue;
        return latest ?? null;
      },
      { timeout: timeoutMs, message: `task ${taskId} never exposed its launch queue` },
    )
    .not.toBeNull();
  if (!latest) throw new Error(`task ${taskId} exposed no launch queue after polling`);
  return latest;
}

export async function waitForLaunchQueueCleared(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
  timeoutMs = 60_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { tasks } = await apiClient.listTasks(workspaceId);
        return tasks.find((task) => task.id === taskId)?.status_summary?.launch_queue ?? null;
      },
      { timeout: timeoutMs, message: `task ${taskId} launch queue did not clear` },
    )
    .toBeNull();
}

export async function waitForSessionState(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  expectedState: string,
  timeoutMs = 30_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions.find((session) => session.id === sessionId)?.state ?? "";
      },
      {
        timeout: timeoutMs,
        message: `session ${sessionId} did not become ${expectedState}`,
      },
    )
    .toBe(expectedState);
}

export async function waitForTaskState(
  apiClient: ApiClient,
  taskId: string,
  expectedState: string,
  timeoutMs = 30_000,
): Promise<void> {
  await expect
    .poll(async () => (await apiClient.getTask(taskId)).state ?? "", {
      timeout: timeoutMs,
      message: `task ${taskId} did not become ${expectedState}`,
    })
    .toBe(expectedState);
}

export async function sessionMessageIds(
  apiClient: ApiClient,
  sessionId: string,
): Promise<string[]> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.map((message) => message.id);
}

export async function expectSessionMessagesUnchanged(
  apiClient: ApiClient,
  sessionId: string,
  expectedMessageIds: string[],
  timeoutMs = 5_000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let unexpectedMessageIds: string[] | null = null;
  await expect
    .poll(
      async () => {
        const actualMessageIds = await sessionMessageIds(apiClient, sessionId);
        if (JSON.stringify(actualMessageIds) !== JSON.stringify(expectedMessageIds)) {
          unexpectedMessageIds = actualMessageIds;
          return true;
        }
        return Date.now() >= deadline;
      },
      {
        // Give the final poll enough time to observe the end of the stability
        // window. With the same timeout on both clocks, an unchanged list can
        // be reported as a timeout instead of a successful invariant check.
        timeout: timeoutMs + 1_000,
        message: `session ${sessionId} received an unexpected message`,
      },
    )
    .toBe(true);
  if (unexpectedMessageIds) {
    throw new Error(
      `session ${sessionId} received unexpected messages: ${JSON.stringify(unexpectedMessageIds)}`,
    );
  }
}

/**
 * Create the smallest real workflow that can demonstrate an accepted launch
 * waiting behind the instance ceiling. The filler task owns the only admitted
 * slot; moving the target then creates its exact destination before retry.
 */
export async function createQueuedSessionOwnershipScenario(
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
): Promise<QueuedSessionOwnershipScenario> {
  const { profileA, profileB } = await createWorkflowAgentProfiles(apiClient);
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${name} workflow`);
  const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Source", 0, {
    is_start_step: true,
    agent_profile_id: profileA.id,
    profile_session_start_policy: "new",
    profile_session_end_policy: "park",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  const destinationStep = await apiClient.createWorkflowStep(workflow.id, "Destination", 1, {
    agent_profile_id: profileB.id,
    profile_session_start_policy: "new",
    profile_session_end_policy: "park",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.updateWorkflowStep(destinationStep.id, {
    prompt: `e2e:message("${QUEUED_DESTINATION_MARKER}")`,
  });

  const taskTitle = `${name} target`;
  const target = await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profileA.id, {
    workflow_id: workflow.id,
    workflow_step_id: sourceStep.id,
    repository_ids: [seedData.repositoryId],
    description: "Prepare the source conversation for inspection",
  });
  const sourceSessionId = await waitForWorkflowProfileSession(apiClient, target.id, profileA.id);

  const filler = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `${name} capacity holder`,
    profileA.id,
    {
      workflow_id: workflow.id,
      workflow_step_id: sourceStep.id,
      repository_ids: [seedData.repositoryId],
      description: "/slow 120s",
    },
  );
  if (!filler.session_id) throw new Error("capacity-holder task did not return a session_id");
  await waitForSessionState(apiClient, filler.id, filler.session_id, "RUNNING");

  await apiClient.moveTask(target.id, workflow.id, destinationStep.id);
  const queue = await waitForLaunchQueue(apiClient, seedData.workspaceId, target.id);
  if (!queue.session_id)
    throw new Error("queued workflow launch did not identify a destination session");

  await waitForSessionState(apiClient, target.id, queue.session_id, "CREATED");
  await waitForTaskState(apiClient, target.id, "SCHEDULING");

  return {
    taskId: target.id,
    taskTitle,
    workflowId: workflow.id,
    sourceSessionId,
    destinationSessionId: queue.session_id,
    fillerSessionId: filler.session_id,
    queue,
    destinationProfileName: profileB.name,
  };
}
