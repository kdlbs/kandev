import { test, expect } from "../../fixtures/test-base";

test.describe("Custom TUI MCP strategy selector on mobile", () => {
  test("keeps the selector contained and touch targets usable", async ({ testPage }) => {
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.1
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.2
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.3
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.4
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.5
    await testPage.goto("/settings/agents");
    await testPage.getByTestId("new-agent-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Add TUI Agent" });
    await expect(dialog).toBeVisible();

    const trigger = dialog.getByTestId("mcp-strategy-select");
    await expect
      .poll(async () => (await trigger.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    const dialogBox = await dialog.boundingBox();

    await trigger.tap();
    const option = testPage.getByRole("option").filter({ hasText: /^opencode/i });
    await expect(option).toBeVisible();

    const descriptionId = await option.getAttribute("aria-describedby");
    expect(descriptionId).toBeTruthy();
    await expect(testPage.locator(`[id="${descriptionId}"]`)).toContainText("OPENCODE_CONFIG");

    const listbox = testPage.getByRole("listbox");
    await expect
      .poll(async () => (await option.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    const [listboxBox, viewport] = await Promise.all([
      listbox.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    expect(dialogBox).not.toBeNull();
    expect(listboxBox).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(listboxBox!.x).toBeGreaterThanOrEqual(0);
    expect(listboxBox!.x + listboxBox!.width).toBeLessThanOrEqual(viewport.width);

    await option.tap();
    await expect(trigger).toHaveText("opencode");
    await expect(
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).resolves.toBe(true);
  });
});
