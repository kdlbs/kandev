import { test, expect, moveTaskToTerminalStep } from "../../fixtures/office-fixture";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";

type RunRow = {
  id: string;
  task_id?: string;
  reason?: string;
  status?: string;
  routing_blocked_status?: string;
};

/**
 * Issue #4611: a blocked Office task opened in the UI gets a prepared session
 * bound to the profile it was prepared with. When the blocker completes, the
 * Office run launches into that session with its routed execution profile and
 * replaces the prepared execution. The run must start the agent instead of
 * being parked as a provider failure.
 */
test.describe("Office prepared session profile replacement", () => {
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("blocker-resolved run launches into the task's prepared session", async ({
    backend,
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Office launches with a routed execution profile only when a routable
    // provider is registered; the default seed's mock agent is not routable,
    // so without this the launch reuses the prepared session's own profile.
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp" });
    await officeApi.updateRouting(
      officeSeed.workspaceId,
      await balancedExecutionProfileRouting(apiClient, officeApi, officeSeed.workspaceId, [
        "claude-acp",
      ]),
    );
    const route = await officeApi.getAgentRoute(officeSeed.agentId);
    expect(route.preview.primary_execution_profile_id).toBeTruthy();

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
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 20_000,
        message: "opening the task page never prepared a session",
      })
      .toBeGreaterThan(0);
    const [prepared] = (await apiClient.listTaskSessions(task.id)).sessions;

    await moveTaskToTerminalStep(apiClient, officeSeed.workflowId, blocker.id);

    const findRun = async () => {
      const { runs } = (await officeApi.listRuns(officeSeed.workspaceId)) as { runs?: RunRow[] };
      return (runs ?? []).find(
        (r) => r.task_id === task.id && r.reason === "task_blockers_resolved",
      );
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

    expect(attempts[0].execution_profile_id).not.toBe(prepared.execution_profile_id);
    expect(attempts.map((a) => a.outcome)).toEqual(["launched"]);
    expect((await findRun())?.routing_blocked_status ?? "").toBe("");

    await expect
      .poll(
        async () =>
          (await apiClient.listTaskSessions(task.id)).sessions.find((s) => s.id === prepared.id)
            ?.state,
        { timeout: 20_000, message: "the prepared session did not reach a live state" },
      )
      .toMatch(/^(STARTING|RUNNING|WAITING_FOR_INPUT|IDLE|COMPLETED)$/);
  });
});
