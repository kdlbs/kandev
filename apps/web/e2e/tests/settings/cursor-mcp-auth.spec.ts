import { test, expect } from "../../fixtures/test-base";
import { createCursorMcpAuthFixture } from "../../helpers/cursor-mcp-auth";

test.describe("Cursor MCP authentication preference", () => {
  test("saves, reloads, re-enables, and discards changes for a Cursor strategy profile", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    const fixture = await createCursorMcpAuthFixture(apiClient);

    try {
      await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
      const checkbox = testPage.getByRole("checkbox", {
        name: "Share local Cursor MCP credentials",
      });
      await expect(checkbox).toBeVisible({ timeout: 15_000 });
      await expect(checkbox).toHaveAttribute("aria-checked", "true");

      const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await checkbox.press("Space");
      await expect(checkbox).toHaveAttribute("aria-checked", "false");
      await expect(save).toBeEnabled();
      await save.click();
      await expect
        .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).cursorMcpAuthEnabled)
        .toBe(false);

      await testPage.reload();
      await expect(checkbox).toHaveAttribute("aria-checked", "false");
      await checkbox.press("Space");
      await save.click();
      await expect
        .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).cursorMcpAuthEnabled)
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
      testPage.getByRole("checkbox", { name: "Share local Cursor MCP credentials" }),
    ).toHaveCount(0);
  });
});
