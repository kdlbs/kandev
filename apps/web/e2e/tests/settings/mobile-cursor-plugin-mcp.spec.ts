import { test, expect } from "../../fixtures/test-base";
import { createCursorMcpAuthFixture } from "../../helpers/cursor-mcp-auth";

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
