import { test, expect } from "../../fixtures/office-fixture";

test.describe("Office runtime agent creation", () => {
  test("allows CEO-created agents and denies workers without create_agent", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Two full page navigations + token mint + agent CRUD — bump
    // above the 30s default to ride out heavy parallel-suite load.
    test.setTimeout(75_000);
    const ceoRun = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "claimed",
      reason: "heartbeat",
      sessionId: "runtime-ceo-create-agent",
    });
    const ceoToken = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: ceoRun.run_id,
      sessionId: "runtime-ceo-create-agent",
      capabilities: JSON.stringify({ create_agent: true }),
    });
    const createdName = `Runtime Worker ${Date.now()}`;

    const createResponse = await apiClient.runtimeCreateAgent(ceoToken.token, {
      name: createdName,
      role: "worker",
      reason: "runtime e2e coverage",
    });

    expect(createResponse.status).toBe(201);
    const createdBody = (await createResponse.json()) as { agent: { id: string } };
    // The office layout's SSR reads agentProfiles for the active
    // workspace. Confirm the runtime-created agent is visible from
    // the public list endpoint before we navigate — otherwise the
    // layout would render "Agent not found." and never paint the
    // topbar portal (no testid).
    await expect
      .poll(
        async () => {
          const list = (await officeApi.listAgents(officeSeed.workspaceId)) as {
            agents?: Array<{ id: string }>;
          };
          return (list.agents ?? []).some((a) => a.id === createdBody.agent.id);
        },
        { timeout: 10_000, message: "runtime-created agent never appeared in listAgents" },
      )
      .toBe(true);
    await testPage.goto(`/office/agents/${createdBody.agent.id}`);
    await expect(testPage.getByTestId("agent-topbar-name")).toHaveText(createdName, {
      timeout: 30_000,
    });

    const worker = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: `Runtime Denied Worker ${Date.now()}`,
      role: "worker",
    })) as { id: string };
    const workerRun = await apiClient.seedRun({
      agentProfileId: worker.id,
      status: "claimed",
      reason: "heartbeat",
      sessionId: "runtime-worker-create-agent",
    });
    const workerToken = await apiClient.mintRuntimeToken({
      agentProfileId: worker.id,
      workspaceId: officeSeed.workspaceId,
      runId: workerRun.run_id,
      sessionId: "runtime-worker-create-agent",
    });

    const denied = await apiClient.runtimeCreateAgent(workerToken.token, {
      name: `Should Not Exist ${Date.now()}`,
      role: "worker",
    });

    expect(denied.status).toBe(403);
    await testPage.goto(`/office/agents/${worker.id}/runs/${workerRun.run_id}`);
    await expect(testPage.getByTestId("events-log")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("runtime.denied")).toBeVisible();
    await expect(testPage.getByText(/create_agent/)).toBeVisible();
  });
});
