import { routeCommitSpacingSource } from "./commit-spacing-source";
import { test, expect } from "../../fixtures/test-base";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";
import {
  captureBrowserObservation,
  expectBoundedTimeline,
  expectNoPageHorizontalOverflow,
  installLongTaskObserver,
  LARGE_CHANGES_FILE_COUNT,
  largeChangesFileTestId,
  scrollChangesToEnd,
  scrollChangesToStart,
  seedLargeWorkingTree,
  type BrowserObservation,
} from "./large-changes-helpers";
import { SessionPage } from "../../pages/session-page";
import {
  expectContiguousTimeline,
  refreshSpacingAndExpectAnchor,
  seedCommitSpacingHistory,
} from "./changes-commit-spacing-helpers";
import path from "node:path";

test.describe("Large Changes virtualization", () => {
  test.describe.configure({ retries: 0, timeout: 180_000 });

  test.beforeEach(({ backend }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.exec("git reset --hard HEAD");
    git.exec("git clean -fd");
  });

  // @covers AC-UI-BOUNDED-CHANGES-001.6
  test("keeps commit spacing contiguous through refresh, resize, and reopening", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    const profile = await createStandardProfile(apiClient, "Commit Spacing Profile");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Commit Spacing",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const history = await routeCommitSpacingSource(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await session.clickTab("Changes");
    await seedCommitSpacingHistory(testPage, history);
    await refreshSpacingAndExpectAnchor(testPage);
    await prCapture.screenshot("changelist-spacing-desktop", {
      caption: "Commit history retains compact, contiguous rows after measurement refresh",
    });
    for (const top of [360, 0, 360]) {
      await testPage.getByTestId("changes-panel-scroll-owner").evaluate((element, offset) => {
        element.scrollTop = offset;
        element.dispatchEvent(new Event("scroll"));
      }, top);
      await refreshSpacingAndExpectAnchor(testPage);
    }
    for (const width of [1040, 768, 1280]) {
      await testPage.setViewportSize({ width, height: 900 });
      await refreshSpacingAndExpectAnchor(testPage);
    }
    await session.clickSessionChatTab();
    await session.clickTab("Changes");
    await expectContiguousTimeline(testPage);
    await expectNoPageHorizontalOverflow(testPage);
  });

  for (const layout of ["tree", "flat"] as const) {
    test(`keeps all 50,000 ${layout} working files reachable in a bounded window`, async ({
      testPage,
      apiClient,
      seedData,
    }, testInfo) => {
      await testPage.setViewportSize({ width: layout === "tree" ? 768 : 1280, height: 900 });
      if (layout === "tree") {
        await expect
          .poll(() => testPage.evaluate(() => matchMedia("(pointer: fine)").matches))
          .toBe(true);
      }
      await apiClient.saveUserSettings({ changes_panel_layout: layout });
      const profile = await createStandardProfile(apiClient, `Large Changes ${layout} Profile`);
      const title = `Large Changes ${layout}`;
      await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });

      const session = await openTaskSession(testPage, title);
      await session.waitForChatIdle({ timeout: 30_000 });
      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("changes-panel-scroll-owner")).toBeVisible();
      await installLongTaskObserver(testPage);
      const observations: BrowserObservation[] = [
        await captureBrowserObservation(testPage, "empty-panel"),
      ];

      await seedLargeWorkingTree(testPage, layout);
      const firstFile = largeChangesFileTestId(0);
      const lastFile = largeChangesFileTestId(LARGE_CHANGES_FILE_COUNT - 1);
      await expect(testPage.getByTestId(firstFile)).toBeVisible({ timeout: 30_000 });
      await expectBoundedTimeline(testPage);
      if (layout === "tree") {
        const directory = testPage.locator("[data-changes-tree-directory]").first();
        await expect(directory).toBeVisible();
        for (const width of [1280, 767]) {
          await testPage.setViewportSize({ width, height: 900 });
          if (width < 768) {
            await testPage
              .getByRole("navigation")
              .getByRole("button", { name: /Changes$/ })
              .click();
            await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
            await scrollChangesToStart(testPage, firstFile);
          } else {
            await expect(testPage.getByTestId("changes-panel")).toBeVisible({ timeout: 30_000 });
          }
          await expect(directory).toBeVisible({ timeout: 30_000 });
          await expect
            .poll(() => testPage.evaluate(() => matchMedia("(pointer: fine)").matches))
            .toBe(true);
          await expect(directory).toHaveCSS("min-height", "24px");
        }
      }
      const firstRow = testPage.getByTestId(firstFile);
      const lastRow = testPage.getByTestId(lastFile);
      await firstRow.focus();
      await testPage.keyboard.press("End");
      await expect(lastRow).toBeFocused({ timeout: 15_000 });
      await scrollChangesToEnd(testPage, lastFile);
      observations.push(await captureBrowserObservation(testPage, "50k-at-end"));

      for (let pass = 0; pass < 3; pass += 1) {
        await scrollChangesToStart(testPage, firstFile);
        await scrollChangesToEnd(testPage, lastFile);
      }
      observations.push(await captureBrowserObservation(testPage, "50k-after-repeat-scroll"));
      await expectNoPageHorizontalOverflow(testPage);
      console.log("Large Changes browser observations", JSON.stringify(observations));
      await testInfo.attach("large-changes-browser-observations", {
        body: Buffer.from(JSON.stringify(observations, null, 2)),
        contentType: "application/json",
      });

      await testPage.setViewportSize({ width: 1280, height: 900 });
      const replacement = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        `${title} Replacement`,
        profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      await testPage.goto(`/t/${replacement.id}`);
      const replacementSession = new SessionPage(testPage);
      await replacementSession.waitForLoad();
      await replacementSession.waitForChatIdle({ timeout: 30_000 });
      await replacementSession.clickTab("Changes");
      await expect(replacementSession.changes).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId(lastFile)).toHaveCount(0);
      await expectNoPageHorizontalOverflow(testPage);
    });
  }
});
