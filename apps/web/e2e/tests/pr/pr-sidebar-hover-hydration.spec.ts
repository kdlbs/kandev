import type { Page, Response } from "@playwright/test";
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { installNewerNegativePRProjectionFixture } from "../../helpers/pr-negative-projection-fixture";

const NAVIGATION_TITLE = "PR summary navigation";
const TARGET_TITLE = "Inactive PR summary target";

test("shows pending automation settings before omitting disabled actions", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const navigation = await apiClient.seedTask(seedData.workspaceId, NAVIGATION_TITLE, stepOptions);
  const target = await apiClient.seedTask(seedData.workspaceId, TARGET_TITLE, stepOptions);
  await apiClient.mockGitHubAssociateTaskPR({
    workspace_id: seedData.workspaceId,
    repository_id: seedData.repositoryId,
    task_id: target.task_id,
    owner: "kandev-e2e",
    repo: "sidebar-loading",
    pr_number: 66,
    pr_url: "https://github.test/kandev-e2e/sidebar-loading/pull/66",
    pr_title: "PR with disabled automation",
    head_branch: "feature/sidebar-loading",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
  });
  await testPage.goto(`/t/${navigation.task_id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  let releaseOptions!: () => void;
  const pendingOptions = new Promise<void>((resolve) => {
    releaseOptions = resolve;
  });
  await testPage.route(`**/api/v1/github/tasks/${target.task_id}/ci-options`, async (route) => {
    const response = await route.fetch();
    await pendingOptions;
    await route.fulfill({ response });
  });
  try {
    await session
      .sidebarTaskItem(TARGET_TITLE)
      .getByTestId(`pr-task-icon-${target.task_id}`)
      .hover();
    const automation = testPage.getByTestId("pr-task-automation-details");
    await expect(automation).toContainText("Loading pull request details");
    await prCapture.screenshot("desktop-pr-automation-loading", {
      caption:
        "Desktop PR summary shows pending automation settings before hiding disabled actions.",
    });
    releaseOptions();
    await expect(testPage.getByTestId("pr-task-status-number")).toHaveText("PR #66");
    await expect(automation).toHaveCount(0);
  } finally {
    releaseOptions();
    await testPage.unrouteAll({ behavior: "wait" });
  }
});

type AutomationE2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      workspaces: { activeId: string | null };
      workspaceContextGeneration: number;
      taskPRs: {
        byTaskId: Record<
          string,
          Array<{ pr_number: number; repository_id?: string; state: string }>
        >;
        workspaceId?: string | null;
        workspaceContextGeneration?: number;
      };
      taskCIAutomation?: {
        byTaskId?: Record<
          string,
          {
            workspace_id?: string;
            pr_options?: Array<{
              pr_number: number;
              repository_id: string;
              auto_fix_enabled: boolean;
              auto_merge_enabled: boolean;
            }>;
          }
        >;
      };
    };
  };
};

// @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.26
// @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.28
test("ignores brief crossings of sidebar PR icons", async ({ testPage, apiClient, seedData }) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 1280, height: 720 });
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");

  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const navigationTask = await apiClient.seedTask(
    seedData.workspaceId,
    "PR hover delay navigation",
    stepOptions,
  );
  const crossedTask = await apiClient.seedTask(seedData.workspaceId, "PR hover delay crossed", {
    ...stepOptions,
    state: "IN_PROGRESS",
  });
  const deliberateTask = await apiClient.seedTask(
    seedData.workspaceId,
    "PR hover delay deliberate",
    { ...stepOptions, state: "IN_PROGRESS" },
  );
  for (const taskId of [navigationTask.task_id, crossedTask.task_id, deliberateTask.task_id]) {
    await apiClient.seedTaskSession(taskId, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
  }

  for (const [task, number] of [
    [crossedTask, 64],
    [deliberateTask, 65],
  ] as const) {
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      task_id: task.task_id,
      owner: "kandev-e2e",
      repo: "sidebar-hover-delay",
      pr_number: number,
      pr_url: `https://github.test/kandev-e2e/sidebar-hover-delay/pull/${number}`,
      pr_title: `Sidebar hover delay PR ${number}`,
      head_branch: `feature/sidebar-hover-delay-${number}`,
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
  }

  const requests = new Map<string, number>([
    [crossedTask.task_id, 0],
    [deliberateTask.task_id, 0],
  ]);
  testPage.on("request", (request) => {
    if (!request.url().includes("/api/v1/github/task-prs?")) return;
    for (const taskId of requests.keys()) {
      if (request.url().includes(`task_ids=${taskId}`)) {
        requests.set(taskId, (requests.get(taskId) ?? 0) + 1);
      }
    }
  });

  await testPage.goto(`/t/${navigationTask.task_id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const crossedIcon = session
    .sidebarTaskItem("PR hover delay crossed")
    .getByTestId(`pr-task-icon-${crossedTask.task_id}`);
  const deliberateIcon = session
    .sidebarTaskItem("PR hover delay deliberate")
    .getByTestId(`pr-task-icon-${deliberateTask.task_id}`);
  await expect(crossedIcon).toBeVisible();
  await expect(deliberateIcon).toBeVisible();

  await testPage.clock.install();
  await testPage.clock.pauseAt(await testPage.evaluate(() => Date.now() + 100));
  await crossedIcon.hover();
  await testPage.clock.runFor(200);
  expect(await testPage.getByTestId("pr-task-summary-scroll-body").count()).toBe(0);
  await deliberateIcon.hover();
  await testPage.clock.runFor(499);
  expect(await testPage.getByTestId("pr-task-summary-scroll-body").count()).toBe(0);
  expect(requests.get(crossedTask.task_id)).toBe(0);
  expect(requests.get(deliberateTask.task_id)).toBe(0);

  await testPage.clock.runFor(1);
  const summary = testPage.getByTestId("pr-task-status-summary");
  await expect(summary).toBeVisible();
  await expect(summary.getByTestId("pr-task-status-number")).toHaveText("PR #65");
  await expect.poll(() => requests.get(deliberateTask.task_id)).toBe(1);
  expect(requests.get(crossedTask.task_id)).toBe(0);
});

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

async function expectTaskAutomationHydrated({
  page,
  taskId,
  workspaceId,
  repositoryId,
  automationOptionsResponse,
  taskPRsResponse,
}: {
  page: Page;
  taskId: string;
  workspaceId: string;
  repositoryId: string;
  automationOptionsResponse: Promise<Response>;
  taskPRsResponse: Promise<Response>;
}) {
  const response = await automationOptionsResponse;
  expect(response.ok()).toBe(true);
  const options = await response.json();
  expect(options.workspace_id).toBe(workspaceId);
  expect(
    options.pr_options?.find((option: { pr_number: number }) => option.pr_number === 51),
  ).toEqual(expect.objectContaining({ auto_fix_enabled: true, auto_merge_enabled: false }));

  const prsResponse = await taskPRsResponse;
  const prs = (await prsResponse.json()) as {
    task_prs?: Record<string, Array<{ pr_number: number; repository_id?: string }>>;
  };
  expect(prs.task_prs?.[taskId]?.find((pr) => pr.pr_number === 51)?.repository_id).toBe(
    repositoryId,
  );

  await expect
    .poll(() =>
      page.evaluate((id) => {
        const state = (window as AutomationE2EStoreWindow).__KANDEV_E2E_STORE__?.getState();
        if (!state) return null;
        const pr = state.taskPRs.byTaskId[id]?.find((candidate) => candidate.pr_number === 51);
        const automation = state.taskCIAutomation?.byTaskId?.[id];
        const matchingAutomation = automation?.pr_options?.find(
          (option) =>
            option.pr_number === pr?.pr_number && option.repository_id === pr?.repository_id,
        );
        return {
          workspaceId: state.workspaces.activeId,
          taskPRWorkspaceId: state.taskPRs.workspaceId,
          currentGeneration:
            state.taskPRs.workspaceContextGeneration === state.workspaceContextGeneration,
          automationWorkspaceId: automation?.workspace_id,
          repositoryId: pr?.repository_id,
          matchingAutomation,
        };
      }, taskId),
    )
    .toMatchObject({
      workspaceId,
      taskPRWorkspaceId: workspaceId,
      currentGeneration: true,
      automationWorkspaceId: workspaceId,
      repositoryId,
      matchingAutomation: {
        repository_id: repositoryId,
        auto_fix_enabled: true,
        auto_merge_enabled: false,
      },
    });
}

test.describe("inactive task PR summary hydration", () => {
  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.21
  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.22
  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.24
  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.25
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
    const automationOptions = await apiClient.updateTaskCIAutomationOptions(targetTask.task_id, {
      repository_id: seedData.repositoryId,
      pr_number: 51,
      auto_fix_enabled: true,
      auto_merge_enabled: false,
    });
    expect(automationOptions.pr_options?.find((option) => option.pr_number === 51)).toEqual(
      expect.objectContaining({ auto_fix_enabled: true, auto_merge_enabled: false }),
    );
    const seededPRs = await apiClient.listTaskPRs(targetTask.task_id);
    expect(seededPRs.find((pr) => pr.pr_number === 51)).toEqual(
      expect.objectContaining({ repository_id: seedData.repositoryId, state: "open" }),
    );
    const automationOptionsResponse = testPage.waitForResponse((response) =>
      response.url().includes(`/api/v1/github/tasks/${targetTask.task_id}/ci-options`),
    );
    const taskPRsResponse = testPage.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/github/task-prs?") &&
        response.url().includes(`task_ids=${targetTask.task_id}`),
    );

    await testPage.goto(`/t/${navigationTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const targetRow = session.sidebarTaskItem("Long PR summary target");
    await expect(targetRow).toBeInViewport({ ratio: 0.5 });
    const icon = targetRow.getByTestId(`pr-task-icon-${targetTask.task_id}`);
    await icon.hover();

    await expectTaskAutomationHydrated({
      page: testPage,
      taskId: targetTask.task_id,
      workspaceId: seedData.workspaceId,
      repositoryId: seedData.repositoryId,
      automationOptionsResponse,
      taskPRsResponse,
    });
    await expect(icon).toHaveAttribute("aria-label", /auto-fix enabled/i);

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
    await expect(keyboardBody).not.toHaveCSS("box-shadow", "none");
    await prCapture.screenshot("desktop-pr-sidebar-overflow-keyboard-focus", {
      caption: "Desktop PR summary shows a visible focus indicator on the keyboard scroll region",
    });
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

  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.2/.3/.17/.24
  // @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4/.8
  test("retains merged PR details after a newer negative approval projection", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 1280, height: 720 });
    const stepOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const navigationTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Negative projection navigation",
      stepOptions,
    );
    const targetTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Merged negative projection target",
      { ...stepOptions, state: "IN_PROGRESS" },
    );
    await apiClient.seedTaskSession(navigationTask.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedTaskSession(targetTask.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      task_id: targetTask.task_id,
      owner: "kandev-e2e",
      repo: "sidebar-summary-fixtures",
      pr_number: 190,
      pr_url: "https://github.test/kandev-e2e/sidebar-summary-fixtures/pull/190",
      pr_title: "Merged negative approval fixture",
      head_branch: "feature/negative-approval",
      base_branch: "main",
      author_login: "negative-author",
      state: "merged",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });

    const assertProjectionTimestamps = await installNewerNegativePRProjectionFixture(
      testPage,
      targetTask.task_id,
      190,
    );
    let detailRequests = 0;
    testPage.on("request", (request) => {
      if (
        request.url().includes("/api/v1/github/task-prs?") &&
        request.url().includes(`task_ids=${targetTask.task_id}`)
      ) {
        detailRequests += 1;
      }
    });

    await testPage.goto(`/t/${navigationTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const targetRow = session.sidebarTaskItem("Merged negative projection target");
    const icon = targetRow.getByTestId(`pr-task-icon-${targetTask.task_id}`);
    await icon.hover();
    await expect.poll(() => detailRequests).toBe(1);
    await assertProjectionTimestamps();

    const tooltip = await expectVisibleTooltipInsideViewport(testPage);
    const summary = tooltip.getByTestId("pr-task-status-summary");
    await expect(summary.getByTestId("pr-task-status-number")).toHaveText("PR #190");
    await expect(summary.getByTestId("pr-task-status-title")).toHaveText(
      "Merged negative approval fixture",
    );
    await expect(summary.getByTestId("pr-task-status-title-author")).toHaveText(
      "by negative-author",
    );
    await expect(summary.getByTestId("pr-task-status-state-value")).toContainText("Merged");
    await expect(summary).not.toContainText("Awaiting maintainer approval");
    await prCapture.screenshot("desktop-pr-negative-projection-merged", {
      caption: "Desktop PR disclosure retains the merged PR after approval clears.",
    });

    await testPage.mouse.move(0, 0);
    await expect(tooltip).toBeHidden();
    await icon.focus();
    const keyboardTooltip = await expectVisibleTooltipInsideViewport(testPage);
    const keyboardSummary = keyboardTooltip.getByTestId("pr-task-status-summary");
    await expect(keyboardSummary.getByTestId("pr-task-status-title")).toHaveText(
      "Merged negative approval fixture",
    );
    expect(detailRequests).toBe(1);
  });
});
