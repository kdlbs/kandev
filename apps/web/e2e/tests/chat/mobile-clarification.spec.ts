import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

/**
 * Mobile parity for the multiline custom clarification answer. On a coarse-pointer
 * device Enter inserts a newline instead of submitting, and the send affordance is
 * the inline "Send" button (there is no overlay Submit button for single-question
 * bundles). Runs under the Pixel 5 `mobile-chrome` project.
 */
async function seedClarificationTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:clarification",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return session;
}

test.describe("Mobile clarification multiline answer", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("Enter inserts a newline and the Send button submits the multiline answer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(testPage, apiClient, seedData, "Mobile Clarify");

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const input = session.clarificationInput();
    await input.click();
    await input.pressSequentially("first line");
    // On touch, Enter inserts a newline rather than submitting.
    await input.press("Enter");
    await input.pressSequentially("second line");
    await expect(input).toHaveValue("first line\nsecond line");
    // The overlay is still open — Enter did not submit.
    await expect(session.clarificationOverlay()).toBeVisible();

    // The inline Send button is the touch send affordance.
    await expect(session.clarificationCustomSubmit()).toBeVisible();
    await session.clarificationCustomSubmit().tap();

    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("first line");
    await expect(session.chat).toContainText("second line");
    await expect(session.chat).not.toContainText("linesecond line");
  });
});
