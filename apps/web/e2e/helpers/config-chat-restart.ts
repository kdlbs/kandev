import { type Locator, type Page, expect } from "@playwright/test";

export async function openConfigurationChat(page: Page, touch = false): Promise<Locator> {
  await page.goto("/settings/agents");
  const launcher = page.getByRole("button", { name: "Configuration Chat", exact: true });
  if (touch) await launcher.tap();
  else await launcher.click();
  const panel = page.getByTestId("config-chat-popover");
  await expect(panel).toBeVisible();
  return panel;
}

export async function confirmConfigurationChatRestart(page: Page, panel: Locator, touch = false) {
  const restart = panel.getByRole("button", { name: "Restart session", exact: true });
  if (touch) await restart.tap();
  else await restart.click();
  const confirmation = page.getByTestId("config-chat-restart-confirmation");
  await expect(confirmation).toBeVisible();
  await expect(confirmation).toContainText("Configuration changes already made are kept.");
  const response = page.waitForResponse(
    (item) => item.request().method() === "POST" && item.url().endsWith("/config-chat/restart"),
  );
  const confirm = confirmation.getByTestId("config-chat-confirm-restart");
  if (touch) await confirm.tap();
  else await confirm.click();
  const result = await response;
  expect(result.status(), await result.text()).toBe(200);
  return result.json() as Promise<{
    task_id: string;
    session_id: string;
    agent_profile_id: string;
  }>;
}

export async function sendConfigurationPrompt(panel: Locator, prompt: string, touch = false) {
  const editor = panel.getByTestId("chat-input-editor");
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
  await editor.fill(prompt);
  if (touch) await panel.getByTestId("submit-message-button").tap();
  else await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);
}
