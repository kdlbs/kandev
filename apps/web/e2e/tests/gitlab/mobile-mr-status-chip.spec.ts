import { test as base, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";

const test = base.extend({
  prCapture: async ({ testPage }, use, testInfo) => {
    const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: testInfo.title });
    await use(capture);
    capture.flush();
  },
});

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
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "Mobile MR chip");

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.mrStatusChip()).toBeVisible({ timeout: 15_000 });
    // @covers AC-UI-COMPOSER-ACTION-WRAP-001.7
    for (const width of [393, 767, 800, 320]) {
      await testPage.setViewportSize({ width, height: 851 });
      await expect
        .poll(async () => {
          const box = await session.mrStatusChip().boundingBox();
          return box ? Math.min(box.width, box.height) : 0;
        })
        .toBeGreaterThanOrEqual(44);
      expect(
        await session.mrStatusChip().evaluate((chip) => {
          const box = chip.getBoundingClientRect();
          return chip.contains(
            document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
          );
        }),
      ).toBe(true);
    }
    await waitForFiniteAnimations(session.chatStatusBar());
    await prCapture.screenshot("mobile-gitlab-composer-target", {
      caption: "GitLab composer status keeps a 44px touch target on a 320px phone",
    });
    await session.tapMRStatusChip();

    const drawer = session.mrStatusChipDrawer();
    await expect(drawer).toBeVisible();
    await expect(session.mrStatusChipPopoverInner()).toContainText(`!${MR_IID}`);

    await session.mrStatusChipDrawerClose().click();
    await expect(drawer).toBeHidden({ timeout: 5_000 });
    await expect(session.mrStatusChip()).toBeFocused();
  });

  test("keeps narrow fine-pointer targets large and desktop chips compact", async ({
    desktopTestPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "Responsive MR chip");
    await desktopTestPage.setViewportSize({ width: 393, height: 851 });
    await desktopTestPage.goto(`/t/${taskId}`);
    const session = new SessionPage(desktopTestPage);
    await session.waitForLoad();
    const chip = session.mrStatusChip();
    await expect(chip).toBeVisible();
    await expect
      .poll(async () => {
        const box = await chip.boundingBox();
        return box ? Math.min(box.width, box.height) : 0;
      })
      .toBeGreaterThanOrEqual(44);
    await chip.hover();
    await expect(session.mrStatusChipPopoverInner()).toBeVisible();
    await desktopTestPage.mouse.move(0, 0);
    await expect(session.mrStatusChipPopoverInner()).toBeHidden();
    await desktopTestPage.setViewportSize({ width: 1280, height: 900 });
    await expect.poll(async () => (await chip.boundingBox())?.height ?? 100).toBeLessThan(44);
    await waitForFiniteAnimations(session.chatStatusBar());
    await prCapture.screenshot("desktop-gitlab-composer-target", {
      page: desktopTestPage,
      caption: "GitLab composer status retains compact sizing on a desktop",
    });
  });
});
