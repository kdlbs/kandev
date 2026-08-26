import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const OWNER = "acme";
const REPO = "demo";
const PR_NUMBER = 144;
const PR_URL = `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`;

function manyRunningChecks(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    name: `Mobile lifecycle check ${index + 1}/${count} / run`,
    status: "in_progress",
    html_url: `https://example.com/checks/${index + 1}`,
  }));
}

async function seedTaskWithPR(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  prOverrides: Partial<Parameters<ApiClient["mockGitHubAssociateTaskPR"]>[0]> = {},
) {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
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
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    workspace_id: seedData.workspaceId,
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    pr_url: PR_URL,
    pr_title: "Add mobile CI automation options",
    head_branch: "feat/mobile-ci-automation",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    review_state: "approved",
    review_count: 1,
    checks_state: "failure",
    checks_total: 2,
    checks_passing: 1,
    ...prOverrides,
  });
  return task.id;
}

async function interceptLifecycleError(
  testPage: import("@playwright/test").Page,
  repositoryId: string,
) {
  await testPage.route("**/api/v1/github/tasks/*/ci-options", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    const options = (await response.json()) as { pr_states?: Array<Record<string, unknown>> };
    await route.fulfill({
      response,
      json: {
        ...options,
        pr_states: [
          ...(options.pr_states ?? []).filter(
            (state) =>
              (state.repository_id !== repositoryId && state.repository_id !== "") ||
              state.pr_number !== PR_NUMBER,
          ),
          {
            repository_id: repositoryId,
            pr_number: PR_NUMBER,
            last_error: "Lifecycle prompt could not be delivered to a task session.",
          },
          {
            repository_id: "",
            pr_number: PR_NUMBER,
            last_error: "Lifecycle prompt could not be delivered to a task session.",
          },
        ],
      },
    });
  });
}

async function interceptTallPRFeedback(testPage: import("@playwright/test").Page) {
  // Match on pathname: the feedback request carries a workspace_id query the
  // glob form would not match.
  await testPage.route(
    (url) => url.pathname === `/api/v1/github/prs/${OWNER}/${REPO}/${PR_NUMBER}`,
    async (route) => {
      await route.fulfill({ json: { checks: manyRunningChecks(30) } });
    },
  );
}

