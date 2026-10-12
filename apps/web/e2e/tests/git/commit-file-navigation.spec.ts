import { test, expect } from "../../fixtures/test-base";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";
import { expectControlHeight, expectTouchControl } from "../../helpers/control-sizing";
import path from "node:path";

test.describe("Commit file navigation", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test.beforeEach(({ backend }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.exec("git reset --hard HEAD");
    git.exec("git clean -fd");
  });

  test("opens a collapsible commit detail with a flat file index and navigates from inline files", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const profile = await createStandardProfile(apiClient, "Commit File Navigation Profile");
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Commit File Navigation",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("navigation-older.ts", "export const older = true;\n");
    git.stageAll();
    const olderSha = git.commit("Add earlier navigation history");
    git.createFile("navigation-one.ts", "export const one = 1;\n");
    git.createFile("navigation-two.ts", "navigation two\n");
    git.stageAll();
    const sha = git.commit("Add commit navigation files");
    git.createFile("navigation-one.ts", "export const one = 2;\n");

    // Seed history before the session mounts and fetches its initial commit
    // snapshot. Mutating git after that fetch relies on a later event to refresh
    // the commit feed, which is not part of this navigation test.
    const session = await openTaskSession(testPage, "Commit File Navigation");
    await session.waitForChatIdle({ timeout: 30_000 });

    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });
    await session.expandCommitsSection();

    const row = testPage.getByTestId(`commit-row-${sha.slice(0, 7)}`);
    await expect(row).toBeVisible({ timeout: 15_000 });
    const toggle = row.getByTestId("commit-toggle");
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expectControlHeight(toggle, 16);
    const openCommit = row.getByTestId(`commit-open-${sha.slice(0, 7)}`);
    await expectControlHeight(openCommit, 16);
    await expect(openCommit).toHaveAccessibleName("Open commit");
    await expect(openCommit).toHaveText("");
    expect((await row.boundingBox())!.height).toBeLessThanOrEqual(26);

    await row.getByTestId(`commit-open-${sha.slice(0, 7)}`).click();
    const detail = testPage.getByTestId("commit-detail-content");
    await expect(detail).toBeVisible({ timeout: 15_000 });
    await expect(detail.getByTestId("commit-file-index-entry")).toHaveCount(2, {
      timeout: 15_000,
    });

    const indexToggle = detail.getByTestId("commit-file-index-toggle");
    await expectControlHeight(indexToggle, 28);
    await expect(indexToggle).toHaveAttribute("aria-expanded", "true");
    await indexToggle.click();
    await expect(indexToggle).toHaveAttribute("aria-expanded", "false");
    await expect(detail.getByTestId("commit-file-index").locator("ol")).toBeHidden();
    await indexToggle.click();
    await expect(detail.getByTestId("commit-file-index-entry")).toHaveCount(2);
    await prCapture.screenshot("commit-file-navigation-desktop-detail", {
      caption: "Commit detail with a collapsible file index",
    });

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    const inlineGroup = testPage.locator(`#${await toggle.getAttribute("aria-controls")}`);
    const inlineFile = inlineGroup.getByTestId("commit-file-navigation-one.ts");
    const secondInlineFile = inlineGroup.getByTestId("commit-file-navigation-two.ts");
    await expect(inlineFile).toBeVisible({ timeout: 15_000 });
    await expect(secondInlineFile).toBeVisible();
    const followingCommit = testPage.getByTestId(`commit-row-${olderSha.slice(0, 7)}`);
    await expect(followingCommit).toBeVisible();
    const inlineGroupId = await inlineGroup.getAttribute("id");
    await expect
      .poll(() =>
        testPage.evaluate(
          ({ commitSha, nextCommitSha, inlineGroupId }) => {
            const inlineGroup = document.getElementById(inlineGroupId)!;
            const commit = document.querySelector<HTMLElement>(
              `[data-testid="commit-row-${commitSha}"]`,
            )!;
            const firstFile = inlineGroup.querySelector<HTMLElement>(
              '[data-testid="commit-file-navigation-one.ts"]',
            )!;
            const secondFile = inlineGroup.querySelector<HTMLElement>(
              '[data-testid="commit-file-navigation-two.ts"]',
            )!;
            const nextCommit = document.querySelector<HTMLElement>(
              `[data-testid="commit-row-${nextCommitSha}"]`,
            )!;
            return {
              headerToFirstFile:
                firstFile.getBoundingClientRect().top -
                commit.closest<HTMLElement>("[data-changes-timeline-row]")!.getBoundingClientRect()
                  .bottom,
              fileSiblingGap:
                secondFile.getBoundingClientRect().top - firstFile.getBoundingClientRect().bottom,
              commitFooterAndSiblingGap:
                nextCommit
                  .closest<HTMLElement>("[data-changes-timeline-row]")!
                  .getBoundingClientRect().top - secondFile.getBoundingClientRect().bottom,
            };
          },
          {
            commitSha: sha.slice(0, 7),
            nextCommitSha: olderSha.slice(0, 7),
            inlineGroupId: inlineGroupId!,
          },
        ),
      )
      .toEqual({
        headerToFirstFile: 0,
        fileSiblingGap: 2,
        commitFooterAndSiblingGap: 6,
      });
    const dirtyFile = testPage.getByTestId("file-row-navigation-one.ts");
    await expect(dirtyFile).toBeVisible({ timeout: 15_000 });
    const inlineFileBox = await inlineFile.boundingBox();
    const dirtyFileBox = await dirtyFile.boundingBox();
    expect(inlineFileBox?.height).toBe(dirtyFileBox?.height);
    const inlineStatus = inlineFile.locator("[data-file-status]");
    const dirtyStatus = dirtyFile.locator("[data-file-status]");
    expect((await inlineStatus.boundingBox())?.width).toBe(
      (await dirtyStatus.boundingBox())?.width,
    );
    await prCapture.screenshot("commit-file-navigation-desktop-inline", {
      caption: "Expanded commit files reuse the changes tree styling",
    });

    await expect(toggle).toBeFocused();
    for (let step = 0; step < 10; step += 1) {
      await testPage.keyboard.press("ArrowDown");
      if (await inlineFile.evaluate((element) => element === document.activeElement)) break;
    }
    await expect(inlineFile).toBeFocused();
    await testPage.keyboard.press("Enter");
    const selectedHeader = detail.locator(
      'section[data-file-path="navigation-one.ts"] button[data-file-path]',
    );
    await expect(selectedHeader).toBeFocused({ timeout: 15_000 });
    await expect(selectedHeader).toHaveAttribute("aria-expanded", "true");

    await inlineFile.click();

    await expect(selectedHeader).toBeFocused({ timeout: 15_000 });
    await expect(selectedHeader).toHaveAttribute("aria-expanded", "true");
  });

  test("uses touch-sized commit controls on a coarse-pointer tablet", async ({
    tabletTestPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await tabletTestPage.setViewportSize({ width: 1024, height: 900 });
    await expect
      .poll(() => tabletTestPage.evaluate(() => matchMedia("(pointer: coarse)").matches))
      .toBe(true);
    const profile = await createStandardProfile(apiClient, "Tablet Commit File Navigation Profile");
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Tablet Commit File Navigation",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("tablet/navigation.ts", "export const tablet = true;\n");
    git.stageAll();
    const sha = git.commit("Add tablet commit navigation");

    const session = await openTaskSession(tabletTestPage, "Tablet Commit File Navigation");
    await session.waitForChatIdle({ timeout: 30_000 });

    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await expect(tabletTestPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });
    await session.expandCommitsSection();

    const row = tabletTestPage.getByTestId(`commit-row-${sha.slice(0, 7)}`);
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expectTouchControl(row.getByTestId("commit-toggle"));
    await expectTouchControl(row.getByTestId(`commit-open-${sha.slice(0, 7)}`));

    await row.getByTestId(`commit-open-${sha.slice(0, 7)}`).click();
    const detail = tabletTestPage.getByTestId("commit-detail-content");
    await expect(detail).toBeVisible({ timeout: 15_000 });
    await expectTouchControl(detail.getByTestId("commit-file-index-toggle"));
    await expectTouchControl(detail.getByRole("button", { name: /More actions for/ }).first());
  });
});
