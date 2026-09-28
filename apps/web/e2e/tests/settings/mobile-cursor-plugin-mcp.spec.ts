import { test, expect } from "../../fixtures/test-base";
import { createCursorMcpAuthFixture } from "../../helpers/cursor-mcp-auth";

test("phone users can select and save a discovered MCP server without horizontal overflow", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await createCursorMcpAuthFixture(apiClient);
  await testPage.route(`**/api/v1/agents/${fixture.agentId}/mcp-discovery`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        agent_id: fixture.agentId,
        provider_id: "cursor",
        status: "ready",
        servers: [
          {
            id: "plugin-atlassian-jira",
            name: "Atlassian MCP server with a long translated name that should wrap on a phone",
            plugin_name: "Atlassian tools",
            source_kind: "plugin",
            credentials_available: true,
          },
        ],
      }),
    });
  });

  try {
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const selector = testPage.getByTestId("cursor-mcp-selection");
    await expect(selector).toBeVisible({ timeout: 15_000 });
    const inherited = testPage.getByRole("checkbox", {
      name: "Select Atlassian MCP server with a long translated name that should wrap on a phone",
    });
    await expect(inherited).toHaveAttribute("aria-checked", "true");
    await expect(inherited).toHaveAttribute("disabled", "");

    const row = selector.getByTestId("cursor-mcp-server-row");
    await expect
      .poll(async () => row.evaluate((element) => getComputedStyle(element).flexDirection))
      .toBe("column");
    const rowBox = await row.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await testPage.getByRole("radio", { name: "Selected servers only" }).tap();
    await inherited.tap();
    await expect(inherited).toHaveAttribute("aria-checked", "true");
    await testPage
      .getByRole("button", { name: /^Save( changes)?$/i })
      .first()
      .click();
    await expect
      .poll(async () => {
        const profile = await apiClient.getAgentProfile(fixture.profileId);
        return [profile.mcpSelectionMode, profile.mcpSelectedServers];
      })
      .toEqual(["selected", ["plugin-atlassian-jira"]]);
  } finally {
    await fixture.dispose();
  }
});

test("phone users can toggle and save Cursor plugin MCP import preference", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await createCursorMcpAuthFixture(apiClient);

  try {
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const checkbox = testPage.getByRole("checkbox", {
      name: "Import local Cursor plugin MCP servers",
    });
    const label = testPage.getByText("Import local Cursor plugin MCP servers", { exact: true });
    await expect(checkbox).toBeVisible({ timeout: 15_000 });
    await expect(checkbox).toHaveAttribute("aria-checked", "true");

    const row = testPage.getByTestId("cursor-plugins-mcp-preference");
    const box = await row.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    await label.tap();
    await expect(checkbox).toHaveAttribute("aria-checked", "false");

    const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
    await save.click();
    await expect
      .poll(
        async () => (await apiClient.getAgentProfile(fixture.profileId)).cursorPluginsMcpEnabled,
      )
      .toBe(false);

    await testPage.reload();
    await expect(checkbox).toHaveAttribute("aria-checked", "false");
    await label.tap();
    await expect(checkbox).toHaveAttribute("aria-checked", "true");
    await save.click();
    await expect
      .poll(
        async () => (await apiClient.getAgentProfile(fixture.profileId)).cursorPluginsMcpEnabled,
      )
      .toBe(true);

    await label.tap();
    await expect(checkbox).toHaveAttribute("aria-checked", "false");
    const reset = testPage.getByRole("button", { name: /^Reset$/i }).first();
    await expect(reset).toBeEnabled();
    await reset.click();
    await expect(checkbox).toHaveAttribute("aria-checked", "true");
  } finally {
    await fixture.dispose();
  }
});
