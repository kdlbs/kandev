import { test, expect } from "../../fixtures/test-base";
import {
  createStandardProfile,
  makeGitEnv,
  openTaskSession,
  GitHelper,
} from "../../helpers/git-helper";
import { expectTouchControl } from "../../helpers/control-sizing";
import {
  expectBoundedTimeline,
  expectNoPageHorizontalOverflow,
  LARGE_CHANGES_FILE_COUNT,
  largeChangesFileTestId,
  scrollChangesToEnd,
  seedLargeWorkingTree,
} from "./large-changes-helpers";
import path from "node:path";
import { SessionPage } from "../../pages/session-page";
import {
  refreshSpacingAndExpectAnchor,
  seedCommitSpacingHistory,
} from "./changes-commit-spacing-helpers";

test.describe("Mobile large Changes virtualization", () => {
  test.describe.configure({ retries: 0, timeout: 180_000 });

  test.beforeEach(({ backend }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.exec("git reset --hard HEAD");
    git.exec("git clean -fd");
  });

  // @covers AC-UI-BOUNDED-CHANGES-001.6 and AC-UI-BOUNDED-CHANGES-001.8
  test("keeps phone commit spacing measured through refresh and breakpoint resize", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 393, height: 851 });
    const profile = await createStandardProfile(apiClient, "Mobile Commit Spacing Profile");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Commit Spacing",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    const changes = testPage.getByRole("navigation").getByRole("button", { name: /Changes$/ });
    await changes.tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
    await seedCommitSpacingHistory(testPage);
    await refreshSpacingAndExpectAnchor(testPage);
    await expectTouchControl(
      testPage.getByTestId("commit-row-0000000").getByTestId("commit-toggle"),
    );
    await prCapture.screenshot("changelist-spacing-phone", {
      caption: "Phone commit rows retain measured spacing and touch-sized actions",
    });
    for (const width of [393, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      await testPage.getByTestId("changes-panel-scroll-owner").evaluate((element) => {
        element.scrollTop = 360;
      });
      await refreshSpacingAndExpectAnchor(testPage);
      await expectNoPageHorizontalOverflow(testPage);
      await expect(testPage.getByTestId("changes-panel-scroll-owner")).toHaveCount(1);
    }
    await testPage.getByRole("navigation").getByRole("button", { name: /Chat$/ }).tap();
    await changes.tap();
    await refreshSpacingAndExpectAnchor(testPage);
  });

  test("keeps 50,000 list files reachable at 393px and 767px with touch actions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 393, height: 851 });
    await apiClient.saveUserSettings({ changes_panel_layout: "flat" });
    const profile = await createStandardProfile(apiClient, "Mobile Large Changes Profile");
    const title = "Mobile Large Changes";
    await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, title);
    await session.waitForChatIdle({ timeout: 30_000 });
    await testPage
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
    await seedLargeWorkingTree(testPage, "flat");

    const firstFile = testPage.getByTestId(largeChangesFileTestId(0));
    const lastFileTestId = largeChangesFileTestId(LARGE_CHANGES_FILE_COUNT - 1);
    await expect(firstFile).toBeVisible({ timeout: 30_000 });
    await expectBoundedTimeline(testPage);

    for (const width of [393, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
      await expect(testPage.getByTestId("changes-panel-scroll-owner")).toHaveCount(1);
      await expectBoundedTimeline(testPage);
      await scrollChangesToEnd(testPage, lastFileTestId);
      await expectNoPageHorizontalOverflow(testPage);

      const finalRow = testPage.getByTestId(lastFileTestId);
      const longName = finalRow.getByText(
        "large-change-49999-with-a-long-mobile-friendly-filename.ts",
        { exact: true },
      );
      await expect(longName).toHaveCSS("white-space", "normal");
      const actionMenuTrigger = finalRow.getByRole("button", { name: "Show more actions" });
      await expectTouchControl(actionMenuTrigger);

      const owner = testPage.getByTestId("changes-panel-scroll-owner");
      await owner.evaluate((element) => {
        element.scrollTop = 0;
        element.dispatchEvent(new Event("scroll"));
      });
      await expect(firstFile).toBeVisible();
      await expectBoundedTimeline(testPage);
      await scrollChangesToEnd(testPage, lastFileTestId);
    }
  });

  test("keeps tree directory controls touch sized at phone and breakpoint widths", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 393, height: 851 });
    await apiClient.saveUserSettings({ changes_panel_layout: "tree" });
    const profile = await createStandardProfile(apiClient, "Mobile Directory Sizing Profile");
    const title = "Mobile Directory Sizing";
    await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, title);
    await session.waitForChatIdle({ timeout: 30_000 });
    await testPage
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
    await seedLargeWorkingTree(testPage, "tree", 12);

    const directory = testPage.locator("[data-changes-tree-directory]").first();
    await expect(directory).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(() => testPage.evaluate(() => matchMedia("(pointer: coarse)").matches))
      .toBe(true);
    for (const width of [393, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      await expect(directory).toHaveCSS("min-height", "44px");
      await expectTouchControl(directory);
    }
  });
});
