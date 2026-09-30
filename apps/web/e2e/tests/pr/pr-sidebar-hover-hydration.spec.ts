import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const NAVIGATION_TITLE = "PR summary navigation";
const TARGET_TITLE = "Inactive PR summary target";

async function expectVisibleTooltipInsideViewport(page: Page) {
  const tooltip = page
    .locator('[data-slot="tooltip-content"]:not([data-state="closed"]):visible')
    .first();
  await expect(tooltip).toBeVisible();
  const viewport = page.viewportSize();
  const box = await tooltip.boundingBox();
  expect(viewport).not.toBeNull();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height);
  return tooltip;
}

test.describe("inactive task PR summary hydration", () => {
  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.21
  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.22
  test("keeps a long PR summary inside the desktop viewport and scrollable", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 1280, height: 420 });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const stepOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const navigationTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Long PR summary navigation",
      stepOptions,
    );
    const targetTask = await apiClient.seedTask(seedData.workspaceId, "Long PR summary target", {
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

    for (let index = 1; index <= 5; index += 1) {
      const number = 50 + index;
      await apiClient.mockGitHubAssociateTaskPR({
        workspace_id: seedData.workspaceId,
        repository_id: seedData.repositoryId,
        task_id: targetTask.task_id,
        owner: "kandev-e2e",
        repo: "sidebar-summary-fixtures",
        pr_number: number,
        pr_url: `https://github.test/kandev-e2e/sidebar-summary-fixtures/pull/${number}`,
        pr_title: `Sidebar summary overflow fixture ${number}: a deliberately long pull request title that wraps across multiple lines in the narrow task status disclosure`,
        head_branch: `feature/sidebar-summary-${number}`,
        base_branch: "main",
        author_login: "test-user",
        state: "open",
        review_state: "approved",
        checks_state: "success",
        mergeable_state: "clean",
      });
    }
    await apiClient.updateTaskCIAutomationOptions(targetTask.task_id, {
      repository_id: seedData.repositoryId,
      pr_number: 51,
      auto_fix_enabled: true,
      auto_merge_enabled: true,
    });

    await testPage.goto(`/t/${navigationTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const targetRow = session.sidebarTaskItem("Long PR summary target");
    const icon = targetRow.getByTestId(`pr-task-icon-${targetTask.task_id}`);
    await icon.hover();

    const tooltip = await expectVisibleTooltipInsideViewport(testPage);
    const summary = tooltip.getByTestId("pr-task-status-summary").first();
    await expect(summary.getByTestId("pr-task-status-entry")).toHaveCount(5);
    const scrollBody = tooltip.getByTestId("pr-task-summary-scroll-body");
    const lastEntry = summary.getByTestId("pr-task-status-entry").last();
    const automation = tooltip.getByTestId("pr-task-automation-details");

    await scrollBody.hover();
    const initialScrollTop = await scrollBody.evaluate((element) => element.scrollTop);
    await testPage.mouse.wheel(0, 1800);
    await expect
      .poll(() => scrollBody.evaluate((element) => element.scrollTop))
      .toBeGreaterThan(initialScrollTop);
    await testPage.mouse.wheel(0, 2600);
    await expect
      .poll(() =>
        scrollBody.evaluate(
          (element) => element.scrollTop + element.clientHeight >= element.scrollHeight - 1,
        ),
      )
      .toBe(true);
    await expect(lastEntry).toBeInViewport({ ratio: 0.5 });
    await expect(automation).toBeInViewport({ ratio: 0.5 });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await prCapture.screenshot("desktop-pr-sidebar-overflow-scrolled", {
      caption: "Desktop task sidebar scrolls long PR summaries and automation details internally",
    });

    await testPage.mouse.move(0, 0);
    await expect(tooltip).toBeHidden();
    await targetRow.focus();
    await testPage.keyboard.press("Tab");
    await expect(icon).toBeFocused();
    const keyboardTooltip = await expectVisibleTooltipInsideViewport(testPage);
    const keyboardBody = keyboardTooltip.getByTestId("pr-task-summary-scroll-body");
    await testPage.keyboard.press("Tab");
    await expect(keyboardBody).toBeFocused();
    await keyboardBody.evaluate((element) => {
      element.scrollTop = 0;
    });
    await testPage.keyboard.press("PageDown");
    await expect
      .poll(() => keyboardBody.evaluate((element) => element.scrollTop))
      .toBeGreaterThan(0);
    await testPage.keyboard.press("Escape");
    await expect(keyboardTooltip).toBeHidden();
    await expect(icon).toBeFocused();

    await testPage.setViewportSize({ width: 767, height: 600 });
    await testPage.getByTestId("mobile-task-picker-trigger").click();
    const picker = testPage.getByRole("dialog", { name: "Tasks" });
    const narrowRow = picker.locator(`[data-task-row-id="${targetTask.task_id}"]`);
    const narrowIcon = narrowRow.getByTestId(`pr-task-icon-${targetTask.task_id}`);
    await narrowIcon.hover();
    await expectVisibleTooltipInsideViewport(testPage);
    await testPage.setViewportSize({ width: 768, height: 600 });
    await testPage.reload();
    await session.waitForLoad();
    const wideIcon = testPage
      .locator(`[data-testid="pr-task-icon-${targetTask.task_id}"]:visible`)
      .first();
    await expect(wideIcon).toBeVisible();
    await wideIcon.hover();
    await expectVisibleTooltipInsideViewport(testPage);
  });

  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.21
  test("keeps pending CI yellow across reload and full PR hydration", async ({
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
      checks_state: "pending",
      mergeable_state: "blocked",
      required_reviews: 1,
      review_count: 1,
      checks_total: 2,
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
    await expect(icon).toHaveClass(/text-yellow-500/);
    expect(taskDetailRequests).toBe(0);

    await testPage.reload();
    await session.waitForLoad();
    await expect(targetRow).toBeVisible({ timeout: 15_000 });
    await expect(icon).toHaveClass(/text-yellow-500/);
    expect(taskDetailRequests).toBe(0);
    await prCapture.screenshot("desktop-pr-sidebar-pending-ci-reloaded", {
      caption: "Desktop sidebar keeps pending CI yellow after reload",
    });

    await icon.hover();
    await expect.poll(() => taskDetailRequests).toBe(1);
    await expect(icon).toHaveClass(/text-yellow-500/);

    const summary = testPage
      .locator(
        '[data-slot="tooltip-content"]:not([data-state="closed"]) [data-testid="pr-task-status-summary"]',
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
    const shortScrollBody = testPage.getByTestId("pr-task-summary-scroll-body").first();
    await expect(shortScrollBody).toBeVisible();
    expect(
      await shortScrollBody.evaluate((element) => element.scrollHeight <= element.clientHeight),
    ).toBe(true);
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
