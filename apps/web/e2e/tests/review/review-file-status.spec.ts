import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { getSingleLineTextInVisualOrder } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import { REVIEW_SIDEBAR_LIMITS } from "../../../hooks/use-review-sidebar-resize";
import path from "node:path";

const ADDED_PATH = "review-status-added.ts";
const DOTTED_PATH = ".agents/skills/pr-fixup/SKILL.md";
const NESTED_PATH = "review-status-nested/nested.ts";
const MODIFIED_PATH = "review-status-modified.ts";
const DELETED_PATH = "review-status-deleted.ts";
const MOVED_FROM_PATH = "review-status-old-name.ts";
const MOVED_PATH = "review-status-a-very-long-new-name-that-must-truncate.ts";

test.describe("Review file status", () => {
  test.describe.configure({ timeout: 120_000 });

  // @covers AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.4
  test("shows every status, keeps the marker visible at minimum width, and explains a pure move", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await testPage.addInitScript(({ key, width }) => sessionStorage.setItem(key, width), {
      key: REVIEW_SIDEBAR_LIMITS.storageKey,
      width: String(REVIEW_SIDEBAR_LIMITS.minWidth),
    });

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("reviewer");
    await apiClient.mockGitHubAddPRs([
      {
        number: 71,
        title: "Review status cues",
        state: "open",
        head_branch: "feat/review-status",
        base_branch: "main",
        author_login: "reviewer",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 0,
        deletions: 0,
      },
    ]);
    const movedFiles = [
      {
        filename: MOVED_PATH,
        status: "renamed",
        additions: 0,
        deletions: 0,
        old_path: MOVED_FROM_PATH,
      },
    ];
    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 71, movedFiles);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Review File Status E2E",
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
      owner: "testorg",
      repo: "testrepo",
      pr_number: 71,
      pr_url: "https://github.com/testorg/testrepo/pull/71",
      pr_title: "Review status cues",
      head_branch: "feat/review-status",
      base_branch: "main",
      author_login: "reviewer",
      additions: 0,
      deletions: 0,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();

    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(MODIFIED_PATH, "before\n");
    git.createFile(DELETED_PATH, "remove me\n");
    git.stageAll();
    git.commit("seed review status files");
    git.createFile(ADDED_PATH, "added line\n".repeat(80));
    git.stageFile(ADDED_PATH);
    git.createFile(NESTED_PATH, "nested\n");
    git.stageFile(NESTED_PATH);
    git.createFile(DOTTED_PATH, "dot-prefixed directory\n");
    git.stageFile(DOTTED_PATH);
    git.modifyFile(MODIFIED_PATH, "after\n");
    git.deleteFile(DELETED_PATH);

    const changesTab = testPage.getByTestId("dockview-tab-changes");
    await expect(changesTab).toBeVisible();
    await changesTab.click();
    for (const filePath of [ADDED_PATH, DOTTED_PATH, NESTED_PATH, MODIFIED_PATH, DELETED_PATH]) {
      const rowTestId = `file-row-${filePath.replace(/[/\\]/g, "-")}`;
      await expect(testPage.getByTestId(rowTestId)).toBeVisible({ timeout: 20_000 });
    }

    await testPage
      .getByTestId("changes-panel")
      .getByRole("button", { name: "Diff", exact: true })
      .click();
    await testPage.getByRole("button", { name: "Expand review" }).click();
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();
    const sidebar = dialog.getByTestId("review-dialog-sidebar");
    await expect(sidebar).toBeVisible();

    const dottedHeader = dialog.locator(
      `[data-testid="review-file-header"][data-file-path="${DOTTED_PATH}"]`,
    );
    await expect(dottedHeader).toBeVisible();
    const dottedCollapseButton = dottedHeader.getByRole("button", {
      name: `Collapse ${DOTTED_PATH}`,
    });
    const dottedDirectory = dottedCollapseButton.locator("[data-review-file-directory]");
    await expect(dottedDirectory).toHaveText(path.dirname(DOTTED_PATH));
    expect(await getSingleLineTextInVisualOrder(dottedDirectory)).toBe(path.dirname(DOTTED_PATH));
    await expect(dottedHeader.locator("[data-review-file-name]")).toHaveText(
      path.basename(DOTTED_PATH),
    );
    await expect(dottedCollapseButton).toBeVisible();

    const sidebarOrder = await sidebar
      .getByTestId("review-file-row")
      .evaluateAll((rows) => rows.map((row) => (row as HTMLElement).dataset.filePath));
    const diffOrder = await dialog
      .getByTestId("review-file-header")
      .evaluateAll((headers) => headers.map((header) => (header as HTMLElement).dataset.filePath));
    expect(sidebarOrder).toEqual(diffOrder);
    const totalFiles = sidebarOrder.length;
    expect(totalFiles).toBeGreaterThanOrEqual(5);

    for (const [path, name] of [
      [ADDED_PATH, "Added"],
      [MODIFIED_PATH, "Modified"],
      [DELETED_PATH, "Deleted"],
    ] as const) {
      const row = sidebar.locator(`[data-testid="review-file-row"][data-file-path="${path}"]`);
      await expect(row.getByRole("img", { name })).toBeVisible();
    }
    await expect(sidebar.getByRole("img", { name: `Moved from ${MOVED_FROM_PATH}` })).toBeVisible({
      timeout: 20_000,
    });

    const movedRow = sidebar.locator(
      `[data-testid="review-file-row"][data-file-path="${MOVED_PATH}"]`,
    );
    const movedMarker = movedRow.locator('[data-file-status="renamed"]');
    await expect(movedMarker).toBeVisible();
    await expect
      .poll(async () => Math.round((await sidebar.boundingBox())?.width ?? 0))
      .toBeGreaterThanOrEqual(REVIEW_SIDEBAR_LIMITS.minWidth - 2);
    const sidebarBox = await sidebar.boundingBox();
    if (!sidebarBox) throw new Error("Review sidebar has no bounding box");
    expect(sidebarBox.width).toBeLessThanOrEqual(REVIEW_SIDEBAR_LIMITS.minWidth + 2);

    const geometry = await movedRow.evaluate((row) => {
      const name = row.querySelector<HTMLElement>("[data-review-file-name]");
      const marker = row.querySelector<HTMLElement>('[data-file-status="renamed"]');
      const rowBounds = row.getBoundingClientRect();
      const sidebarBounds = row
        .closest<HTMLElement>('[data-testid="review-dialog-sidebar"]')
        ?.getBoundingClientRect();
      if (!name || !marker || !sidebarBounds) return null;
      const nameBounds = name.getBoundingClientRect();
      const markerBounds = marker.getBoundingClientRect();
      return {
        nameRight: nameBounds.right,
        markerLeft: markerBounds.left,
        markerRight: markerBounds.right,
        rowRight: rowBounds.right,
        sidebarRight: sidebarBounds.right,
      };
    });
    if (!geometry) throw new Error("Moved row geometry is unavailable");
    expect(geometry.nameRight).toBeLessThanOrEqual(geometry.markerLeft);
    expect(geometry.markerRight).toBeLessThanOrEqual(geometry.rowRight);
    expect(geometry.markerRight).toBeLessThanOrEqual(geometry.sidebarRight);

    const reviewProgress = dialog.getByText(new RegExp(`^\\d+ of ${totalFiles} files reviewed$`));
    await expect(reviewProgress).toHaveText(`0 of ${totalFiles} files reviewed`);
    await sidebar
      .locator(`[data-testid="review-file-row"][data-file-path="${NESTED_PATH}"]`)
      .click();
    const reviewScroll = dialog.getByTestId("review-diff-scroll");
    await expect
      .poll(() => reviewScroll.evaluate((element) => element.scrollTop))
      .toBeGreaterThan(0);
    await dwell(
      testPage,
      600,
      "negative-assertion",
      "asserts that jumping to a file never marks it reviewed; the count staying at zero is the absence of an event, so a regression needs the auto-review window to elapse to have room to fire",
    );
    await expect(reviewProgress).toHaveText(`0 of ${totalFiles} files reviewed`);
    await prCapture.screenshot("review-ordered-safe-jump", {
      caption: "Review keeps tree and diff order aligned without auto-reviewing a file jump",
    });

    await reviewScroll.evaluate((element) => {
      element.scrollTop = 0;
    });
    await expect.poll(() => reviewScroll.evaluate((element) => element.scrollTop)).toBe(0);
    await reviewScroll.hover();
    await testPage.mouse.wheel(0, 500);
    await expect
      .poll(async () => Number.parseInt((await reviewProgress.textContent()) ?? "0", 10))
      .toBeGreaterThan(0);

    await movedRow.click();
    const movedHeader = dialog.locator(
      `[data-testid="review-file-header"][data-file-path="${MOVED_PATH}"]`,
    );
    const movedSection = movedHeader.locator("..");
    await expect(
      movedSection.getByText(`Moved from ${MOVED_FROM_PATH}; no textual changes`),
    ).toBeVisible();
    await expect(movedSection.getByText("Loading diff...")).toHaveCount(0);
  });
});
