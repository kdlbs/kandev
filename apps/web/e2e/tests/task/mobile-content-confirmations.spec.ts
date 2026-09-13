import { test, expect } from "../../fixtures/test-base";
import { startQuickChatFromSetup } from "../chat/quick-chat-helpers";
import { waitForFiniteAnimations } from "../../helpers/animations";

test("Quick Chat hosts context reset without losing its draft or dialog", async ({
  testPage,
  prCapture,
}) => {
  await testPage.goto("/");
  await testPage.getByTestId("mobile-topbar-menu").tap();
  await testPage.getByTestId("mobile-quick-chat-button").tap();
  const chat = testPage.getByRole("dialog", { name: "Quick Chat" });
  await startQuickChatFromSetup(chat, testPage);
  const reset = chat.getByTestId("reset-context-button");
  await expect(reset).toBeVisible({ timeout: 30_000 });
  const editor = chat.locator(".tiptap.ProseMirror");
  await editor.fill("Keep this unsent mobile draft");
  const dialogId = await chat.getAttribute("id");
  await reset.tap();
  const step = testPage.getByRole("dialog", { name: "Reset agent context?" });
  await expect(step).toHaveAttribute("id", dialogId!);
  await expect(testPage.locator('[data-slot="dialog-content"]')).toHaveCount(1);
  await expect(testPage.locator('[data-slot="drawer-content"]')).toHaveCount(0);
  await expect(step.getByRole("button", { name: "Cancel" })).toBeFocused();
  await waitForFiniteAnimations(step);
  await expect(testPage.getByRole("tooltip")).toHaveCount(0);
  await prCapture.screenshot("quick-chat-context-reset-step", {
    caption: "Quick Chat keeps one modal and the unsent draft behind its context-reset step",
  });
  await step.getByRole("button", { name: "Back" }).tap();
  await expect(chat).toBeVisible();
  await expect(editor).toHaveText("Keep this unsent mobile draft");
  await expect(reset).toBeFocused();
});
