import { type Locator, type Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { startQuickChatFromSetup, waitForSessionSettledBaseline } from "./quick-chat-helpers";

export const SAVED_PROMPT_NAME = "e2e-quick-chat-saved-prompt";
export const SAVED_PROMPT_DELIVERY_RESPONSE = "SAVED_PROMPT_DELIVERED";
export const SAVED_PROMPT_CONTENT = `e2e:saved_prompt_delivery("${SAVED_PROMPT_DELIVERY_RESPONSE}")`;

type SessionMessage = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];

type SavedPromptDeliveryScenarioOptions = {
  page: Page;
  apiClient: ApiClient;
  openSetup: (page: Page) => Promise<Locator>;
  activateEditor: (editor: Locator) => Promise<void>;
  selectPrompt: (option: Locator) => Promise<void>;
  submitMessage: (button: Locator) => Promise<void>;
};

export async function openMobileQuickChatSetup(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  await page.getByTestId("mobile-quick-chat-button").tap();

  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  const setup = dialog.getByTestId("quick-chat-setup");
  if (!(await setup.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByTestId("quick-chat-add-menu-trigger").tap();
    await page.getByTestId("quick-chat-new-agent").tap();
  }
  await expect(setup).toBeVisible({ timeout: 5_000 });
  return dialog;
}

async function waitForSavedPromptResponse(
  apiClient: ApiClient,
  sessionId: string,
): Promise<{ user: SessionMessage; agent: SessionMessage }> {
  let user: SessionMessage | undefined;
  let agent: SessionMessage | undefined;
  await expect
    .poll(
      async () => {
        const result = await apiClient.listSessionMessages(sessionId);
        user = result.messages.find(
          (message) =>
            message.author_type === "user" && message.content.includes(`@${SAVED_PROMPT_NAME}`),
        );
        agent = result.messages.find(
          (message) =>
            message.author_type === "agent" &&
            message.content.includes(SAVED_PROMPT_DELIVERY_RESPONSE),
        );
        return Boolean(user && agent);
      },
      {
        timeout: 30_000,
        message: `saved prompt delivery did not complete on session ${sessionId}`,
      },
    )
    .toBe(true);

  return { user: user!, agent: agent! };
}

export async function runSavedPromptDeliveryScenario({
  page,
  apiClient,
  openSetup,
  activateEditor,
  selectPrompt,
  submitMessage,
}: SavedPromptDeliveryScenarioOptions): Promise<void> {
  const prompt = await apiClient.createPrompt(SAVED_PROMPT_NAME, SAVED_PROMPT_CONTENT);

  try {
    const startResponse = page.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    const dialog = await openSetup(page);
    await startQuickChatFromSetup(dialog, page);
    const started = (await (await startResponse).json()) as {
      task_id: string;
      session_id: string;
    };

    await waitForSessionSettledBaseline(apiClient, started.task_id, started.session_id);

    const editor = dialog.locator(".tiptap.ProseMirror");
    await activateEditor(editor);
    await editor.fill("");
    await editor.pressSequentially(`Please run @${SAVED_PROMPT_NAME}`);

    const menu = page.getByRole("listbox", { name: /Mention tasks, files, prompts/i });
    const option = menu.getByRole("option").filter({ hasText: SAVED_PROMPT_NAME });
    await expect(menu).toBeVisible({ timeout: 10_000 });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await selectPrompt(option);
    await expect(menu).toHaveCount(0);
    await expect(editor).toContainText(SAVED_PROMPT_NAME);

    await submitMessage(dialog.getByTestId("submit-message-button"));
    await expect(editor).toHaveText("");

    const delivered = await waitForSavedPromptResponse(apiClient, started.session_id);
    expect(delivered.user.content).toContain(`@${SAVED_PROMPT_NAME}`);

    const raw = delivered.user.raw_content ?? "";
    expect(raw).toContain("<kandev-system>EXPANDED PROMPT REFERENCES:");
    expect(raw).toContain(SAVED_PROMPT_CONTENT);
    expect(raw).not.toContain("CONTEXT PROMPTS:");
    expect((raw.match(/<kandev-system>EXPANDED PROMPT REFERENCES:/g) ?? []).length).toBe(1);
    expect(delivered.agent.content).toContain(SAVED_PROMPT_DELIVERY_RESPONSE);
  } finally {
    await apiClient.deletePrompt(prompt.id).catch(() => undefined);
  }
}
