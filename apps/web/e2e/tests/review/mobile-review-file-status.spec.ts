import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import path from "node:path";

const MOBILE_FILE = "mobile-review-status-added.ts";
const MOBILE_MOVED_FROM_FILE = "mobile-review-status-old-name.ts";
const MOBILE_MOVED_FILE = "mobile-review-status-new-name.ts";

test.describe("Review file status on mobile", () => {
  test.describe.configure({ timeout: 120_000 });

  test("shows status in the sticky header without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("reviewer");
    await apiClient.mockGitHubAddPRs([
      {
        number: 72,
        title: "Mobile review status cues",
        state: "open",
        head_branch: "feat/mobile-review-status",
        base_branch: "main",
        author_login: "reviewer",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 0,
        deletions: 0,
      },
    ]);
    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 72, [
      {
        filename: MOBILE_MOVED_FILE,
        status: "renamed",
        additions: 0,
        deletions: 0,
        old_path: MOBILE_MOVED_FROM_FILE,
      },
    ]);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Review File Status E2E",
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
      pr_number: 72,
      pr_url: "https://github.com/testorg/testrepo/pull/72",
      pr_title: "Mobile review status cues",
      head_branch: "feat/mobile-review-status",
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
    git.createFile(MOBILE_FILE, "mobile added file\n");
    git.stageFile(MOBILE_FILE);

    await testPage.getByRole("button", { name: "Changes" }).tap();
    const changesPanel = testPage.getByTestId("mobile-changes-panel");
    await expect(changesPanel).toBeVisible();
    await expect(testPage.getByTestId(`file-row-${MOBILE_FILE}`)).toBeVisible({ timeout: 20_000 });
    await changesPanel.getByRole("button", { name: "Review", exact: true }).tap();

    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();
    const header = dialog.getByTestId("review-file-header").filter({ hasText: MOBILE_FILE });
    await expect(header).toBeVisible();
    const marker = header.getByRole("img", { name: "Added" });
    await expect(marker).toBeVisible();

    const movedHeader = dialog
      .getByTestId("review-file-header")
      .filter({ hasText: MOBILE_MOVED_FILE });
    await expect(movedHeader).toBeVisible();
    await expect(
      movedHeader.getByRole("img", { name: `Moved from ${MOBILE_MOVED_FROM_FILE}` }),
    ).toBeVisible();
    await expect(
      dialog.getByText(`Moved from ${MOBILE_MOVED_FROM_FILE}; no textual changes`),
    ).toBeVisible();

    const headerGeometry = await header.evaluate((element) => {
      const markerElement = element.querySelector<HTMLElement>('[data-file-status="added"]');
      if (!markerElement) return null;
      const headerBounds = element.getBoundingClientRect();
      const markerBounds = markerElement.getBoundingClientRect();
      return {
        scrollWidth: element.scrollWidth,
        clientWidth: element.clientWidth,
        headerLeft: headerBounds.left,
        headerRight: headerBounds.right,
        markerLeft: markerBounds.left,
        markerRight: markerBounds.right,
      };
    });
    if (!headerGeometry) throw new Error("Mobile Review header geometry is unavailable");
    expect(headerGeometry.scrollWidth).toBeLessThanOrEqual(headerGeometry.clientWidth);
    expect(headerGeometry.markerLeft).toBeGreaterThanOrEqual(headerGeometry.headerLeft);
    expect(headerGeometry.markerRight).toBeLessThanOrEqual(headerGeometry.headerRight);

    const checkbox = header.getByRole("checkbox");
    await checkbox.tap();
    await expect(checkbox).toHaveAttribute("aria-checked", "true");

    const overflow = await testPage.evaluate(() => ({
      viewport: window.innerWidth,
      document: document.documentElement.scrollWidth,
    }));
    expect(overflow.document).toBeLessThanOrEqual(overflow.viewport + 1);
  });
});
