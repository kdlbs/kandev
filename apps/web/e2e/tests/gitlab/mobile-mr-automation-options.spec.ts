import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  seedGitLabReview,
  seedTaskWithLinkedGitLabMRs,
  GITLAB_HOST,
  GITLAB_PROJECT,
} from "../../helpers/gitlab";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import { dwell } from "../../helpers/causal-waits";

const MR_IID = 211;

async function expectTouchTarget(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} has no layout box`).not.toBeNull();
  expect(box!.height, `${label} touch target height`).toBeGreaterThanOrEqual(44);
}

// DropdownMenuContent runs a 100ms zoom-in-95 entrance animation
// (data-open:animate-in ... duration-100 in @kandev/ui's dropdown-menu.tsx).
// A boundingBox() measured mid-animation reports the pre-settle scaled-down
// size, not the final CSS size — wait past the animation before measuring.
async function waitForDropdownSettled(page: import("@playwright/test").Page) {
  await dwell(
    page,
    200,
    "library-timer",
    "@kandev/ui's DropdownMenuContent runs a 100ms zoom-in entrance animation that publishes nothing on completion, and a boundingBox read mid-animation reports the pre-settle scaled size",
  );
}

async function seedTaskWithLinkedMR(apiClient: ApiClient, seedData: SeedData, title: string) {
  await seedGitLabReview(apiClient, seedData.workspaceId, MR_IID, "Mobile MR automation MR");
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

async function interceptLoadFailure(testPage: import("@playwright/test").Page) {
  await testPage.route("**/api/v1/gitlab/tasks/*/mr-automation", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    await route.fulfill({ status: 500, json: { error: "backend unavailable" } });
  });
}

test.describe("mobile GitLab MR automation options", () => {
  test("dropdown exposes touch-sized auto-fix/auto-merge switches above Review follow-up", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR automation section mobile");

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const mrButton = testPage.getByTestId("mr-topbar-button");
    await expect(mrButton).toBeVisible({ timeout: 15_000 });
    await mrButton.tap();

    const controls = testPage.getByTestId("mr-automation-controls");
    await expect(controls).toBeVisible();
    await waitForDropdownSettled(testPage);

    const autoFixSwitch = controls.getByRole("switch", {
      name: "Auto-fix CI and address comments",
    });
    const autoMergeSwitch = controls.getByRole("switch", { name: "Auto-merge when ready" });
    await expectTouchTarget(autoFixSwitch.locator(".."), "auto-fix row");
    await expectTouchTarget(autoMergeSwitch.locator(".."), "auto-merge row");

    await autoFixSwitch.tap();
    await expect
      .poll(async () => apiClient.getTaskMRAutomationOptions(taskId))
      .toMatchObject({ auto_fix_enabled: true });

    await assertNoDocumentHorizontalOverflow(testPage, "mobile MR automation section");
  });

  test("dropdown exposes touch-sized automation controls and persists switches", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR automation mobile");

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const mrButton = testPage.getByTestId("mr-topbar-button");
    await expect(mrButton).toBeVisible({ timeout: 15_000 });
    await expectTouchTarget(mrButton, "GitLab MR topbar button");
    await mrButton.tap();

    const controls = testPage.getByTestId("mr-automation-controls");
    await expect(controls).toBeVisible();
    await waitForDropdownSettled(testPage);
    const reviewFollowUp = controls.getByTestId("mr-review-follow-up-trigger");
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "false");
    await expectTouchTarget(reviewFollowUp, "review follow-up trigger");
    await reviewFollowUp.tap();
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "true");

    for (const name of ["Your review is requested", "MR merged", "MR closed without merging"]) {
      const switchLocator = controls.getByRole("switch", { name });
      await expect(switchLocator).toBeVisible();
      const row = switchLocator.locator("..");
      const box = await row.boundingBox();
      expect(box, `${name} row has no layout box`).not.toBeNull();
      expect(box!.height, `${name} row touch target height`).toBeGreaterThanOrEqual(44);
    }

    const helpPopover = controls.locator('[data-slot="popover-content"]');
    await controls.getByTestId("mr-review-requested-help").tap();
    await expect(
      helpPopover.getByText(
        "Wake the agent when the workspace's connected GitLab account is added as a reviewer. Being re-added after removal counts as a new request; staying assigned across MR updates does not.",
      ),
    ).toBeVisible();
    await testPage.keyboard.press("Escape");
    await controls.getByTestId("mr-terminal-help").tap();
    await expect(
      helpPopover.getByText(
        "Wake the agent when review work ends. Choose either or both outcomes.",
      ),
    ).toBeVisible();
    await testPage.keyboard.press("Escape");

    await controls.getByRole("switch", { name: "Your review is requested" }).tap();
    await controls.getByRole("switch", { name: "MR closed without merging" }).tap();
    await expect
      .poll(async () => apiClient.getTaskMRAutomationOptions(taskId))
      .toMatchObject({
        prompt_on_review_requested: true,
        prompt_on_closed: true,
      });

    await assertNoDocumentHorizontalOverflow(testPage, "mobile MR automation dropdown");
  });

  test("dropdown contains a load-error retry banner without horizontal document overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskId = await seedTaskWithLinkedMR(
      apiClient,
      seedData,
      "MR automation mobile load error",
    );
    await interceptLoadFailure(testPage);

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mr-topbar-button").tap();

    const controls = testPage.getByTestId("mr-automation-controls");
    await expect(controls).toBeVisible();
    await expect(controls.getByRole("alert")).toContainText("backend unavailable");
    await waitForDropdownSettled(testPage);
    const retry = controls.getByTestId("mr-automation-retry");
    await expectTouchTarget(retry, "MR automation retry button");

    await assertNoDocumentHorizontalOverflow(testPage, "mobile MR automation load error");
  });

  test("dropdown renders one attributed automation block per linked MR, independently toggleable (AC1, AC26)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const iidA = 222;
    const iidB = 223;
    const taskId = await seedTaskWithLinkedGitLabMRs(
      apiClient,
      seedData,
      "Mobile MR automation independence",
      [iidA, iidB],
      "Mobile MR automation independence",
    );

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const mrButton = testPage.getByTestId("mr-topbar-button");
    await expect(mrButton).toBeVisible({ timeout: 15_000 });
    await mrButton.tap();
    await waitForDropdownSettled(testPage);

    const controlsA = testPage.locator(
      `[data-testid="mr-automation-controls"][data-mr-iid="${iidA}"]`,
    );
    const controlsB = testPage.locator(
      `[data-testid="mr-automation-controls"][data-mr-iid="${iidB}"]`,
    );
    await expect(controlsA).toBeVisible();
    await expect(controlsB).toBeVisible();
    await expect(controlsA.getByTestId("mr-automation-scope-label")).toHaveText(
      `Applies to !${iidA}`,
    );
    await expect(controlsB.getByTestId("mr-automation-scope-label")).toHaveText(
      `Applies to !${iidB}`,
    );

    const autoFixA = controlsA.getByRole("switch", { name: "Auto-fix CI and address comments" });
    const autoFixB = controlsB.getByRole("switch", { name: "Auto-fix CI and address comments" });
    await autoFixA.tap();
    await expect
      .poll(async () => {
        const options = await apiClient.getTaskMRAutomationOptions(taskId);
        return options.mr_options?.find((o) => o.mr_iid === iidA)?.auto_fix_enabled;
      })
      .toBe(true);
    const options = await apiClient.getTaskMRAutomationOptions(taskId);
    expect(options.mr_options?.find((o) => o.mr_iid === iidB)?.auto_fix_enabled).toBe(false);
    await expect(autoFixB).not.toBeChecked();

    await assertNoDocumentHorizontalOverflow(testPage, "mobile MR automation multi-MR dropdown");
  });
});
