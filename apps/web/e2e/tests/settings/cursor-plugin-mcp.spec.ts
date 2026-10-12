import { test, expect } from "../../fixtures/test-base";
import { createCursorMcpAuthFixture } from "../../helpers/cursor-mcp-auth";

test.describe("Cursor plugin MCP import preference", () => {
  test("discovers grouped MCP servers and saves selected-only IDs even after refresh fails", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    const fixture = await createCursorMcpAuthFixture(apiClient);
    let discoveryCalls = 0;
    let failRefresh = false;
    await testPage.route(`**/api/v1/agents/${fixture.agentId}/mcp-discovery`, async (route) => {
      discoveryCalls += 1;
      if (failRefresh) {
        await route.fulfill({ status: 503, body: "discovery unavailable" });
        return;
      }
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
              name: "Jira tools",
              plugin_name: "Atlassian tools",
              source_kind: "plugin",
              credentials_available: true,
            },
            {
              id: "global-build-server",
              name: "Build tools",
              source_kind: "global",
              credentials_available: false,
            },
          ],
        }),
      });
    });

    try {
      await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
      const selector = testPage.getByTestId("cursor-mcp-selection");
      await expect(selector).toBeVisible({ timeout: 15_000 });
      await expect.poll(() => discoveryCalls).toBeGreaterThan(0);
      await expect(selector.getByText("Atlassian tools")).toBeVisible();
      await expect(selector.getByText("Other servers")).toBeVisible();
      await expect(selector.getByText("Credentials available")).toBeVisible();

      await testPage.getByRole("radio", { name: "Selected servers only" }).click();
      const jira = testPage.getByRole("checkbox", { name: "Select Jira tools" });
      await jira.click();
      await expect(jira).toHaveAttribute("aria-checked", "true");

      failRefresh = true;
      await testPage.getByTestId("cursor-mcp-discovery-refresh").click();
      await expect(selector).toContainText("Could not load MCP server discovery");
      await expect(jira).toHaveAttribute("aria-checked", "true");

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

  test("saves, reloads, re-enables, and discards changes for a Cursor strategy profile", async ({
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
      await expect(checkbox).toBeVisible({ timeout: 15_000 });
      await expect(checkbox).toHaveAttribute("aria-checked", "true");

      const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await checkbox.press("Space");
      await expect(checkbox).toHaveAttribute("aria-checked", "false");
      await expect(save).toBeEnabled();
      await save.click();
      await expect
        .poll(
          async () => (await apiClient.getAgentProfile(fixture.profileId)).cursorPluginsMcpEnabled,
        )
        .toBe(false);

      await testPage.reload();
      await expect(checkbox).toHaveAttribute("aria-checked", "false");
      await checkbox.press("Space");
      await save.click();
      await expect
        .poll(
          async () => (await apiClient.getAgentProfile(fixture.profileId)).cursorPluginsMcpEnabled,
        )
        .toBe(true);

      await checkbox.press("Space");
      await expect(testPage.getByRole("button", { name: /^Reset$/i }).first()).toBeEnabled();
      await testPage
        .getByRole("button", { name: /^Reset$/i })
        .first()
        .click();
      await expect(checkbox).toHaveAttribute("aria-checked", "true");
    } finally {
      await fixture.dispose();
    }
  });

  test("hides the preference for non-Cursor profiles", async ({ testPage, apiClient }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((candidate) => candidate.name === "mock-agent");
    expect(agent?.profiles[0]).toBeDefined();
    await testPage.goto(`/settings/agents/${agent!.name}/profiles/${agent!.profiles[0].id}`);
    await expect(testPage.getByTestId("profile-name-input")).toBeVisible({ timeout: 15_000 });
    await expect(
      testPage.getByRole("checkbox", { name: "Import local Cursor plugin MCP servers" }),
    ).toHaveCount(0);
  });
});
