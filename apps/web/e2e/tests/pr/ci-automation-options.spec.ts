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
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  return { taskId: task.id, sessionId: task.session_id };
}

async function openTask(
  testPage: import("@playwright/test").Page,
  task: { taskId: string; sessionId: string },
) {
  await testPage.goto(`/t/${task.taskId}?sessionId=${task.sessionId}`);
  const session = new SessionPage(testPage);
  await session.waitForDockviewReady(30_000);
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

test.describe("PR CI automation options", () => {
  test("desktop popover persists toggles and task prompt overrides", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await seedTaskWithPR(apiClient, seedData, "CI automation desktop");
    const session = await openTask(testPage, task);
    const popover = session.prTopbarPopover();

    await expect(popover.getByTestId("pr-ci-automation-controls")).toBeVisible();
    await expect(
      popover.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).toBeVisible();
    await expect(popover.getByRole("switch", { name: "Auto-merge when ready" })).toBeVisible();

    await popover.getByRole("switch", { name: "Auto-fix CI and address comments" }).click();
    await popover.getByRole("switch", { name: "Auto-merge when ready" }).click();

    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(task.taskId))
      .toMatchObject({ auto_fix_enabled: true, auto_merge_enabled: true });

    await popover.getByLabel("Explain CI automation options").hover();
    await expect(testPage.getByText(/1 minute PR refresh loop/)).toBeVisible();
    await expect(testPage.getByText(/snapshots what was handled/)).toBeVisible();

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
      .poll(async () => apiClient.getTaskCIAutomationOptions(task.taskId))
      .toMatchObject({ auto_fix_prompt_override: "Please fix only the new CI issues." });

    await openPromptDialog(session);
    await testPage.getByRole("button", { name: "Use default" }).click();
    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(task.taskId))
      .toMatchObject({ auto_fix_prompt_override: null });

    await testPage.reload();
    const reloaded = await openTask(testPage, task);
    await expect(
      reloaded.prTopbarPopover().getByRole("switch", {
        name: "Auto-fix CI and address comments",
      }),
    ).toBeChecked();
    await expect(
      reloaded.prTopbarPopover().getByRole("switch", { name: "Auto-merge when ready" }),
    ).toBeChecked();
  });
});
