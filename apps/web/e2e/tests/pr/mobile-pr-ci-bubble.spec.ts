import { test, expect, type SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const OWNER = "testorg";
const REPO = "testrepo";
const PR_NUMBER = 611;

async function seedMobileCI(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ navigationTaskId: string; targetTaskId: string }> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const navigationTask = await apiClient.seedTask(
    seedData.workspaceId,
    "Mobile CI navigation",
    stepOptions,
  );
  const targetTask = await apiClient.seedTask(
    seedData.workspaceId,
    "Mobile CI target",
    stepOptions,
  );
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: targetTask.task_id,
    workspace_id: seedData.workspaceId,
    repository_id: seedData.repositoryId,
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
    pr_title: "Mobile draft CI",
    head_branch: "feat/mobile-ci-bubble",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    head_sha: "mobile-ci-head",
    checks_state: "failure",
    mergeable_state: "draft",
    has_merge_conflicts: true,
  });
  await expect
    .poll(async () => {
      const response = await apiClient.listTasks(seedData.workspaceId);
      const pullRequest = response.tasks.find((task) => task.id === targetTask.task_id)
        ?.status_summary?.pull_request;
      return {
        ci_state: pullRequest?.ci_state,
        conflicts: pullRequest?.has_merge_conflicts === true,
      };
    })
    .toMatchObject({ ci_state: "failed", conflicts: true });
  return { navigationTaskId: navigationTask.task_id, targetTaskId: targetTask.task_id };
}

test.describe("Mobile PR CI bubble", () => {
  test("shows the bubble before tap and opens the existing PR drawer from the wide hitbox", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const { navigationTaskId, targetTaskId } = await seedMobileCI(apiClient, seedData);

    await testPage.goto(`/t/${navigationTaskId}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    await waitForFiniteAnimations(sheet);
    const row = sheet.locator(`[data-task-row-id="${targetTaskId}"]`);
    await expect(row).toBeVisible();
    const icon = row.getByTestId(`pr-task-icon-${targetTaskId}`);
    await expect(icon).toBeVisible();
    await expect(icon.getByTestId("pr-ci-bubble-failed")).toBeVisible();
    await expect(icon.getByTestId("pr-merge-conflict-warning")).toBeVisible();
    await expect(icon).toHaveAttribute("aria-label", /Checks failed/);
    await expect(icon).toHaveAttribute("aria-label", /Conflicts/);
    await assertNoDocumentHorizontalOverflow(testPage);
    await prCapture.screenshot("mobile-ci-bubble-before-tap", {
      caption: "Phone task row shows the draft CI bubble and separate conflict triangle.",
    });

    const rowURL = testPage.url();
    const box = await icon.boundingBox();
    expect(box).not.toBeNull();
    // The coarse-pointer hitbox extends 15px beyond the 14px glyph, so a tap
    // near its outer edge still opens the disclosure.
    await testPage.touchscreen.tap(box!.x + box!.width / 2 + 15, box!.y + box!.height / 2);

    const drawer = testPage.getByTestId(`pr-task-automation-drawer-${targetTaskId}`);
    await expect(drawer).toBeVisible({ timeout: 15_000 });
    await waitForFiniteAnimations(drawer);
    await expect(drawer.getByTestId("pr-task-status-ci")).toContainText("Failed");
    await expect(drawer.getByTestId("pr-task-status-merge")).toContainText("Conflicts");
    expect(testPage.url()).toBe(rowURL);
    await assertNoDocumentHorizontalOverflow(testPage);
    await prCapture.screenshot("mobile-ci-drawer", {
      caption: "Phone PR drawer keeps the CI and conflict explanations in one scrolling body.",
    });

    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
    await expect(icon).toBeFocused();
  });
});
