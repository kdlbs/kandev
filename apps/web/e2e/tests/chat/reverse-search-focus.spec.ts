import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";

test.describe("Reverse search focus restoration", () => {
  // @covers AC-UI-REVERSE-SEARCH-FOCUS-001.1
  // @covers AC-UI-REVERSE-SEARCH-FOCUS-001.3
  test("Escape restores task chat focus and preserves the draft", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Reverse search focus restoration",
    );
    const chat = session.activeChat();
    const editor = chat.locator(".tiptap.ProseMirror:visible").first();
    await editor.fill("task chat draft");
    await expect(editor).toHaveText("task chat draft");

    await testPage.keyboard.press("Control+r");
    const overlay = testPage.getByTestId("history-search-overlay");
    await expect(overlay).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("history-search-input")).toBeFocused();

    await testPage.keyboard.press("Escape");

    await expect(overlay).not.toBeVisible();
    await expect(editor).toBeFocused();
    await testPage.keyboard.type(" continued");
    await expect(editor).toHaveText("task chat draft continued");

    await testPage.keyboard.press("Control+r");
    await expect(testPage.getByTestId("history-search-input")).toBeFocused();
    await testPage.evaluate(() => window.dispatchEvent(new Event("resize")));
    await expect(overlay).not.toBeVisible();
    await expect(editor).not.toBeFocused();
  });
});
