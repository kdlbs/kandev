// Mobile project: tap-activated PR/CI chip in the chat status bar.
//
// On mobile the desktop topbar PRTopbarButton is not mounted, and the chip's
// HoverCard is unreachable by touch. The chip wraps a vaul Drawer that hosts
// the PRCIPopover body — these specs exercise the open/close path and verify
// the popover content renders the same surface as desktop.
//
// File name starts with "mobile-" so it lands in the `mobile-chrome` project
// (Pixel 5 emulation) defined in playwright.config.ts.
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

const OWNER = "acme";
const REPO = "demo";
const PR_NUMBER = 99;
const PR_URL = `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`;

type SeedTaskArgs = {
  apiClient: ApiClient;
  seedData: SeedData;
  title: string;
  prOverrides?: Partial<Parameters<ApiClient["mockGitHubAssociateTaskPR"]>[0]>;
};

async function seedTaskWithPR({
  apiClient,
  seedData,
  title,
  prOverrides = {},
}: SeedTaskArgs): Promise<string> {
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
    pr_title: "Add mobile CI drawer",
    head_branch: "feat/mobile-drawer",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    additions: 10,
    deletions: 1,
    ...prOverrides,
  });
  return task.id;
}

async function openTask(testPage: import("@playwright/test").Page, taskId: string) {
  await testPage.goto(`/t/${taskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return session;
}

test.describe("mobile PR CI chip drawer", () => {
  test("tapping the chip opens the drawer with PR CI content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Mobile CI tap-open";
    const taskId = await seedTaskWithPR({
      apiClient,
      seedData,
      title,
      prOverrides: {
        checks_state: "success",
        checks_total: 4,
        checks_passing: 4,
        review_state: "approved",
        review_count: 1,
      },
    });
    const session = await openTask(testPage, taskId);

    // Desktop topbar PR button never mounts on mobile — sanity check that we
    // are exercising the chip path and not a leftover desktop affordance.
    await expect(session.prTopbarButton()).toHaveCount(0);

    await expect(session.prStatusChip()).toBeVisible({ timeout: 15_000 });
    await expect(session.prStatusChipDrawer()).toHaveCount(0);

    await session.tapPRStatusChip();

    await expect(session.prStatusChipPopoverInner()).toBeVisible();
    await expect(
      session.prStatusChipDrawer().locator("[data-testid='pr-check-group'][data-kind='passed']"),
    ).toBeVisible();
    await expect(session.prStatusChipDrawer().getByTestId("pr-review-row")).toContainText(
      "Approved",
    );
  });

  test("failed PR shows failed bucket inside the drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Mobile CI failed";
    const taskId = await seedTaskWithPR({
      apiClient,
      seedData,
      title,
      prOverrides: {
        checks_state: "failure",
        checks_total: 3,
        checks_passing: 2,
      },
    });
    const session = await openTask(testPage, taskId);
    await expect(session.prStatusChip()).toBeVisible({ timeout: 15_000 });
    await session.tapPRStatusChip();

    const failedGroup = session
      .prStatusChipDrawer()
      .locator("[data-testid='pr-check-group'][data-kind='failed']");
    await expect(failedGroup).toBeVisible();
    // 3 checks_total − 2 checks_passing = 1 in the failed bucket.
    await expect(failedGroup.getByTestId("pr-check-group-count")).toHaveText("1");
  });

  test("close button dismisses the drawer", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const title = "Mobile CI close";
    const taskId = await seedTaskWithPR({
      apiClient,
      seedData,
      title,
      prOverrides: {
        checks_state: "success",
        checks_total: 1,
        checks_passing: 1,
      },
    });
    const session = await openTask(testPage, taskId);
    await expect(session.prStatusChip()).toBeVisible({ timeout: 15_000 });
    await session.tapPRStatusChip();
    await expect(session.prStatusChipDrawer()).toBeVisible();

    await session.prStatusChipDrawerClose().tap();
    await expect(session.prStatusChipDrawer()).toHaveCount(0, { timeout: 5_000 });
  });
});
