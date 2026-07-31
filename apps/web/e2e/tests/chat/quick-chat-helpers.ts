import { type Locator, type Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";

/**
 * Shared Quick Chat drivers. Kept out of the spec files so several specs
 * (single-client behaviour and cross-device sync) can drive the same surface
 * without duplicating its eager-init timing workarounds.
 */

export async function openQuickChatSetup(page: Page, navigateHome = true): Promise<Locator> {
  if (navigateHome) {
    await page.goto("/");
    await page.waitForLoadState("networkidle");
  }

  if (navigateHome) {
    // Open Quick Chat via keyboard shortcut (Cmd+Shift+Q / Ctrl+Shift+Q).
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${modifier}+Shift+q`);
  } else {
    await page.getByTestId("sidebar-quick-chat-shortcut").click();
  }

  // Wait for Quick Chat dialog.
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  // If a stale session tab is showing, click "+" to start a fresh setup form.
  const setup = dialog.getByTestId("quick-chat-setup");
  if (!(await setup.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByLabel("Start new chat").click();
  }
  await expect(setup).toBeVisible({ timeout: 5_000 });
  return dialog;
}

export async function selectAgentIfNeeded(dialog: Locator, page: Page) {
  const selector = dialog.getByTestId("agent-profile-selector");
  if (
    await selector
      .getByText("Select agent", { exact: false })
      .isVisible()
      .catch(() => false)
  ) {
    await selector.click();
    await page.getByRole("option").first().click();
  }
}

export async function startQuickChatFromSetup(dialog: Locator, page: Page) {
  await selectAgentIfNeeded(dialog, page);
  await dialog.getByTestId("quick-chat-start").click();

  // Wait for chat input to appear AND become editable. Eager init means the
  // agent starts during the HTTP request, so the input is briefly disabled
  // while the FE store catches up to the RUNNING session state.
  const editor = dialog.locator(".tiptap.ProseMirror");
  await expect(editor).toBeVisible({ timeout: 15_000 });
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
}

export async function openQuickChatWithAgent(page: Page, navigateHome = true): Promise<Locator> {
  const dialog = await openQuickChatSetup(page, navigateHome);
  await startQuickChatFromSetup(dialog, page);
  return dialog;
}

export async function sendQuickChatMessage(dialog: Locator, page: Page, text: string) {
  const editor = dialog.locator(".tiptap.ProseMirror");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  // With eager init, the agent boots during picker -> tab transition and the
  // input can briefly toggle disabled while the FE store catches up. Retry the
  // full edit action so fill() cannot race a contenteditable=false flip.
  await expect(async () => {
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 1_000 });
    await editor.click({ timeout: 1_000 });
    await editor.fill(text, { timeout: 1_000 });
    await expect(editor).toHaveText(text, { timeout: 1_000 });
    await editor.press(`${modifier}+Enter`, { timeout: 1_000 });
    await expect(editor).toHaveText("", { timeout: 2_000 });
  }).toPass({ timeout: 30_000, intervals: [250, 500, 1_000] });
}
