import { test, expect } from "../../fixtures/test-base";

test.describe("Custom TUI MCP strategy selector", () => {
  test("keeps strategy descriptions out of the selected value and dialog width", async ({
    testPage,
  }) => {
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.1
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.2
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.3
    // @covers AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.4
    await testPage.goto("/settings/agents");
    await testPage.getByTestId("new-agent-button").click();

    const dialog = testPage.getByRole("dialog", { name: "Add TUI Agent" });
    await expect(dialog).toBeVisible();

    const trigger = dialog.getByTestId("mcp-strategy-select");
    const dialogBox = await dialog.boundingBox();
    await trigger.click();

    const listbox = testPage.getByRole("listbox");
    const option = listbox.getByRole("option").filter({ hasText: /^opencode/i });
    await expect(option).toBeVisible();
    const descriptionId = await option.getAttribute("aria-describedby");
    expect(descriptionId).toBeTruthy();
    await expect(testPage.locator(`[id="${descriptionId}"]`)).toContainText("OPENCODE_CONFIG");

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

    await option.click();
    await expect(trigger).toHaveText("opencode");
    await expect(
      dialog.evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).resolves.toBe(true);
    await expect(
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).resolves.toBe(true);
  });
});
