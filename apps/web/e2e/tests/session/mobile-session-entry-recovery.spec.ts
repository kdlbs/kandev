import { expect, test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { openTaskSession } from "../../helpers/session";
import { routeSessionEntryRecovery } from "../../helpers/session-entry-recovery";

async function createEntryTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

test.describe("mobile session entry recovery", () => {
  test.describe.configure({ retries: 1 });

  test("keeps cached access while history recovers with phone-sized controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const proxy = await routeSessionEntryRecovery(testPage);
    const task = await createEntryTask(
      apiClient,
      seedData,
      `Mobile history recovery ${Date.now()}`,
    );

    if (!task.session_id) throw new Error("created recovery task has no session");
    proxy.dropNextResponses("message.list", 2, { sessionId: task.session_id });

    const session = await openTaskSession(testPage, task.id);
    const chat = session.activeChat();
    const historyNotice = chat.getByTestId("session-history-unavailable");
    await expect(historyNotice).toBeVisible({ timeout: 45_000 });

    const retry = historyNotice.getByTestId("session-history-retry");
    const retryBox = await retry.boundingBox();
    expect(retryBox?.height).toBeGreaterThanOrEqual(44);
    const detailsSummary = historyNotice.getByTestId("session-history-details-summary");
    const detailsBox = await detailsSummary.boundingBox();
    expect(detailsBox?.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile session history recovery");

    await retry.click();
    await expect(historyNotice).toHaveCount(0);
    await expect(chat).toContainText("simple mock response", { timeout: 30_000 });
    expect(proxy.droppedResponseCount("message.list")).toBe(2);
  });
});
