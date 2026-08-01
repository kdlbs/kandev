import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { seedGitLabReview, GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const MR_IID = 210;

async function seedTaskWithLinkedMR(apiClient: ApiClient, seedData: SeedData, title: string) {
  await seedGitLabReview(apiClient, seedData.workspaceId, MR_IID, "MR automation options MR");
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

async function openTask(testPage: import("@playwright/test").Page, taskId: string) {
  await testPage.goto(`/t/${taskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(testPage.getByTestId("mr-topbar-button")).toBeVisible({ timeout: 15_000 });
  return session;
}

async function openDropdown(testPage: import("@playwright/test").Page) {
  await testPage.getByTestId("mr-topbar-button").click();
  const content = testPage.getByTestId("mr-automation-controls");
  await expect(content).toBeVisible();
  return content;
}

async function interceptLoadFailure(testPage: import("@playwright/test").Page) {
  await testPage.route("**/api/v1/gitlab/tasks/*/mr-automation", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    await route.fulfill({ status: 500, json: { error: "backend unavailable" } });
  });
}

test.describe("GitLab MR automation options", () => {
  test("desktop dropdown persists lifecycle notification switches and survives reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR automation desktop");
    await openTask(testPage, taskId);
    const controls = await openDropdown(testPage);

    const reviewFollowUp = controls.getByTestId("mr-review-follow-up-trigger");
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "false");
    await reviewFollowUp.click();
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "true");
    await expect(controls.getByRole("switch", { name: "Your review is requested" })).toBeVisible();
    await expect(controls.getByRole("switch", { name: "MR merged" })).toBeVisible();
    await expect(controls.getByRole("switch", { name: "MR closed without merging" })).toBeVisible();

    await controls.getByTestId("mr-review-requested-help").hover();
    await expect(
      testPage
        .getByRole("tooltip")
        .getByText(
          "Wake the agent when you're added as a reviewer, including re-review after changes.",
        ),
    ).toBeVisible();
    await controls.getByTestId("mr-terminal-help").hover();
    await expect(
      testPage
        .getByRole("tooltip")
        .getByText("Wake the agent when review work ends. Choose either or both outcomes."),
    ).toBeVisible();

    await controls.getByRole("switch", { name: "Your review is requested" }).click();
    await controls.getByRole("switch", { name: "MR merged" }).click();

    await expect
      .poll(async () => apiClient.getTaskMRAutomationOptions(taskId))
      .toMatchObject({
        prompt_on_review_requested: true,
        prompt_on_merged: true,
      });

    await testPage.reload();
    await expect(testPage.getByTestId("mr-topbar-button")).toBeVisible({ timeout: 15_000 });
    const reloadedControls = await openDropdown(testPage);
    const reloadedTrigger = reloadedControls.getByTestId("mr-review-follow-up-trigger");
    // Auto-expands: a switch is already enabled.
    await expect(reloadedTrigger).toHaveAttribute("aria-expanded", "true");
    await expect(
      reloadedControls.getByRole("switch", { name: "Your review is requested" }),
    ).toBeChecked();
    await expect(reloadedControls.getByRole("switch", { name: "MR merged" })).toBeChecked();
  });

  test("desktop dropdown shows a retry banner when the initial load fails", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR automation load error");
    await interceptLoadFailure(testPage);
    await openTask(testPage, taskId);

    await testPage.getByTestId("mr-topbar-button").click();
    const controls = testPage.getByTestId("mr-automation-controls");
    await expect(controls).toBeVisible();
    // Visible without opening the collapsible section — the group never
    // auto-expands with no loaded options.
    await expect(controls.getByRole("alert")).toContainText("backend unavailable");
    const retry = controls.getByTestId("mr-automation-retry");
    await expect(retry).toBeVisible();

    await testPage.unroute("**/api/v1/gitlab/tasks/*/mr-automation");
    await retry.click();
    await expect(controls.getByRole("alert")).toHaveCount(0);
    const reviewFollowUp = controls.getByTestId("mr-review-follow-up-trigger");
    await reviewFollowUp.click();
    await expect(
      controls.getByRole("switch", { name: "Your review is requested" }),
    ).not.toBeChecked();
  });
});
