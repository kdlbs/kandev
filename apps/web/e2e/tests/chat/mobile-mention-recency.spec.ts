import { type Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { waitForPromptInStore } from "../../helpers/settings-prompt-editor";
import { CHAT_MENTION_RECENCY_STORAGE_KEY } from "../../../lib/chat-mention-recency";

const RECENT_PROMPT_NAME = "Project-Mention-Recency-Prompt";
const STRONGER_PROMPT_NAME = "Mention-Recency-Strong-Prompt";

async function openReadyTask(page: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Mention Recency Active Task",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Mobile chat mention recency", () => {
  test.describe.configure({ timeout: 90_000 });

  // @covers AC-UI-COMPOSER-MENTION-RECENCY-001.1
  // @covers AC-UI-COMPOSER-MENTION-RECENCY-001.4
  // @covers AC-UI-COMPOSER-MENTION-RECENCY-001.8
  test("touch selection uses the shared recency order in the mobile popup", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const recentPrompt = await apiClient.createPrompt(
      RECENT_PROMPT_NAME,
      "Mobile recent mention prompt",
    );
    const strongerPrompt = await apiClient.createPrompt(
      STRONGER_PROMPT_NAME,
      "Mobile stronger mention prompt",
    );

    try {
      await testPage.goto("/");
      await testPage.evaluate(
        (key) => window.localStorage.removeItem(key),
        CHAT_MENTION_RECENCY_STORAGE_KEY,
      );
      const session = await openReadyTask(testPage, apiClient, seedData);
      await waitForPromptInStore(testPage, RECENT_PROMPT_NAME);
      await waitForPromptInStore(testPage, STRONGER_PROMPT_NAME);
      const editor = await session.composerReady();

      await editor.tap();
      await editor.fill("");
      await editor.pressSequentially("@mention");
      const baselineMenu = testPage.getByRole("listbox", {
        name: /Mention tasks, files, prompts/i,
      });
      await expect(baselineMenu).toBeVisible({ timeout: 10_000 });
      await expect(baselineMenu.getByRole("option").first()).toContainText(STRONGER_PROMPT_NAME);
      await editor.press("Escape");

      await editor.fill("");
      await editor.pressSequentially("@Project-Mention");
      const uniqueMenu = testPage.getByRole("listbox", {
        name: /Mention tasks, files, prompts/i,
      });
      await expect(uniqueMenu).toBeVisible({ timeout: 10_000 });
      await uniqueMenu.getByRole("option").filter({ hasText: RECENT_PROMPT_NAME }).tap();
      await expect(uniqueMenu).toHaveCount(0);

      await editor.fill("");
      await editor.pressSequentially("@mention");
      const rankedMenu = testPage.getByRole("listbox", {
        name: /Mention tasks, files, prompts/i,
      });
      await expect(rankedMenu).toBeVisible({ timeout: 10_000 });
      await expect(rankedMenu.getByRole("option").first()).toContainText(RECENT_PROMPT_NAME);
      await prCapture.screenshot("mobile-mention-recency-menu", {
        caption: "A touch-selected prompt leads the mobile mention popup",
      });
    } finally {
      await apiClient.deletePrompt(recentPrompt.id).catch(() => undefined);
      await apiClient.deletePrompt(strongerPrompt.id).catch(() => undefined);
    }
  });
});
