import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { openTaskSession } from "./session";

export const DYNAMIC_FALLBACK_SUCCESS = "Dynamic fallback successor response.";
export const DYNAMIC_FALLBACK_DRAFT = "Keep this composer draft during route recovery.";

export async function createDynamicFallbackProfile(
  apiClient: ApiClient,
  seedData: SeedData,
  options: { name: string; enabled: boolean; threshold?: number },
) {
  const firstCandidate = await apiClient.getAgentProfile(seedData.agentProfileId);
  const secondCandidate = await apiClient.createAgentProfile(
    firstCandidate.agentId,
    `${options.name} successor`,
    { model: firstCandidate.model || "mock-fast" },
  );
  const dynamicProfile = await apiClient.createDynamicAgentProfile(options.name, [
    {
      executionProfileId: firstCandidate.id,
      enabled: true,
      unclassifiedEnabled: options.enabled,
      consecutiveFailureThreshold: options.threshold ?? 3,
    },
    {
      executionProfileId: secondCandidate.id,
      enabled: true,
      unclassifiedEnabled: false,
      consecutiveFailureThreshold: 0,
    },
  ]);
  return { dynamicProfile, firstCandidate, secondCandidate };
}

export async function startDynamicFallbackSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  options: { title: string; prompt: string; profileId: string },
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    options.title,
    options.profileId,
    {
      description: options.prompt,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Dynamic fallback task did not return a session ID");
  const session = await openTaskSession(testPage, task.id);
  return { taskId: task.id, sessionId: task.session_id, session };
}

export async function waitForRouteActionRequired(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  minimumCompletedTurnCount?: number,
) {
  let current: Awaited<ReturnType<ApiClient["listTaskSessions"]>>["sessions"][number] | undefined;
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        current = sessions.find((candidate) => candidate.id === sessionId);
        const { turns } = await apiClient.listSessionTurns(sessionId);
        const openTurnCount = turns.filter((turn) => turn.completed_at == null).length;
        return {
          manualRecovery:
            current?.route_state === "action_required" &&
            (minimumCompletedTurnCount === undefined ||
              (turns.length >= minimumCompletedTurnCount && openTurnCount === 0)),
          snapshot: current
            ? {
                state: current.state,
                route_state: current.route_state,
                route_generation: current.route_generation,
                agent_profile_id: current.agent_profile_id,
                execution_profile_id: current.execution_profile_id,
                error_message: current.error_message,
                turn_count: turns.length,
                open_turn_count: openTurnCount,
              }
            : null,
        };
      },
      { timeout: 60_000, message: "Waiting for the dynamic route manual recovery state" },
    )
    .toMatchObject({ manualRecovery: true });
  if (!current) throw new Error(`Task session ${sessionId} disappeared during route recovery`);
  return current;
}

export async function retryCurrentDynamicCandidate(options: {
  page: Page;
  apiClient: ApiClient;
  taskId: string;
  sessionId: string;
  mobile?: boolean;
}) {
  await waitForRouteActionRequired(options.apiClient, options.taskId, options.sessionId);
  const { turns } = await options.apiClient.listSessionTurns(options.sessionId);
  const retry = options.page.getByTestId("dynamic-route-retry");
  if (options.mobile) await retry.tap();
  else await retry.click();
  return waitForRouteActionRequired(
    options.apiClient,
    options.taskId,
    options.sessionId,
    turns.length + 1,
  );
}

export async function expectCurrentCandidate(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  logicalProfileId: string,
  executionProfileId: string,
) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  expect(sessions).toHaveLength(1);
  const current = sessions.find((candidate) => candidate.id === sessionId);
  expect(current).toMatchObject({
    agent_profile_id: logicalProfileId,
    execution_profile_id: executionProfileId,
  });
  return current;
}
