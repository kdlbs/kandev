import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const NAVIGATION_TITLE = "PR summary navigation";
const TARGET_TITLE = "Inactive PR summary target";

test.describe("inactive task PR summary hydration", () => {
  test("loads full PR details on hover and reuses the settled task cache", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);

    const stepOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const navigationTask = await apiClient.seedTask(
      seedData.workspaceId,
      NAVIGATION_TITLE,
      stepOptions,
    );
    const targetTask = await apiClient.seedTask(seedData.workspaceId, TARGET_TITLE, {
      ...stepOptions,
      state: "IN_PROGRESS",
    });
    await apiClient.seedTaskSession(navigationTask.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedTaskSession(targetTask.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });

    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: targetTask.task_id,
      owner: "kandev-e2e",
      repo: "sidebar-summary-fixtures",
      pr_number: 44,
      pr_url: "https://github.test/kandev-e2e/sidebar-summary-fixtures/pull/44",
      pr_title: "Hydrate the inactive task summary",
      head_branch: "feature/sidebar-summary",
      base_branch: "main",
      author_login: "persisted-author",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
      required_reviews: 1,
      review_count: 1,
      checks_total: 1,
      checks_passing: 1,
    });

    let taskDetailRequests = 0;
    testPage.on("request", (request) => {
      if (
        request.url().includes("/api/v1/github/task-prs?") &&
        request.url().includes(`task_ids=${targetTask.task_id}`)
      ) {
        taskDetailRequests += 1;
      }
    });

    await testPage.goto(`/t/${navigationTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const targetRow = session.sidebarTaskItem(TARGET_TITLE);
    await expect(targetRow).toBeVisible({ timeout: 15_000 });
    const icon = targetRow.getByTestId(`pr-task-icon-${targetTask.task_id}`);
    await expect(icon).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveAttribute("data-pr-state", "Open");
    await expect(icon).toHaveAttribute("data-pr-count", "1");
    await expect(icon).not.toHaveAttribute("data-pr-ready-to-merge");
    expect(taskDetailRequests).toBe(0);

    await icon.hover();
    await expect.poll(() => taskDetailRequests).toBe(1);

    const summary = testPage
      .locator(
        '[data-slot="tooltip-content"]:not([data-state="closed"]) > [data-testid="pr-task-status-summary"]',
      )
      .first();
    await expect(summary).toBeVisible();
    await expect(summary.getByTestId("pr-task-status-number")).toHaveText("PR #44");
    await expect(summary.getByTestId("pr-task-status-title")).toHaveText(
      "Hydrate the inactive task summary",
    );
    await expect(summary.getByTestId("pr-task-status-title-author")).toHaveText(
      "by persisted-author",
    );
    await prCapture.screenshot("desktop-pr-sidebar-hover-hydration", {
      caption: "Desktop task sidebar PR summary with persisted author identity",
    });

    await testPage.mouse.move(0, 0);
    await expect(summary).toBeHidden();
    await icon.hover();
    await expect(summary).toBeVisible();
    await expect.poll(() => taskDetailRequests).toBe(1);
  });
});
