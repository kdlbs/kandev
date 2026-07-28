import { test, expect } from "../../fixtures/test-base";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile coarse RUNNING busy signal", () => {
  test.describe.configure({ retries: 1 });

  test("held-open background turn remains busy across input and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile coarse busy signal",
      seedData.agentProfileId,
      {
        description: "/background 30s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.agentStatus()).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByText("Kicking off background work")).toBeVisible({
      timeout: 20_000,
    });
    await testPage.waitForTimeout(500);

    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).not.toBeVisible();
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "queue this mobile follow-up");
    await testPage.getByTestId("submit-message-button").tap();
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).not.toBeVisible();
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();
  });
});
