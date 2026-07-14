import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import { REVIEW_SIDEBAR_LIMITS } from "../../../hooks/use-review-sidebar-resize";
import path from "node:path";

const ADDED_PATH = "review-status-added.ts";
const MODIFIED_PATH = "review-status-modified.ts";
const DELETED_PATH = "review-status-deleted.ts";
const MOVED_FROM_PATH = "review-status-old-name.ts";
const MOVED_PATH = "review-status-a-very-long-new-name-that-must-truncate.ts";

test.describe("Review file status", () => {
  test.describe.configure({ timeout: 120_000 });

  test("shows every status, keeps the marker visible at minimum width, and explains a pure move", async ({
    testPage,
    apiClient,
    seedData,
    backend,
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
    git.createFile(ADDED_PATH, "added\n");
    git.stageFile(ADDED_PATH);
    git.modifyFile(MODIFIED_PATH, "after\n");
    git.deleteFile(DELETED_PATH);

    const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
    await expect(changesTab).toBeVisible();
    await changesTab.click();
    for (const filePath of [ADDED_PATH, MODIFIED_PATH, DELETED_PATH]) {
      await expect(testPage.getByTestId(`file-row-${filePath}`)).toBeVisible({ timeout: 20_000 });
    }

    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();
    const sidebar = dialog.getByTestId("review-dialog-sidebar");
    await expect(sidebar).toBeVisible();

    for (const name of ["Added", "Modified", "Deleted"]) {
      await expect(sidebar.getByRole("img", { name })).toBeVisible();
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

    await movedRow.click();
    await expect(
      dialog.getByText(`Moved from ${MOVED_FROM_PATH}; no textual changes`),
    ).toBeVisible();
    await expect(dialog.getByText("Loading diff...")).toHaveCount(0);
  });
});
