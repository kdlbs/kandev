import { type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { startQuickChatFromSetup } from "./quick-chat-helpers";
import { NATIVE_SUGGESTION, setPromptSuggestions } from "./prompt-suggestion-helpers";

async function openMobileQuickChat(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  await page.getByTestId("app-nav-trigger").tap();
  await page.getByTestId("mobile-quick-chat-button").tap();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

// @covers AC-UI-PROMPT-SUGGEST-004.4 AC-UI-PROMPT-SUGGEST-004.8
test("phone: the Use reply button is a 44px touch target that fills the draft", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, true, false);
  try {
    const dialog = await openMobileQuickChat(testPage);
    await startQuickChatFromSetup(dialog, testPage);
    const ghost = dialog.locator("p.has-prompt-suggestion");
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);

    const accept = dialog.getByTestId("prompt-suggestion-accept");
    await expect(accept).toHaveText("Use reply");
    const box = await accept.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);

    await accept.tap();
    await expect(dialog.locator(".tiptap.ProseMirror")).toHaveText(NATIVE_SUGGESTION);
    await expect(ghost).toHaveCount(0);
    await expect(accept).toHaveCount(0);
  } finally {
    await setPromptSuggestions(apiClient, false, false);
  }
});
