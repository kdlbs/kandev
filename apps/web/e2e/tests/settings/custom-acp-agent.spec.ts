import { test, expect } from "../../fixtures/test-base";

const AGENT_NAME = "E2E Acp Agent";
const AGENT_SLUG = "e2e-acp-agent";

test.describe("Custom ACP agent", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.deleteCustomAgentByName(AGENT_SLUG);
  });

  test("registers a custom agent kandev drives over ACP", async ({ testPage, apiClient }) => {
    await testPage.goto("/settings/agents");
    await testPage.getByTestId("new-agent-button").click();

    const dialog = testPage.getByRole("dialog", { name: "Add custom agent" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("mcp-strategy-select")).toBeVisible();

    await dialog.getByTestId("agent-protocol-select").click();
    await testPage.getByRole("listbox").getByRole("option").filter({ hasText: /^ACP/ }).click();

    // Resolved MCP servers reach an ACP agent through session/new, so the
    // passthrough strategy picker is not offered for it.
    await expect(dialog.getByTestId("mcp-strategy-select")).toHaveCount(0);

    await dialog.getByLabel("Display Name").fill(AGENT_NAME);
    await dialog.getByLabel("Command").fill(`${AGENT_SLUG} --acp`);
    await dialog.getByRole("button", { name: "Create" }).click();

    await expect(testPage.getByTestId(`agent-group-${AGENT_SLUG}`)).toBeVisible();
    await expect(testPage.getByTestId(`agent-mcp-strategy-${AGENT_SLUG}`)).toHaveCount(0);

    // The stored definition is what a restart replays, so it has to say ACP
    // rather than leaving the agent to come back as a terminal one.
    const { agents } = await apiClient.listAgents();
    const created = agents.find((agent) => agent.name === AGENT_SLUG);
    expect(created?.tui_config?.protocol).toBe("acp");
  });
});
