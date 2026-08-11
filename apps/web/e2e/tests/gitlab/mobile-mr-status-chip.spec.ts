import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const MR_IID = 411;

/**
 * Seeds one open MR directly with state: "open" (already normalized) rather
 * than the shared seedGitLabReview helper's GitLab-native "opened" — the
 * mock provider never normalizes state, and the chip's spec-mandated
 * `state === "open"` exact-equality filter needs the normalized form. See
 * mr-status-chip.spec.ts's seedChipMR for the full rationale.
 */
async function seedTaskWithLinkedMR(apiClient: ApiClient, seedData: SeedData, title: string) {
  await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
  const now = new Date().toISOString();
  await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
    {
      iid: MR_IID,
      id: MR_IID + 10_000,
      project_id: 101,
      title: "Mobile chip MR",
      url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
      web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
      state: "open",
      head_branch: "feature/mobile-chip",
      head_sha: `sha-${MR_IID}`,
      base_branch: "main",
      author_username: "contributor",
      project_namespace: "platform",
      project_path: GITLAB_PROJECT,
      body: "",
      draft: false,
      merge_status: "can_be_merged",
      has_conflicts: false,
      additions: 1,
      deletions: 1,
      reviewers: [],
      assignees: [],
      created_at: now,
      updated_at: now,
    },
  ]);
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

test.describe("mobile GitLab MR status chip", () => {
  test("tapping the chip opens the drawer with the popover body, and closing returns focus to the trigger", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "Mobile MR chip");

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.mrStatusChip()).toBeVisible({ timeout: 15_000 });
    await session.tapMRStatusChip();

    const drawer = session.mrStatusChipDrawer();
    await expect(drawer).toBeVisible();
    await expect(session.mrStatusChipPopoverInner()).toContainText(`!${MR_IID}`);

    await session.mrStatusChipDrawerClose().click();
    await expect(drawer).toBeHidden({ timeout: 5_000 });
    await expect(session.mrStatusChip()).toBeFocused();
  });
});
