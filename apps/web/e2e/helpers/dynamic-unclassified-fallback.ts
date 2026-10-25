import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { openTaskSession } from "./session";

export const DYNAMIC_FALLBACK_SUCCESS = "Dynamic fallback successor response.";
export const DYNAMIC_FALLBACK_DRAFT = "Keep this composer draft during route recovery.";

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessions: {
        items: Record<string, { route_generation?: number; route_state?: string }>;
      };
    };
  };
};

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

export async function cleanupDynamicFallbackProfiles(
  apiClient: ApiClient,
  dynamicProfileIds: string[],
  candidateProfileIds: string[],
) {
  const failures: unknown[] = [];
  for (const profileId of [...dynamicProfileIds, ...candidateProfileIds]) {
    try {
      await apiClient.deleteAgentProfile(profileId, true);
    } catch (error) {
      failures.push(error);
    }
  }
  if (failures.length > 0) {
    throw new AggregateError(failures, "Failed to clean up dynamic fallback test profiles");
  }
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
  let latestSnapshot: Record<string, unknown> | null = null;
  try {
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskId);
          current = sessions.find((candidate) => candidate.id === sessionId);
          const { turns } = await apiClient.listSessionTurns(sessionId);
          const openTurnCount = turns.filter((turn) => turn.completed_at == null).length;
          latestSnapshot = current
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
            : null;
          return {
            manualRecovery:
              current?.route_state === "action_required" &&
              (minimumCompletedTurnCount === undefined ||
                (turns.length >= minimumCompletedTurnCount && openTurnCount === 0)),
            snapshot: latestSnapshot,
          };
        },
        { timeout: 60_000, message: "Waiting for the dynamic route manual recovery state" },
      )
      .toMatchObject({ manualRecovery: true });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(
      `${message}\nLatest route recovery session: ${JSON.stringify(latestSnapshot)}`,
      {
        cause: error,
      },
    );
  }
  if (!current) throw new Error(`Task session ${sessionId} disappeared during route recovery`);
  return current;
}

export async function waitForDynamicRouteRecoveryControls(page: Page, sessionId: string) {
  await page.waitForFunction(
    (id) => {
      const session = (window as E2EStoreWindow).__KANDEV_E2E_STORE__?.getState().taskSessions
        .items[id];
      return session?.route_state === "action_required" && session.route_generation !== undefined;
    },
    sessionId,
    { timeout: 30_000, message: "Waiting for dynamic route recovery state in the page store" },
  );
  await expect(page.getByTestId("dynamic-route-retry")).toBeVisible({ timeout: 30_000 });
}

export async function retryCurrentDynamicCandidate(options: {
  page: Page;
  apiClient: ApiClient;
  taskId: string;
  sessionId: string;
  mobile?: boolean;
}) {
  await waitForRouteActionRequired(options.apiClient, options.taskId, options.sessionId);
  await waitForDynamicRouteRecoveryControls(options.page, options.sessionId);
  const { turns } = await options.apiClient.listSessionTurns(options.sessionId);
  const retry = options.page.getByTestId("dynamic-route-retry");
  if (options.mobile) await retry.tap();
  else await retry.click();
  const failedAgain = await waitForRouteActionRequired(
    options.apiClient,
    options.taskId,
    options.sessionId,
    turns.length + 1,
  );
  await waitForDynamicRouteRecoveryControls(options.page, options.sessionId);
  return failedAgain;
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
