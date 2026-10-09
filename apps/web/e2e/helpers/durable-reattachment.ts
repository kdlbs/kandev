import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import {
  observeAgentRuntimeAvailability,
  waitForAgentRuntimeReplacement,
} from "./agent-runtime-availability";
import { SessionPage } from "../pages/session-page";
import { attachGatewayTrafficCapture } from "./ws-traffic";

type RuntimeStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      agentRuntime: { status: string; runtime_epoch?: number } | null;
    };
  };
};

type RecoveryRecord = {
  phase?: string;
  revision?: number;
  session_id?: string;
  agent_execution_id?: string;
  submission_id?: string;
  stream_id?: string;
  incarnation_id?: string;
  harness_generation?: number;
  prompt_generation?: number;
};

async function waitForUncertainRecovery(apiClient: ApiClient, taskId: string, sessionId: string) {
  let latestSession: unknown;
  try {
    await expect
      .poll(
        async () => {
          const current = await apiClient.listTaskSessions(taskId);
          const session = current.sessions.find((candidate) => candidate.id === sessionId);
          latestSession = session
            ? {
                state: session.state,
                agent_execution_id: session.agent_execution_id,
                queue_incarnation_id: session.queue_incarnation_id,
                error_message: session.error_message,
                last_agent_error: session.metadata?.last_agent_error,
                agent_delivery_recovery: session.metadata?.agent_delivery_recovery,
              }
            : null;
          return (
            (session?.metadata?.agent_delivery_recovery as RecoveryRecord | undefined)?.phase ?? ""
          );
        },
        { timeout: 15_000 },
      )
      .toBe("uncertain");
  } catch (error) {
    throw new Error(`${String(error)}\nLatest session state: ${JSON.stringify(latestSession)}`);
  }
}

function assertPersistedRecoveryIdentity(
  recovery: RecoveryRecord | undefined,
  sessionId: string,
  executionId: string | undefined,
): asserts recovery is RecoveryRecord {
  if (
    recovery?.session_id !== sessionId ||
    recovery.agent_execution_id !== executionId ||
    !recovery.submission_id ||
    !recovery.stream_id ||
    !recovery.incarnation_id ||
    !recovery.harness_generation ||
    !recovery.prompt_generation ||
    !recovery.revision
  ) {
    throw new Error(`persisted recovery identity is incomplete: ${JSON.stringify(recovery)}`);
  }
}

export async function triggerActualDeliveryDisconnect(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const traffic = attachGatewayTrafficCapture(testPage);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
      start_agent: false,
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const sessionPage = new SessionPage(testPage);
  await sessionPage.waitForLoad();
  await sessionPage.sendMessageViaButton("/slow 120");
  await expect(
    testPage.getByTestId("session-chat").getByText("Running slow response", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });

  const bootID = await apiClient.getBackendBootID();
  const runtime = await testPage.evaluate(
    () => (window as RuntimeStoreWindow).__KANDEV_E2E_STORE__?.getState().agentRuntime ?? null,
  );
  if (!runtime || runtime.status !== "available" || runtime.runtime_epoch === undefined) {
    throw new Error("local runtime is not available before the delivery disconnect test");
  }
  const previousEpoch = runtime.runtime_epoch;
  const pageTimeOrigin = await testPage.evaluate(() => performance.timeOrigin);
  await observeAgentRuntimeAvailability(testPage);

  const killed = await apiClient.killLocalAgentRuntimeChild();
  expect(killed.killed).toBe(true);
  expect(killed.runtime_epoch).toBe(previousEpoch);
  await waitForAgentRuntimeReplacement(testPage, previousEpoch, bootID);

  expect(await apiClient.getBackendBootID()).toBe(bootID);
  expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(pageTimeOrigin);
  const sessions = await apiClient.listTaskSessions(task.id);
  const session = sessions.sessions.find((candidate) => candidate.is_primary);
  if (!session) throw new Error("created task has no primary session");

  await waitForUncertainRecovery(apiClient, task.id, session.id);

  const persistedSession = (await apiClient.listTaskSessions(task.id)).sessions.find(
    (candidate) => candidate.id === session.id,
  );
  const recovery = persistedSession?.metadata?.agent_delivery_recovery as
    | RecoveryRecord
    | undefined;
  assertPersistedRecoveryIdentity(recovery, session.id, session.agent_execution_id);

  return { traffic, task, sessionId: session.id, recovery };
}
