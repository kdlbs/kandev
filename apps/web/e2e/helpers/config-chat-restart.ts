import { type Locator, type Page, expect } from "@playwright/test";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/app-state-types";
import type { ApiClient } from "./api-client";

export async function waitForConfigurationResponse(
  apiClient: ApiClient,
  sessionId: string,
  content: string,
) {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => message.author_type === "agent" && message.content.includes(content),
        );
      },
      { timeout: 30_000 },
    )
    .toBe(true);
}

export async function openUncertainExpandedConfigChat(
  page: Page,
  workspaceId: string,
  sessionId: string,
  touch = false,
) {
  const panel = await openConfigurationChat(page, touch);
  const expand = panel.getByRole("button", { name: "Open in Quick Chat" });
  if (touch) await expand.tap();
  else await expand.click();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible();
  await page.evaluate(
    ({ workspaceId, sessionId }) => {
      (window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> }).__KANDEV_E2E_STORE__
        .getState()
        .setConfigChatRestart(workspaceId, { sessionId, source: "local", status: "uncertain" });
    },
    { workspaceId, sessionId },
  );
  await expect(dialog.getByText(/Could not check whether restart finished\./)).toBeVisible();
  return dialog;
}

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

export async function sendConfigurationPrompt(
  panel: Locator,
  prompt: string,
  apiClient: ApiClient,
  sessionId: string,
  touch = false,
) {
  await expect
    .poll(async () => (await apiClient.getTaskSession(sessionId)).session.state, {
      timeout: 30_000,
    })
    .toBe("WAITING_FOR_INPUT");
  const editor = panel.getByTestId("chat-input-editor");
  await expect(editor).toHaveAttribute("contenteditable", "true");
  await editor.fill(prompt);
  if (touch) await panel.getByTestId("submit-message-button").tap();
  else await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);
}
