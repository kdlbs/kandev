import { test, expect } from "../../fixtures/test-base";

const AGENT_NAME = "Mobile ACP Agent";
const AGENT_SLUG = "mobile-acp-agent";

test.describe("Custom ACP agent on mobile", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.deleteCustomAgentByName(AGENT_SLUG);
  });

  test("selects ACP and keeps the protocol controls usable", async ({ testPage, apiClient }) => {
    await testPage.goto("/settings/agents");
    await testPage.getByTestId("new-agent-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Add custom agent" });
    await expect(dialog).toBeVisible();

    const protocol = dialog.getByTestId("agent-protocol-select");
    await expect
      .poll(async () => (await protocol.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);

    await protocol.tap();
    const listbox = testPage.getByRole("listbox");
    const acpOption = listbox.getByRole("option", { name: "ACP" });
    await expect(acpOption).toBeVisible();
    await expect
      .poll(async () => (await acpOption.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await acpOption.tap();

    await expect(dialog.getByTestId("mcp-strategy-select")).toHaveCount(0);
    await dialog.getByLabel("Display Name").fill(AGENT_NAME);
    await dialog.getByLabel("Command").fill(`${AGENT_SLUG} --acp`);
    await dialog.getByRole("button", { name: "Create" }).tap();

    await expect(testPage.getByTestId(`agent-group-${AGENT_SLUG}`)).toBeVisible();
    const { agents } = await apiClient.listAgents();
    const created = agents.find((agent) => agent.name === AGENT_SLUG);
    expect(created?.tui_config?.protocol).toBe("acp");
  });
});
