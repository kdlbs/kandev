import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { seedGitLabReview, GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const MR_IID = 321;

async function seedTaskWithLinkedMR(apiClient: ApiClient, seedData: SeedData, title: string) {
  await seedGitLabReview(apiClient, seedData.workspaceId, MR_IID, "Mobile popover MR");
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
    task_id: task.id,
    repository_id: seedData.repositoryId,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
  });
  return task.id;
}

test.describe("mobile GitLab MR topbar hover popover", () => {
  test("AC26: no hover popover element is ever rendered; the click-to-dropdown flow is unchanged", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR popover mobile");

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const trigger = testPage.getByTestId("mr-topbar-button");
    await expect(trigger).toBeVisible({ timeout: 15_000 });

    // Touch has no hover; assert the popover never appears in the DOM at all,
    // not just that it is hidden.
    await expect(testPage.getByTestId("mr-topbar-popover")).toHaveCount(0);

    await trigger.tap();
    await expect(testPage.getByTestId("mr-automation-controls")).toBeVisible();
    await expect(testPage.getByTestId("mr-topbar-popover")).toHaveCount(0);
  });
});
