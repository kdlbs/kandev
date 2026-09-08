import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Agents", () => {
  test("list agents returns CEO agent from onboarding", async ({ officeApi, officeSeed }) => {
    const result = await officeApi.listAgents(officeSeed.workspaceId);
    const agents = (result as { agents?: Record<string, unknown>[] }).agents ?? [];
    expect(agents.length).toBeGreaterThan(0);
    const ceo = agents.find((a) => (a as Record<string, unknown>).id === officeSeed.agentId);
    expect(ceo).toBeDefined();
    expect((ceo as Record<string, unknown>).name).toBe("CEO");
  });

  test("get agent by id returns correct data", async ({ officeApi, officeSeed }) => {
    const agent = await officeApi.getAgent(officeSeed.agentId);
    const a = agent as Record<string, unknown>;
    expect(a.id).toBe(officeSeed.agentId);
    expect(a.name).toBe("CEO");
  });

  test("create worker agent appears in list", async ({ officeApi, officeSeed }) => {
    await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Worker",
      role: "worker",
    });

    const result = await officeApi.listAgents(officeSeed.workspaceId);
    const agents = (result as { agents?: Record<string, unknown>[] }).agents ?? [];
    const worker = agents.find((a) => (a as Record<string, unknown>).name === "Worker");
    expect(worker).toBeDefined();
    expect((worker as Record<string, unknown>).role).toBe("worker");
  });

  test("update agent name persists", async ({ officeApi, officeSeed }) => {
    await officeApi.updateAgent(officeSeed.agentId, { name: "CEO Updated" });
    const agent = await officeApi.getAgent(officeSeed.agentId);
    expect((agent as Record<string, unknown>).name).toBe("CEO Updated");
  });

  test("delete agent removes it from list", async ({ officeApi, officeSeed }) => {
    const created = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Temp Agent",
      role: "worker",
    });
    const id = (created as Record<string, unknown>).id as string;
    expect(id).toBeTruthy();

    await officeApi.deleteAgent(id);

    const result = await officeApi.listAgents(officeSeed.workspaceId);
    const agents = (result as { agents?: Record<string, unknown>[] }).agents ?? [];
    const found = agents.find((a) => (a as Record<string, unknown>).id === id);
    expect(found).toBeUndefined();
  });

  test("update agent status to paused", async ({ officeApi, officeSeed }) => {
    const result = await officeApi.updateAgentStatus(officeSeed.agentId, "paused");
    const r = result as Record<string, unknown>;
    expect(r).toBeDefined();

    const agent = await officeApi.getAgent(officeSeed.agentId);
    expect((agent as Record<string, unknown>).status).toBe("paused");
  });

  test("update agent status to active (resume)", async ({ officeApi, officeSeed }) => {
    // First pause, then resume — valid transition: paused → idle
    await officeApi.updateAgentStatus(officeSeed.agentId, "paused");
    await officeApi.updateAgentStatus(officeSeed.agentId, "idle");

    const agent = await officeApi.getAgent(officeSeed.agentId);
    expect((agent as Record<string, unknown>).status).toBe("idle");
  });

  test("agents page shows CEO from onboarding", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/agents");
    // Agent names appear in <span> elements within cards, not headings.
    // Name may have been updated by prior tests in the same worker (e.g. "CEO Updated"),
    // so match any span whose text contains "CEO".
    await expect(testPage.locator("span").filter({ hasText: /CEO/ }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("agents page shows New Agent button", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/agents");
    await expect(testPage.getByRole("button", { name: /New Agent/i })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("agent detail page shows name and role", async ({ testPage, officeSeed: _officeSeed }) => {
    // Navigate to the agents list first. After the agent cards render, click the
    // card to perform a client-side navigation to the detail page. This avoids a
    // full-page reload that would reset the Zustand store before the detail page
    // reads from it.
    // Name may have been updated by prior tests in the same worker (e.g. "CEO Updated"),
    // so match any element whose text contains "CEO".
    await testPage.goto("/office/agents");
    // Wait for at least one agent card to appear so the store is populated.
    const agentCardLink = testPage.locator("a").filter({ hasText: /CEO/ }).first();
    await expect(agentCardLink).toBeVisible({ timeout: 10_000 });
    await agentCardLink.click();
    // The agent name moved into the office topbar's portal slot when
    // the detail layout was simplified — the page-level <h2> heading
    // was removed alongside the "Back to agents" link. Confirm the
    // detail surface mounted by waiting for the tab navigation
    // (data-testid="agent-tab-dashboard") which the layout only
    // renders once the agent record resolves.
    await expect(testPage.getByTestId("agent-tab-dashboard")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("newly created agent appears on agents page", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.createAgent(officeSeed.workspaceId, {
      name: "UI Worker",
      role: "worker",
    });

    await testPage.goto("/office/agents");
    // Agent names appear in <span> elements within cards, not headings.
    await expect(
      testPage
        .locator("span")
        .filter({ hasText: /^UI Worker$/ })
        .first(),
    ).toBeVisible({
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
