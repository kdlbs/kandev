import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const OWNER = "acme";
const REPO = "demo";
const PR_NUMBER = 145;
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
  });
  return task.id;
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
    await expect(drawer.getByRole("switch", { name: "Auto-merge when ready" })).toBeVisible();

    await drawer.getByRole("switch", { name: "Auto-fix CI and address comments" }).tap();
    await expect
      .poll(async () => apiClient.getTaskCIAutomationOptions(taskId))
      .toMatchObject({ auto_fix_enabled: true });

    await drawer.getByLabel("Edit auto-fix prompt for this task").tap();
    const promptDialog = testPage.getByRole("dialog", { name: "Auto-fix prompt" });
    await expect(promptDialog).toBeVisible();
    await expect(testPage.getByRole("link", { name: "Edit default prompt" })).toHaveAttribute(
      "href",
      "/settings/prompts",
    );
    await expect(promptDialog.locator("code").filter({ hasText: "{{pr.feedback}}" })).toBeVisible();
    await expect(testPage.getByText(/new or changed review comments/)).toBeVisible();
  });
});
