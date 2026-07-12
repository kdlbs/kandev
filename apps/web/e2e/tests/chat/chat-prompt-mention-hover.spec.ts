import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const PROMPT_NAME = "e2e-hover-template";
const PROMPT_CONTENT = "Reproduce the bug, isolate the cause, fix with a regression test.";
const CHIP = "custom-prompt-mention";

/**
 * Create a task whose initial user message is a bare @mention of the seeded
 * prompt. The description is persisted verbatim as the first user message, so
 * the chat renders the prompt-mention chip once the prompts slice loads.
 */
async function createTaskMentioningPrompt(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<string> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Prompt mention hover",
    seedData.agentProfileId,
    {
      description: `@${PROMPT_NAME}`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  return task.id;
}

async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

test.describe("Chat prompt-mention hover", () => {
  test.afterEach(async ({ apiClient }) => {
    const { prompts } = await apiClient.listPrompts();
    for (const p of prompts) {
      if (!p.builtin && p.name === PROMPT_NAME) {
        await apiClient.deletePrompt(p.id).catch(() => undefined);
      }
    }
  });

  test("hovering a prompt chip reveals the prompt contents", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.createPrompt(PROMPT_NAME, PROMPT_CONTENT);
    const taskId = await createTaskMentioningPrompt(apiClient, seedData);
    const session = await openTaskChat(testPage, taskId);

    // The mention chip renders once the prompts slice has loaded on the
    // session page (via useCustomPrompts) and matched the persisted @mention.
    const chip = session.activeChat().getByTestId(CHIP).first();
    await expect(chip).toBeVisible({ timeout: 30_000 });
    await expect(chip).toHaveAttribute("data-prompt-name", PROMPT_NAME);

    // With content loaded, the chip is wired as a Radix hover-card trigger.
    await expect(chip).toHaveAttribute("data-slot", "hover-card-trigger", {
      timeout: 15_000,
    });

    // Hover reveals the PromptPreview popover with the prompt body.
    await chip.hover();
    await expect(testPage.getByText(PROMPT_CONTENT, { exact: false })).toBeVisible({
      timeout: 10_000,
    });
  });
});
