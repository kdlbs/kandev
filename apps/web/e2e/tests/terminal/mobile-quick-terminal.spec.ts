import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const QUICK_TERMINAL_TITLE = "Quick terminal";

test.describe("mobile quick terminal", () => {
  test("uses a full-height terminal without horizontal overflow", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto("/");

    const terminalButton = testPage.getByTestId("mobile-quick-terminal-button");
    const quickChatButton = testPage.getByTestId("mobile-quick-chat-button");
    await expect(terminalButton).toBeVisible();
    await expect(quickChatButton).toBeVisible();
    expect(
      await terminalButton.evaluate((element) =>
        element.nextElementSibling?.getAttribute("data-testid"),
      ),
    ).toBe("mobile-quick-chat-button");

    await terminalButton.tap();
    const dialog = testPage.getByRole("dialog", { name: QUICK_TERMINAL_TITLE });
    await expect(dialog).toBeVisible();
    const terminal = testPage.getByTestId("host-shell-terminal");
    await expect(terminal).toBeVisible();

    const viewport = testPage.viewportSize();
    const dialogBox = await dialog.boundingBox();
    const terminalBox = await terminal.boundingBox();
    expect(viewport).not.toBeNull();
    expect(dialogBox).not.toBeNull();
    expect(terminalBox).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(-1);
    expect(dialogBox!.y).toBeGreaterThanOrEqual(-1);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport!.width + 1);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewport!.height + 1);
    expect(dialogBox!.width).toBeGreaterThanOrEqual(viewport!.width - 8);
    expect(dialogBox!.height).toBeGreaterThanOrEqual(viewport!.height - 8);
    expect(terminalBox!.height).toBeGreaterThan(400);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile quick terminal");
    await prCapture.screenshot("mobile-full-height-terminal", {
      caption: "Pixel 5 Quick Terminal full-height surface",
    });

    await testPage.getByTestId("host-shell-done").tap();
    await expect(dialog).toBeHidden();
    await expect(terminalButton).toBeFocused();
  });
});
