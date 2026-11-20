import type { Page } from "@playwright/test";
import { test, expect, moveTaskToTerminalStep } from "../../fixtures/office-fixture";
import type { ApiClient } from "../../helpers/api-client";
import type { OfficeApiClient } from "../../helpers/office-api-client";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";

type RunRow = {
  id: string;
  task_id?: string;
  reason?: string;
  routing_blocked_status?: string;
};

type Clients = {
  testPage: Page;
  apiClient: ApiClient;
  officeApi: OfficeApiClient;
  officeSeed: { workspaceId: string; agentId: string; workflowId: string };
};

// Office launches with a routed execution profile only when a routable
// provider is registered; the default seed's mock agent is not routable.
async function enableRouting({ apiClient, officeApi, officeSeed }: Clients) {
  await officeApi.updateRouting(
    officeSeed.workspaceId,
    await balancedExecutionProfileRouting(apiClient, officeApi, officeSeed.workspaceId, [
      "claude-acp",
    ]),
  );
  const route = await officeApi.getAgentRoute(officeSeed.agentId);
  expect(route.preview.primary_execution_profile_id).toBeTruthy();
}

// Creates an assigned task held by a blocker and opens its workbench, which
// prepares the task's session while the assignment wake is held.
async function prepareBlockedTask({ testPage, apiClient, officeApi, officeSeed }: Clients) {
  const blocker = (await officeApi.createTask(officeSeed.workspaceId, "Replacement blocker", {
    workflow_id: officeSeed.workflowId,
  })) as { id: string };
  const task = (await officeApi.createTask(officeSeed.workspaceId, "Replacement target", {
    workflow_id: officeSeed.workflowId,
    blocked_by: [blocker.id],
  })) as { id: string };
  await officeApi.assignTask(task.id, officeSeed.agentId);

  await testPage.goto(`/t/${task.id}`);
  await expect
    .poll(
      async () => (await apiClient.listTaskSessions(task.id)).sessions[0]?.agent_execution_id ?? "",
      { timeout: 20_000, message: "opening the task page never prepared a session runtime" },
    )
    .not.toBe("");
  const [prepared] = (await apiClient.listTaskSessions(task.id)).sessions;
  return { blocker, task, prepared };
}

// Completes the blocker and returns the launched run's route attempts once the
// prepared session is live.
async function resolveBlockerAndLaunch(
  { apiClient, officeApi, officeSeed }: Clients,
  blockerId: string,
  taskId: string,
  sessionId: string,
) {
  await moveTaskToTerminalStep(apiClient, officeSeed.workflowId, blockerId);
  const findRun = async () => {
    const { runs } = (await officeApi.listRuns(officeSeed.workspaceId)) as { runs?: RunRow[] };
    return (runs ?? []).find((r) => r.task_id === taskId && r.reason === "task_blockers_resolved");
  };
  let attempts: Array<{ outcome: string; execution_profile_id: string }> = [];
  await expect
    .poll(
      async () => {
        const run = await findRun();
        if (!run) return 0;
        attempts = (await officeApi.listRouteAttempts(run.id)).attempts;
        return attempts.length;
      },
      { timeout: 40_000, message: "the blocker-resolved run never recorded a route attempt" },
    )
    .toBeGreaterThan(0);
  expect(attempts.map((a) => a.outcome)).toEqual(["launched"]);
  expect((await findRun())?.routing_blocked_status ?? "").toBe("");

  const liveSession = async () =>
    (await apiClient.listTaskSessions(taskId)).sessions.find((s) => s.id === sessionId);
  await expect
    .poll(async () => (await liveSession())?.state, {
      timeout: 20_000,
      message: "the prepared session did not reach a live state",
    })
    .toMatch(/^(STARTING|RUNNING|WAITING_FOR_INPUT|IDLE|COMPLETED)$/);
  return { attempts, launched: await liveSession() };
}

/**
 * Issue #4611: a blocked Office task opened in the UI gets a prepared session.
 * When the blocker completes, the Office run launches into that session with
 * its routed execution profile and must start the agent instead of being
 * parked as a provider failure.
 */
test.describe("Office prepared session profile replacement", () => {
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("prepared session binds to the routed profile the run launches with", async ({
    backend,
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const clients = { testPage, apiClient, officeApi, officeSeed };
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp" });
    await enableRouting(clients);
    const { blocker, task, prepared } = await prepareBlockedTask(clients);

    const { attempts, launched } = await resolveBlockerAndLaunch(
      clients,
      blocker.id,
      task.id,
      prepared.id,
    );

    expect(attempts[0].execution_profile_id).toBe(prepared.execution_profile_id);
    // The prepared runtime is reused rather than replaced.
    expect(launched?.agent_execution_id).toBe(prepared.agent_execution_id);
  });

  test("routing that changes after preparation replaces the prepared runtime", async ({
    backend,
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const clients = { testPage, apiClient, officeApi, officeSeed };
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp" });
    const { blocker, task, prepared } = await prepareBlockedTask(clients);
    await enableRouting(clients);

    const { attempts, launched } = await resolveBlockerAndLaunch(
      clients,
      blocker.id,
      task.id,
      prepared.id,
    );

    expect(attempts[0].execution_profile_id).not.toBe(prepared.execution_profile_id);
    expect(launched?.agent_execution_id).not.toBe(prepared.agent_execution_id);
  });
});
