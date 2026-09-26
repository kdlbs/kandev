import { test, expect } from "../../fixtures/test-base";
import { startQuickChatFromSetup } from "./quick-chat-helpers";

test.describe("Reverse search focus restoration on mobile", () => {
  // @covers AC-UI-REVERSE-SEARCH-FOCUS-001.1
  // @covers AC-UI-REVERSE-SEARCH-FOCUS-001.3
  test("Escape returns the phone Quick Chat editor to the hardware keyboard", async ({
    testPage,
  }) => {
    test.setTimeout(90_000);
    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await startQuickChatFromSetup(dialog, testPage);
    const editor = dialog.locator(".tiptap.ProseMirror:visible").first();
    await editor.fill("phone quick chat draft");

    await testPage.keyboard.press("Control+r");
    const overlay = dialog.getByTestId("history-search-overlay");
    await expect(overlay).toBeVisible({ timeout: 10_000 });
    await expect(dialog.getByTestId("history-search-input")).toBeFocused();

    await testPage.keyboard.press("Escape");

    await expect(overlay).not.toBeVisible();
    await expect(dialog).toBeVisible();
    await expect(editor).toBeFocused();
    await testPage.keyboard.type(" continued");
    await expect(editor).toHaveText("phone quick chat draft continued");
  });
});
