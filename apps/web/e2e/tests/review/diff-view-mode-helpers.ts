import fs from "node:fs";
import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { waitForSessionDone, waitForWorkspacePath } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";
import type { GitStatusData } from "../../../lib/types/git-events";

const fileName = "diff-mode-controls.ts";
const modeIcons = ".tabler-icon-layout-columns, .tabler-icon-layout-rows";
const topbarMode = (root: Locator) =>
  root.locator("button:not([aria-label])").filter({ has: root.page().locator(modeIcons) });
const expectedIcon = (split: boolean) =>
  split ? ".tabler-icon-layout-rows" : ".tabler-icon-layout-columns";

async function activate(control: Locator, mobile: boolean) {
  await expect(control).toHaveCount(1);
  if (mobile) await control.tap();
  else await control.click();
}

async function assertStored(page: Page, split: boolean) {
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("diff-view-mode")))
    .toBe(split ? "split" : "unified");
}

async function toggleChanges(page: Page, header: Locator, split: boolean, mobile: boolean) {
  await expect(header).toBeVisible();
  const direct = topbarMode(header);
  if (await direct.isVisible()) {
    await expect(direct.locator(expectedIcon(split))).toBeVisible();
    await activate(direct, mobile);
  } else {
    await activate(header.getByTestId("panel-header-overflow"), mobile);
    await activate(
      page.getByRole("menuitem", { name: split ? "Unified view" : "Split view", exact: true }),
      mobile,
    );
  }
  await assertStored(page, !split);
}

export function diffViewModeScenario(mobile: boolean) {
  test.setTimeout(120_000);
  // @covers AC-UI-DIFF-PRESENTATION-001.1, AC-UI-DIFF-PRESENTATION-001.2, AC-UI-DIFF-PRESENTATION-001.3
  test("synchronizes mounted Review and Changes mode controls and the next activation", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const seedGit = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
    const seedHead = seedGit.getCurrentSha();
    const seedStatus = seedGit.exec("git status --porcelain=v1 --untracked-files=all");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Diff mode controls",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await waitForSessionDone(apiClient, task.id, task.session_id!, "Diff mode fixture settled");
    const workspace = await waitForWorkspacePath(apiClient, task.id, task.session_id!);
    expect(fs.realpathSync(workspace)).not.toBe(fs.realpathSync(seedData.repositoryPath));
    const git = new GitHelper(workspace, makeGitEnv(backend.tmpDir));
    const gateway = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    await testPage.evaluate(() => localStorage.setItem("diff-view-mode", "unified"));
    await testPage.reload();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const published = gateway.waitForEvent("session.git.event", {
      where: (payload) =>
        payload.session_id === task.session_id &&
        payload.type === "status_update" &&
        Boolean((payload.status as GitStatusData | undefined)?.files?.[fileName]),
    });
    try {
      git.createFile(fileName, "export const value = 1;\n");
      git.stageFile(fileName);
      expect(seedGit.getCurrentSha()).toBe(seedHead);
      expect(seedGit.exec("git status --porcelain=v1 --untracked-files=all")).toBe(seedStatus);
      await published;
      if (mobile) {
        await testPage
          .getByRole("navigation")
          .getByRole("button", { name: /Changes$/ })
          .tap();
        await activate(
          testPage.getByTestId("mobile-changes-panel").getByTestId(`file-row-${fileName}`),
          true,
        );
      } else {
        await session.clickTab("Changes");
        await expect(session.changes.getByTestId(`file-row-${fileName}`)).toBeVisible();
        await session.openChangesDiff();
      }
      const diffRoot = mobile
        ? testPage.getByTestId("mobile-diff-sheet")
        : testPage.locator("body");
      const header = diffRoot.locator("[data-panel-header]").filter({
        has: testPage.getByRole("button", { name: "Expand review", exact: true }),
      });
      await expect(header).toHaveCount(1);
      if (!mobile) {
        const fileHeader = diffRoot.locator(
          `[data-testid="review-file-header"][data-file-path="${fileName}"]`,
        );
        await activate(
          fileHeader.getByRole("button", { name: "Switch to split view", exact: true }),
          false,
        );
        await assertStored(testPage, true);
        await toggleChanges(testPage, header, true, false);
      }
      // Leave Changes mounted underneath Review, then change the shared mode there.
      await activate(header.getByRole("button", { name: "Expand review", exact: true }), mobile);
      const dialog = testPage.getByRole("dialog", { name: "Review Changes", exact: true });
      const reviewMode = topbarMode(dialog);
      for (const initialSplit of [false, true]) {
        await assertStored(testPage, initialSplit);
        if (!mobile) {
          const fileHeader = dialog.locator(
            `[data-testid="review-file-header"][data-file-path="${fileName}"]`,
          );
          await activate(
            fileHeader.getByRole("button", {
              name: initialSplit ? "Switch to unified view" : "Switch to split view",
              exact: true,
            }),
            false,
          );
          await expect(reviewMode.locator(expectedIcon(!initialSplit))).toBeVisible();
          await activate(reviewMode, false);
          await assertStored(testPage, initialSplit);
        }
        await activate(reviewMode, mobile);
        await assertStored(testPage, !initialSplit);
        await activate(dialog.getByRole("button", { name: "Close review", exact: true }), mobile);
        await expect(dialog).not.toBeVisible();
        await toggleChanges(testPage, header, !initialSplit, mobile);
        // Normal Changes toggle establishes the other direction for the next pass.
        await toggleChanges(testPage, header, initialSplit, mobile);
        await activate(header.getByRole("button", { name: "Expand review", exact: true }), mobile);
        await expect(topbarMode(dialog).locator(expectedIcon(!initialSplit))).toBeVisible();
      }
      await testPage.evaluate(async () => {
        const animations = document
          .getAnimations()
          .filter(
            (animation) =>
              animation.playState === "running" &&
              Number.isFinite(animation.effect?.getComputedTiming().iterations),
          );
        await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
      });
      await prCapture.screenshot(mobile ? "diff-mode-phone" : "diff-mode-desktop", {
        caption: "Review and Changes share the saved diff view mode",
      });
      await expect(testPage.locator("html")).toHaveJSProperty(
        "scrollWidth",
        await testPage.locator("html").evaluate((element) => element.clientWidth),
      );
    } finally {
      git.exec(`git reset -- "${fileName}"`);
      fs.rmSync(path.join(workspace, fileName), { force: true });
      expect(seedGit.getCurrentSha()).toBe(seedHead);
      expect(seedGit.exec("git status --porcelain=v1 --untracked-files=all")).toBe(seedStatus);
    }
  });
}
