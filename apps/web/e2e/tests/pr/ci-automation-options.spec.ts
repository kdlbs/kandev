import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const OWNER = "acme";
const REPO = "demo";
const PR_NUMBER = 144;
const PR_URL = `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`;

async function seedTaskWithPR(apiClient: ApiClient, seedData: SeedData, title: string) {
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
    await expect(popover.getByRole("switch", { name: "Auto-merge when ready" })).toBeVisible();
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
    await popover.getByRole("switch", { name: "Auto-merge when ready" }).click();
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
    await expect(testPage.getByText(/1 minute PR refresh loop/)).toBeVisible();
    await expect(testPage.getByText(/notification switches wake the task's agent/)).toBeVisible();
    await expect(
      testPage.getByText(/workspace's connected GitHub account is requested for review/i),
    ).toBeVisible();

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

    await testPage.reload();
    const reloaded = await openTask(testPage, taskId);
    await expect(
      reloaded.prTopbarPopover().getByRole("switch", {
        name: "Auto-fix CI and address comments",
      }),
    ).toBeChecked();
    await expect(
      reloaded.prTopbarPopover().getByRole("switch", { name: "Auto-merge when ready" }),
    ).toBeChecked();
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
  });
});