test.describe("mobile PR CI automation options", () => {
  test("drawer exposes automation controls and task prompt settings link", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI automation mobile");

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.prTopbarButton()).toHaveCount(0);
    await expect(session.prStatusChip()).toBeVisible({ timeout: 15_000 });
    await session.tapPRStatusChip();

    const drawer = session.prStatusChipDrawer();
    await expect(drawer.getByTestId("pr-ci-automation-controls")).toBeVisible();
    await expect(
      drawer.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).toBeVisible();
    await expect(
      drawer.getByRole("switch", { name: "Auto-merge or requeue when ready" }),
    ).toBeVisible();
    const reviewFollowUp = drawer.getByTestId("ci-review-follow-up-trigger");
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "false");
    await expect(reviewFollowUp).toHaveCSS("min-height", "44px");
    await reviewFollowUp.tap();
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "true");
    await expect(drawer.getByRole("switch", { name: "Your review is requested" })).toBeVisible();
    await expect(drawer.getByRole("switch", { name: "PR merged" })).toBeVisible();
    await expect(drawer.getByRole("switch", { name: "PR closed without merging" })).toBeVisible();
    const helpPopover = drawer.locator('[data-slot="popover-content"]');
    await drawer.getByTestId("ci-review-requested-help").tap();
    await expect(
      helpPopover.getByText(
        "Wake the agent for any new request, including re-review after changes.",
      ),
    ).toBeVisible();
    await testPage.keyboard.press("Escape");
    await drawer.getByTestId("ci-pr-terminal-help").tap();
    await expect(
      helpPopover.getByText(
        "Wake the agent when review work ends. Choose either or both outcomes.",
      ),
    ).toBeVisible();
    await testPage.keyboard.press("Escape");

    for (const name of ["Your review is requested", "PR merged", "PR closed without merging"]) {
      await expect(drawer.getByRole("switch", { name }).locator("..")).toHaveCSS(
        "min-height",
        "44px",
      );
    }

    await drawer.getByRole("switch", { name: "Auto-fix CI and address comments" }).tap();
    await drawer.getByRole("switch", { name: "Your review is requested" }).tap();
    await drawer.getByRole("switch", { name: "PR closed without merging" }).tap();
    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({
        auto_fix_enabled: true,
        prompt_on_review_requested: true,
        prompt_on_closed: true,
      });

    await drawer.getByLabel("Edit auto-fix prompt for this task").tap();
    const promptDialog = testPage.getByRole("dialog", { name: "Auto-fix prompt" });
    await expect(promptDialog).toBeVisible();
    await expect(testPage.getByRole("link", { name: "Edit default prompt" })).toHaveAttribute(
      "href",
      "/settings/prompts",
    );
    await expect(promptDialog.getByTestId("ci-auto-fix-pr-feedback-placeholder")).toHaveText(
      "{{pr.feedback}}",
    );
    await expect(promptDialog.getByTestId("ci-auto-fix-pr-feedback-help")).toContainText(
      "new or changed review comments",
    );
  });

  test("drawer keeps two linked PRs' automation switches independent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI automation mobile independence");
    const secondPRNumber = PR_NUMBER + 1;
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskId,
      workspace_id: seedData.workspaceId,
      owner: OWNER,
      repo: REPO,
      pr_number: secondPRNumber,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${secondPRNumber}`,
      pr_title: "Second PR",
      head_branch: "feat/second",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
    });

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.tapPRStatusChip();

    const drawer = session.prStatusChipDrawer();
    await drawer.getByRole("tab", { name: `${REPO} #${PR_NUMBER}` }).tap();
    await expect(
      drawer.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).toBeVisible();
    await drawer.getByRole("switch", { name: "Auto-fix CI and address comments" }).tap();

    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({
        pr_options: expect.arrayContaining([
          expect.objectContaining({ pr_number: PR_NUMBER, auto_fix_enabled: true }),
          expect.objectContaining({ pr_number: secondPRNumber, auto_fix_enabled: false }),
        ]),
      });

    await drawer.getByRole("tab", { name: `${REPO} #${secondPRNumber}` }).tap();
    await expect(
      drawer.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).not.toBeChecked();
  });

  test("drawer contains lifecycle errors without horizontal document overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskId = await seedTaskWithPR(
      apiClient,
      seedData,
      "CI automation mobile lifecycle error",
    );
    await interceptLifecycleError(testPage, seedData.repositoryId);
    await interceptTallPRFeedback(testPage);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.tapPRStatusChip();

    const drawer = session.prStatusChipDrawer();
    await drawer.getByTestId("ci-review-follow-up-trigger").tap();
    await expect(drawer.getByRole("switch", { name: "Your review is requested" })).toBeVisible();
    await expect(drawer.getByRole("alert")).toContainText(
      "Lifecycle prompt could not be delivered to a task session.",
    );
    await expect(drawer.getByTestId("pr-workflow-row")).toHaveCount(30);

    const scrollBody = drawer.locator("[data-vaul-no-drag]");
    await expect(scrollBody).toHaveCSS("overflow-y", "auto");
    const [drawerBox, scrollMetrics, documentMetrics] = await Promise.all([
      drawer.boundingBox(),
      scrollBody.evaluate((element) => ({
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      })),
      testPage.evaluate(() => ({
        clientWidth: document.documentElement.clientWidth,
        scrollWidth: document.documentElement.scrollWidth,
      })),
    ]);
    expect(drawerBox).not.toBeNull();
    expect(drawerBox!.x).toBeGreaterThanOrEqual(0);
    expect(drawerBox!.x + drawerBox!.width).toBeLessThanOrEqual(documentMetrics.clientWidth);
    expect(scrollMetrics.scrollHeight).toBeGreaterThan(scrollMetrics.clientHeight);
    expect(documentMetrics.scrollWidth).toBeLessThanOrEqual(documentMetrics.clientWidth);
  });

  test("mobile merge queue recovery proves repair and new-head requeue", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const queuedHead = "head-queued-mobile";
    const replacementHead = "head-replacement-mobile";
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI mobile merge queue recovery", {
      head_sha: queuedHead,
      checks_state: "success",
      checks_total: 1,
      checks_passing: 1,
      unresolved_review_threads: 0,
      mergeable_state: "clean",
      merge_queue_state: "queued",
      merge_queue_position: 1,
      merge_queue_entry_id: "entry-mobile-a",
      merge_queue_entry_head_sha: queuedHead,
    });
    await apiClient.mockGitHubSetMergeOutcome(OWNER, REPO, PR_NUMBER, "queued");
    await interceptTallPRFeedback(testPage);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.tapPRStatusChip();
    const drawer = session.prStatusChipDrawer();
    await expect(drawer.getByText("Merge queue automation")).toBeVisible();
    await expect(drawer.getByText("PR #144 is in the merge queue")).toBeVisible();
    await expect(drawer.getByTestId("ci-merge-queue-recovery-status")).toContainText(
      "Active merge queue attempt",
    );
    await expect(drawer.getByRole("switch")).toHaveCount(2);
    await expect(
      drawer.getByRole("switch", { name: "Auto-fix CI and address comments" }).locator(".."),
    ).toHaveCSS("min-height", "44px");
    await expect(
      drawer.getByRole("switch", { name: "Auto-merge or requeue when ready" }).locator(".."),
    ).toHaveCSS("min-height", "44px");

    await drawer.getByRole("switch", { name: "Auto-fix CI and address comments" }).tap();
    await drawer.getByRole("switch", { name: "Auto-merge or requeue when ready" }).tap();
    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({
        pr_options: expect.arrayContaining([
          expect.objectContaining({
            pr_number: PR_NUMBER,
            auto_fix_enabled: true,
            auto_merge_enabled: true,
          }),
        ]),
      });
    await expect.poll(() => apiClient.mockGitHubGetMergeAttempts()).toHaveLength(0);

    await apiClient.mockGitHubTransitionMergeQueue({
      task_id: taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      head_sha: queuedHead,
      merge_queue_state: "",
      merge_queue_entry_id: "",
      merge_queue_entry_head_sha: "",
      merge_queue_last_removal_id: "removal-mobile-a",
      merge_queue_last_removed_at: new Date().toISOString(),
      merge_queue_last_removal_reason: "checks failed on merge group",
      merge_queue_last_removal_before_sha: "merge-group-mobile-a",
      checks: [
        {
          name: "merge group checks",
          status: "completed",
          conclusion: "failure",
          html_url: "https://example.test/checks/merge-group-mobile",
        },
      ],
    });
    await expect
      .poll(async () => {
        const options = await apiClient.getTaskCIAutomationOptions(taskId);
        const state = options.pr_states?.find((item) => item.pr_number === PR_NUMBER);
        return {
          round: state?.auto_fix_round_count,
          event: state?.last_queue_fix_event_id,
          cause: state?.last_queue_removal_cause,
        };
      })
      .toEqual({ round: 1, event: "removal-mobile-a", cause: "checks_failed" });
    await expect(drawer.getByText("Merge queue recovery")).toBeVisible();
    await expect(drawer.getByText("PR #144 was removed: checks failed")).toBeVisible();
    await expect(drawer.getByTestId("ci-merge-queue-recovery-status")).toContainText(
      "Repair requested. Waiting for a new commit before requeue",
    );

    const scrollBody = drawer.locator("[data-vaul-no-drag]");
    const [drawerBox, scrollMetrics, documentMetrics] = await Promise.all([
      drawer.boundingBox(),
      scrollBody.evaluate((element) => ({
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      })),
      testPage.evaluate(() => ({
        clientWidth: document.documentElement.clientWidth,
        scrollWidth: document.documentElement.scrollWidth,
      })),
    ]);
    expect(drawerBox).not.toBeNull();
    expect(drawerBox!.x).toBeGreaterThanOrEqual(0);
    expect(drawerBox!.x + drawerBox!.width).toBeLessThanOrEqual(documentMetrics.clientWidth);
    expect(scrollMetrics.scrollHeight).toBeGreaterThan(scrollMetrics.clientHeight);
    expect(documentMetrics.scrollWidth).toBeLessThanOrEqual(documentMetrics.clientWidth);

    await apiClient.mockGitHubTransitionMergeQueue({
      task_id: taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      head_sha: queuedHead,
      merge_queue_state: "",
      merge_queue_entry_id: "",
      merge_queue_entry_head_sha: "",
      merge_queue_last_removal_id: "removal-mobile-a",
      merge_queue_last_removal_reason: "checks failed on merge group",
      merge_queue_last_removal_before_sha: "merge-group-mobile-a",
      checks: [
        {
          name: "merge group checks",
          status: "completed",
          conclusion: "success",
          html_url: "https://example.test/checks/merge-group-mobile",
        },
      ],
    });
    await expect.poll(() => apiClient.mockGitHubGetMergeAttempts()).toHaveLength(0);

    await apiClient.mockGitHubTransitionMergeQueue({
      task_id: taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      head_sha: replacementHead,
      merge_queue_state: "",
      merge_queue_entry_id: "",
      merge_queue_entry_head_sha: "",
      merge_queue_last_removal_id: "removal-mobile-a",
      merge_queue_last_removal_reason: "checks failed on merge group",
      merge_queue_last_removal_before_sha: "merge-group-mobile-a",
      checks: [
        {
          name: "merge group checks",
          status: "completed",
          conclusion: "success",
          html_url: "https://example.test/checks/merge-group-mobile",
        },
      ],
    });
    await expect.poll(() => apiClient.mockGitHubGetMergeAttempts()).toHaveLength(1);
    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({
        pr_states: expect.arrayContaining([
          expect.objectContaining({ last_queue_attempt_head_sha: replacementHead }),
        ]),
      });

    await apiClient.mockGitHubTransitionMergeQueue({
      task_id: taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      head_sha: replacementHead,
      merge_queue_state: "queued",
      merge_queue_position: 1,
      merge_queue_entry_id: "entry-mobile-b",
      merge_queue_entry_head_sha: replacementHead,
      checks: [
        {
          name: "merge group checks",
          status: "completed",
          conclusion: "success",
          html_url: "https://example.test/checks/merge-group-mobile",
        },
      ],
    });
    await expect(drawer.getByText("Merge queue automation")).toBeVisible();
    await expect(drawer.getByTestId("ci-merge-queue-recovery-status")).toContainText(
      "Active merge queue attempt",
    );
    await expect.poll(() => apiClient.mockGitHubGetMergeAttempts()).toHaveLength(1);
  });
});
