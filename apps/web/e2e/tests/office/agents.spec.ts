import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Agents", () => {
  test("onboarded CEO is listed, opens its dashboard and can be renamed", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const { agents } = (await officeApi.listAgents(officeSeed.workspaceId)) as {
      agents: Array<Record<string, unknown>>;
    };
    expect(agents.length).toBeGreaterThan(0);
    expect(agents.find((agent) => agent.id === officeSeed.agentId)).toMatchObject({ name: "CEO" });
    expect(await officeApi.getAgent(officeSeed.agentId)).toMatchObject({
      id: officeSeed.agentId,
      name: "CEO",
    });
    await testPage.goto("/office/agents");
    await expect(testPage.getByRole("button", { name: /New Agent/i })).toBeVisible({
      timeout: 10_000,
    });
    const card = testPage
      .locator("main")
      .locator('a[href="/office/agents/' + officeSeed.agentId + '"]');
    await expect(card).toHaveCount(1);
    await expect(card.locator("span.truncate").filter({ hasText: /^CEO$/ })).toBeVisible();
    await card.click();
    await expect(testPage.getByTestId("agent-tab-dashboard")).toBeVisible({ timeout: 10_000 });
    try {
      await officeApi.updateAgent(officeSeed.agentId, { name: "CEO Updated" });
      expect((await officeApi.getAgent(officeSeed.agentId)).name).toBe("CEO Updated");
    } finally {
      await officeApi.updateAgent(officeSeed.agentId, { name: "CEO" });
    }
  });

  test("worker creation, status transitions and deletion persist", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const worker = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Lifecycle Worker",
      role: "worker",
    });
    const id = worker.id as string;
    expect(id).toBeTruthy();
    let deleted = false;
    try {
      const { agents } = (await officeApi.listAgents(officeSeed.workspaceId)) as {
        agents: Array<Record<string, unknown>>;
      };
      expect(agents.find((agent) => agent.id === id)).toMatchObject({
        name: "Lifecycle Worker",
        role: "worker",
      });
      const agent = await officeApi.getAgent(id);
      expect(typeof agent.permissions).toBe("string");
      const permissions = JSON.parse(agent.permissions as string);
      expect(permissions).toMatchObject({
        can_create_tasks: true,
        can_approve: false,
        can_create_agents: false,
      });
      await testPage.goto("/office/agents");
      await expect(
        testPage
          .locator("main")
          .locator('a[href="/office/agents/' + id + '"]')
          .getByText("Lifecycle Worker", { exact: true }),
      ).toBeVisible({ timeout: 10_000 });
      for (const status of [
        "paused",
        "idle",
        "pending_approval",
        "idle",
        "pending_approval",
        "stopped",
      ]) {
        expect(await officeApi.updateAgentStatus(id, status)).toBeDefined();
        expect((await officeApi.getAgent(id)).status).toBe(status);
      }
      await officeApi.deleteAgent(id);
      deleted = true;
      const remaining = (await officeApi.listAgents(officeSeed.workspaceId)) as {
        agents: Array<Record<string, unknown>>;
      };
      expect(remaining.agents.some((agent) => agent.id === id)).toBe(false);
    } finally {
      if (!deleted) await officeApi.deleteAgent(id);
    }
  });

  test("paused agent remains reachable from the agents list", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.updateAgentStatus(officeSeed.agentId, "paused");

    await testPage.goto("/office/agents");
    const agentCardLink = testPage.locator(`a[href="/office/agents/${officeSeed.agentId}"]`);
    await expect(agentCardLink).toBeVisible({ timeout: 10_000 });
    await agentCardLink.click();
    await testPage.waitForURL(new RegExp(`/office/agents/${officeSeed.agentId}/dashboard$`));
    await expect(testPage.getByTestId("agent-recovery-control")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("recovery control returns a paused agent to idle and then disappears", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.updateAgentStatus(officeSeed.agentId, "paused", "Manually paused for testing");

    await testPage.goto(`/office/agents/${officeSeed.agentId}/dashboard`);
    const recoveryControl = testPage.getByTestId("agent-recovery-control");
    await expect(recoveryControl).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("agent-pause-reason")).toBeVisible();

    const recovered = waitForHttp(
      testPage,
      "PATCH",
      new RegExp(`/agents/${officeSeed.agentId}/status$`),
    );
    await recoveryControl.click();
    await recovered;

    await expect(recoveryControl).toBeHidden();
    await expect(testPage.getByTestId("agent-pause-reason")).toBeHidden();

    const agent = await officeApi.getAgent(officeSeed.agentId);
    expect((agent as Record<string, unknown>).status).toBe("idle");
  });

  test("recovery control is absent for an idle agent and appears again after a manual stop", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await testPage.goto(`/office/agents/${officeSeed.agentId}/dashboard`);
    await expect(testPage.getByTestId("agent-tab-dashboard")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("agent-recovery-control")).toHaveCount(0);

    await officeApi.updateAgentStatus(officeSeed.agentId, "paused");
    await officeApi.updateAgentStatus(officeSeed.agentId, "stopped");

    await testPage.reload();
    await expect(testPage.getByTestId("agent-recovery-control")).toBeVisible({ timeout: 10_000 });
  });
});
