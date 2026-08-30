import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const OWNER = "acme";
const REPO = "demo";
const PR_NUMBER = 144;
const PR_URL = `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`;

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
    repository_id: seedData.repositoryId,
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    pr_url: PR_URL,
    pr_title: "Add CI automation options",
    head_branch: "feat/ci-automation",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    review_state: "approved",
    review_count: 1,
    checks_state: "failure",
    checks_total: 3,
    checks_passing: 2,
    unresolved_review_threads: 1,
    ...prOverrides,
  });
  return task.id;
}

async function openTask(testPage: import("@playwright/test").Page, taskId: string) {
  await testPage.goto(`/t/${taskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
  await session.hoverPRTopbar();
  await session.prTopbarPopover().hover();
  return session;
}

async function openPromptDialog(session: SessionPage) {
  await session.hoverPRTopbar();
  const popover = session.prTopbarPopover();
  await popover.hover();
  const editButton = popover.getByLabel("Edit auto-fix prompt for this task");
  await expect(editButton).toBeVisible();
  await editButton.click({ force: true });
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

test.describe("PR CI automation options", () => {
  test("composer tray groups PR event automations in a narrow window", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 820, height: 800 });
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI automation composer tray", {
      checks_state: "pending",
      checks_passing: 0,
      review_state: "pending",
      review_count: 0,
      pending_review_count: 1,
      unresolved_review_threads: 0,
    });
    await apiClient.updateTaskCIAutomationOptions(taskId, {
      auto_fix_enabled: true,
      auto_merge_enabled: true,
      prompt_on_review_requested: true,
      prompt_on_merged: true,
      prompt_on_closed: true,
    });

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const chip = session.prStatusChip();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    await expect(chip.getByTestId("pr-status-auto-fix-chip")).toContainText("Auto-fix 0/10");
    await expect(chip.getByTestId("pr-status-auto-merge-chip")).toHaveText("Auto-merge");
    const prEvents = chip.getByTestId("pr-status-pr-events-chip");
    await expect(prEvents).toHaveText("PR events 3/3");
    await expect(prEvents).toHaveAttribute("data-pr-events-count", "3");
    await expect(chip).toHaveAttribute("aria-label", /Your review is requested/);
    await expect(chip).toHaveAttribute("aria-label", /PR merged/);
    await expect(chip).toHaveAttribute("aria-label", /PR closed without merging/);

    const statusBar = session.activeChat().getByTestId("chat-status-bar");
    await expect(statusBar).toHaveCSS("flex-wrap", "wrap");
    expect(
      await statusBar.evaluate((element) => {
        const bar = element.getBoundingClientRect();
        return Array.from(element.children).every((child) => {
          const rect = child.getBoundingClientRect();
          return rect.left >= bar.left - 1 && rect.right <= bar.right + 1;
        });
      }),
    ).toBe(true);
    expect(
      await chip.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        const hit = document.elementFromPoint(
          rect.left + rect.width / 2,
          rect.top + rect.height / 2,
        );
        return hit === element || element.contains(hit);
      }),
    ).toBe(true);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await session.hoverPRChip();
    await expect(session.prChipPopover()).toBeVisible();
  });

  test("desktop popover persists automation and lifecycle notification options", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI automation desktop");
    const session = await openTask(testPage, taskId);
    const popover = session.prTopbarPopover();

    await expect(popover.getByTestId("pr-ci-automation-controls")).toBeVisible();
    await expect(
      popover.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).toBeVisible();
    await expect(
      popover.getByRole("switch", { name: "Auto-merge or requeue when ready" }),
    ).toBeVisible();
    const reviewFollowUp = popover.getByTestId("ci-review-follow-up-trigger");
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "false");
    await reviewFollowUp.click();
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "true");
    await expect(popover.getByRole("switch", { name: "Your review is requested" })).toBeVisible();
    await expect(popover.getByRole("switch", { name: "PR merged" })).toBeVisible();
    await expect(popover.getByRole("switch", { name: "PR closed without merging" })).toBeVisible();
    await popover.getByTestId("ci-review-requested-help").hover();
    await expect(
      testPage
        .getByRole("tooltip")
        .getByText("Wake the agent for any new request, including re-review after changes."),
    ).toBeVisible();
    await popover.getByTestId("ci-pr-terminal-help").hover();
    await expect(
      testPage
        .getByRole("tooltip")
        .getByText("Wake the agent when review work ends. Choose either or both outcomes."),
    ).toBeVisible();

    await popover.getByRole("switch", { name: "Auto-fix CI and address comments" }).click();
    await popover.getByRole("switch", { name: "Auto-merge or requeue when ready" }).click();
    await popover.getByRole("switch", { name: "Your review is requested" }).click();
    await popover.getByRole("switch", { name: "PR merged" }).click();

    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({
        auto_fix_enabled: true,
        auto_merge_enabled: true,
        prompt_on_review_requested: true,
        prompt_on_merged: true,
      });

    await popover.getByLabel("Explain CI automation options").hover();
    const queueRecoveryHelp = testPage.getByRole("tooltip");
    await expect(queueRecoveryHelp).toContainText("Auto-fix repairs actionable queue removals.");
    await expect(queueRecoveryHelp).toContainText(
      "Auto-merge submits an eligible head or requeues it after a new commit.",
    );
    await expect(queueRecoveryHelp).toContainText(
      "Both controls form the repair and requeue loop.",
    );
    await expect(queueRecoveryHelp).toContainText(
      "Kandev never requeues the same head after removal.",
    );

    await openPromptDialog(session);
    const promptDialog = testPage.getByRole("dialog", { name: "Auto-fix prompt" });
    await expect(promptDialog).toBeVisible();
    await expect(testPage.getByRole("link", { name: "Edit default prompt" })).toHaveAttribute(
      "href",
      "/settings/prompts",
    );
    await expect(promptDialog.getByTestId("ci-auto-fix-pr-feedback-placeholder")).toHaveText(
      "{{pr.feedback}}",
    );
    const feedbackHelp = promptDialog.getByTestId("ci-auto-fix-pr-feedback-help");
    await expect(feedbackHelp).toContainText("new or changed failing checks");
    await expect(feedbackHelp).toContainText("pull or fetch the branch");
    await testPage.getByLabel("Task auto-fix prompt").fill("Please fix only the new CI issues.");
    await testPage.getByRole("button", { name: "Save prompt" }).click();

    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({ auto_fix_prompt_override: "Please fix only the new CI issues." });

    await openPromptDialog(session);
    await testPage.getByRole("button", { name: "Use default" }).click();
    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({ auto_fix_prompt_override: null });

    const reloaded = await openTask(testPage, taskId);
    await expect(
      reloaded.prTopbarPopover().getByRole("switch", {
        name: "Auto-fix CI and address comments",
      }),
    ).toBeChecked();
    await expect(
      reloaded.prTopbarPopover().getByRole("switch", { name: "Auto-merge or requeue when ready" }),
    ).toBeChecked();
  });

  test("desktop popover keeps two linked PRs' automation switches independent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI automation independence");
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

    const session = await openTask(testPage, taskId);
    const popover = session.prTopbarPopover();

    // Pin the tab to PR #144 explicitly: the default tab tracks live
    // "worst status" data and can otherwise flip between clicks as
    // background PR sync updates check status.
    await popover.getByRole("tab", { name: `${REPO} #${PR_NUMBER}` }).click();
    await expect(
      popover.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).toBeVisible();
    await popover.getByRole("switch", { name: "Auto-fix CI and address comments" }).click();
    await popover.getByRole("switch", { name: "Auto-merge or requeue when ready" }).click();

    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({
        pr_options: expect.arrayContaining([
          expect.objectContaining({
            pr_number: PR_NUMBER,
            auto_fix_enabled: true,
            auto_merge_enabled: true,
          }),
          expect.objectContaining({
            pr_number: secondPRNumber,
            auto_fix_enabled: false,
            auto_merge_enabled: false,
          }),
        ]),
      });

    // The second PR's tab must show its own, independently off, state.
    await popover.getByRole("tab", { name: `${REPO} #${secondPRNumber}` }).click();
    await expect(
      popover.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).not.toBeChecked();
    await expect(
      popover.getByRole("switch", { name: "Auto-merge or requeue when ready" }),
    ).not.toBeChecked();

    // Reload: independence must persist across a full page load. Select each
    // tab explicitly — the default tab tracks live "worst status" data (real
    // CI automation may have altered it), so it is not a stable signal here.
    await testPage.reload();
    const reloaded = await openTask(testPage, taskId);
    const reloadedPopover = reloaded.prTopbarPopover();
    await reloadedPopover.getByRole("tab", { name: `${REPO} #${PR_NUMBER}` }).click();
    await expect(
      reloadedPopover.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).toBeChecked();
    await expect(
      reloadedPopover.getByRole("switch", { name: "Auto-merge or requeue when ready" }),
    ).toBeChecked();
    await reloadedPopover.getByRole("tab", { name: `${REPO} #${secondPRNumber}` }).click();
    await expect(
      reloadedPopover.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).not.toBeChecked();
    await expect(
      reloadedPopover.getByRole("switch", { name: "Auto-merge or requeue when ready" }),
    ).not.toBeChecked();
  });

  test("desktop popover shows the selected PR lifecycle delivery error", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI automation lifecycle error");
    await interceptLifecycleError(testPage, seedData.repositoryId);

    const session = await openTask(testPage, taskId);
    const popover = session.prTopbarPopover();
    await popover.getByTestId("ci-review-follow-up-trigger").click();
    await expect(popover.getByRole("switch", { name: "Your review is requested" })).toBeVisible();
    await expect(popover.getByRole("alert")).toContainText(
      "Lifecycle prompt could not be delivered to a task session.",
    );
    await expect(popover.getByRole("button", { name: "Refresh" })).toBeVisible();
  });

  test("desktop retries one failed automatic merge for the reviewed head", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const headSHA = "head-auto-merge-retry-desktop";
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI auto-merge retry desktop", {
      head_sha: headSHA,
      checks_state: "success",
      checks_total: 1,
      checks_passing: 1,
      unresolved_review_threads: 0,
      mergeable_state: "clean",
    });
    await apiClient.mockGitHubSetMergeOutcome(OWNER, REPO, PR_NUMBER, "failed");
    await apiClient.updateTaskCIAutomationOptions(taskId, {
      repository_id: seedData.repositoryId,
      pr_number: PR_NUMBER,
      auto_merge_enabled: true,
    });

    await expect
      .poll(async () => {
        const options = await apiClient.getTaskCIAutomationOptions(taskId);
        const state = options.pr_states.find((item) => item.pr_number === PR_NUMBER);
        return { kind: state?.last_error_kind, result: state?.last_merge_result };
      })
      .toEqual({ kind: "auto_merge", result: "failed" });
    await expect.poll(() => apiClient.mockGitHubGetMergeAttempts()).toHaveLength(1);

    // An unchanged provider event reevaluates the PR but cannot repeat the
    // failed signature without an explicit retry authorization.
    await apiClient.mockGitHubTransitionMergeQueue({
      task_id: taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      head_sha: headSHA,
      merge_queue_state: "",
      checks: [{ name: "required", status: "completed", conclusion: "success" }],
    });
    await expect.poll(() => apiClient.mockGitHubGetMergeAttempts()).toHaveLength(1);

    const session = await openTask(testPage, taskId);
    const popover = session.prTopbarPopover();
    const retry = popover.getByRole("button", { name: "Retry" });
    await expect(retry).toBeVisible();
    await apiClient.mockGitHubSetMergeOutcome(OWNER, REPO, PR_NUMBER, "queued");
    await retry.click();

    await expect.poll(() => apiClient.mockGitHubGetMergeAttempts()).toHaveLength(2);
    const attempts = await apiClient.mockGitHubGetMergeAttempts();
    expect(attempts).toEqual([
      expect.objectContaining({
        owner: OWNER,
        repo: REPO,
        number: PR_NUMBER,
        expected_head_sha: headSHA,
      }),
      expect.objectContaining({
        owner: OWNER,
        repo: REPO,
        number: PR_NUMBER,
        expected_head_sha: headSHA,
      }),
    ]);
    await expect
      .poll(async () => {
        const options = await apiClient.getTaskCIAutomationOptions(taskId);
        const state = options.pr_states.find((item) => item.pr_number === PR_NUMBER);
        return { error: state?.last_error ?? null, result: state?.last_merge_result };
      })
      .toEqual({ error: null, result: "accepted" });
  });

  test("desktop merge queue recovery proves repair and new-head requeue", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const queuedHead = "head-queued-desktop";
    const replacementHead = "head-replacement-desktop";
    const taskId = await seedTaskWithPR(apiClient, seedData, "CI merge queue recovery", {
      head_sha: queuedHead,
      checks_state: "success",
      checks_total: 1,
      checks_passing: 1,
      unresolved_review_threads: 0,
      mergeable_state: "clean",
      merge_queue_state: "queued",
      merge_queue_position: 1,
      merge_queue_entry_id: "entry-desktop-a",
      merge_queue_entry_head_sha: queuedHead,
    });
    await apiClient.mockGitHubSetMergeOutcome(OWNER, REPO, PR_NUMBER, "queued");

    const session = await openTask(testPage, taskId);
    const popover = session.prTopbarPopover();
    await expect(popover.getByText("Merge queue automation")).toBeVisible();
    await expect(popover.getByText("PR #144 is in the merge queue")).toBeVisible();
    await expect(popover.getByTestId("ci-merge-queue-recovery-status")).toContainText(
      "Active merge queue attempt",
    );
    await expect(popover.getByRole("switch")).toHaveCount(2);

    await popover.getByRole("switch", { name: "Auto-fix CI and address comments" }).click();
    await popover.getByRole("switch", { name: "Auto-merge or requeue when ready" }).click();
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
      merge_queue_last_removal_id: "removal-desktop-a",
      merge_queue_last_removed_at: new Date().toISOString(),
      merge_queue_last_removal_reason: "checks failed on merge group",
      merge_queue_last_removal_before_sha: "merge-group-desktop-a",
      checks: [
        {
          name: "merge group checks",
          status: "completed",
          conclusion: "failure",
          html_url: "https://example.test/checks/merge-group-desktop",
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
      .toEqual({ round: 1, event: "removal-desktop-a", cause: "checks_failed" });
    await expect(popover.getByText("Merge queue recovery")).toBeVisible();
    await expect(popover.getByText("PR #144 was removed: checks failed")).toBeVisible();
    await expect(popover.getByTestId("ci-merge-queue-recovery-status")).toContainText(
      "Repair requested. Waiting for a new commit before requeue",
    );

    // A clean replacement of the same head is still blocked by the durable
    // queue-attempt head guard, so it cannot create a merge request.
    await apiClient.mockGitHubTransitionMergeQueue({
      task_id: taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      head_sha: queuedHead,
      merge_queue_state: "",
      merge_queue_entry_id: "",
      merge_queue_entry_head_sha: "",
      merge_queue_last_removal_id: "removal-desktop-a",
      merge_queue_last_removal_reason: "checks failed on merge group",
      merge_queue_last_removal_before_sha: "merge-group-desktop-a",
      checks: [
        {
          name: "merge group checks",
          status: "completed",
          conclusion: "success",
          html_url: "https://example.test/checks/merge-group-desktop",
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
      merge_queue_last_removal_id: "removal-desktop-a",
      merge_queue_last_removal_reason: "checks failed on merge group",
      merge_queue_last_removal_before_sha: "merge-group-desktop-a",
      checks: [
        {
          name: "merge group checks",
          status: "completed",
          conclusion: "success",
          html_url: "https://example.test/checks/merge-group-desktop",
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

    // Reflect GitHub accepting the queued merge request. The old removal
    // evidence remains durable, but the active entry takes presentation
    // precedence and a second merge request is still prohibited.
    await apiClient.mockGitHubTransitionMergeQueue({
      task_id: taskId,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      head_sha: replacementHead,
      merge_queue_state: "queued",
      merge_queue_position: 1,
      merge_queue_entry_id: "entry-desktop-b",
      merge_queue_entry_head_sha: replacementHead,
      checks: [
        {
          name: "merge group checks",
          status: "completed",
          conclusion: "success",
          html_url: "https://example.test/checks/merge-group-desktop",
        },
      ],
    });
    await expect(popover.getByText("Merge queue automation")).toBeVisible();
    await expect(popover.getByTestId("ci-merge-queue-recovery-status")).toContainText(
      "Active merge queue attempt",
    );
    await expect.poll(() => apiClient.mockGitHubGetMergeAttempts()).toHaveLength(1);
  });
});
