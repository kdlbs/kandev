import { test, expect } from "../../fixtures/test-base";
import { createCursorMcpAuthFixture } from "../../helpers/cursor-mcp-auth";

test("phone users can toggle and save Cursor MCP authentication", async ({
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
    const label = testPage.getByText("Share local Cursor MCP credentials", { exact: true });
    await expect(checkbox).toBeVisible({ timeout: 15_000 });
    await expect(checkbox).toHaveAttribute("aria-checked", "true");

    const row = testPage.getByTestId("cursor-mcp-auth-preference");
    const box = await row.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    await label.tap();
    await expect(checkbox).toHaveAttribute("aria-checked", "false");

    const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
    await save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).cursorMcpAuthEnabled)
      .toBe(false);

    await testPage.reload();
    await expect(checkbox).toHaveAttribute("aria-checked", "false");
    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);
    await testPage.getByTestId("cursor-mcp-auth-preference").scrollIntoViewIfNeeded();
    await testPage.screenshot({
      path: test.info().outputPath("cursor-mcp-auth-mobile.png"),
    });
  } finally {
    await fixture.dispose();
  }
});
