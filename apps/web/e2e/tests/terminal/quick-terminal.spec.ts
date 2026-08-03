import { expect, test } from "../../fixtures/test-base";
import { assertLocatorWithinViewportX } from "../../helpers/layout-assertions";

const QUICK_TERMINAL_TITLE = "Quick terminal";

test.describe("quick terminal", () => {
  test("opens from the desktop sidebar and uses the larger floating surface", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto("/");

    const terminalButton = testPage.getByTestId("sidebar-quick-terminal-shortcut");
    const quickChatButton = testPage.getByTestId("sidebar-quick-chat-shortcut");
    await expect(terminalButton).toBeVisible();
    await expect(quickChatButton).toBeVisible();
    expect(
      await terminalButton.evaluate((element) =>
        element.nextElementSibling?.getAttribute("data-testid"),
      ),
    ).toBe("sidebar-quick-chat-shortcut");

    await terminalButton.click();
    const dialog = testPage.getByRole("dialog", { name: QUICK_TERMINAL_TITLE });
    await expect(dialog).toBeVisible();
    await expect(testPage.getByTestId("host-shell-terminal")).toBeVisible();

    const dialogBox = await dialog.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.width).toBeGreaterThan(820);
    expect(dialogBox!.height).toBeGreaterThan(420);
    await assertLocatorWithinViewportX(dialog, "desktop quick terminal dialog");
    await prCapture.screenshot("desktop-floating-terminal", {
      caption: "Desktop Quick Terminal floating surface",
    });

    await testPage.getByTestId("host-shell-done").click();
    await expect(dialog).toBeHidden();
    await expect(terminalButton).toBeFocused();

    if (process.env.CAPTURE_PR_ASSETS) {
      await testPage.setViewportSize({ width: 700, height: 800 });
      await testPage.goto("/");
      const tabletTerminalButton = testPage.getByTestId("tablet-quick-terminal-button");
      await expect(tabletTerminalButton).toBeVisible();
      await tabletTerminalButton.click();
      const tabletDialog = testPage.getByRole("dialog", { name: QUICK_TERMINAL_TITLE });
      await expect(tabletDialog).toBeVisible();
      await expect(testPage.getByTestId("host-shell-terminal")).toBeVisible();
      await prCapture.screenshot("tablet-floating-terminal", {
        caption: "Tablet Quick Terminal floating surface",
      });
      await testPage.getByTestId("host-shell-done").click();
      await expect(tabletDialog).toBeHidden();
    }
  });

  test("keeps the tablet action order and hit targets", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 700, height: 800 });
    await testPage.goto("/");

    const terminalButton = testPage.getByTestId("tablet-quick-terminal-button");
    const quickChatButton = testPage.getByTestId("tablet-quick-chat-button");
    await expect(terminalButton).toBeVisible();
    await expect(quickChatButton).toBeVisible();
    expect(
      await terminalButton.evaluate((element) =>
        element.nextElementSibling?.getAttribute("data-testid"),
      ),
    ).toBe("tablet-quick-chat-button");

    const buttonBox = await terminalButton.boundingBox();
    expect(buttonBox).not.toBeNull();
    expect(buttonBox!.width).toBeGreaterThanOrEqual(44);
    expect(buttonBox!.height).toBeGreaterThanOrEqual(44);

    await terminalButton.click();
    const dialog = testPage.getByRole("dialog", { name: QUICK_TERMINAL_TITLE });
    await expect(dialog).toBeVisible();
    await expect(testPage.getByTestId("host-shell-terminal")).toBeVisible();

    const dialogBox = await dialog.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.width).toBeGreaterThan(500);
    expect(dialogBox!.height).toBeGreaterThan(420);
    await assertLocatorWithinViewportX(dialog, "tablet quick terminal dialog");
    await testPage.getByTestId("host-shell-done").click();
    await expect(dialog).toBeHidden();
    await expect(terminalButton).toBeFocused();
  });
});
