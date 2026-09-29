import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { waitForFiniteAnimations } from "../../helpers/animations";
import type { Page } from "@playwright/test";
import path from "node:path";

async function openMobileChangesPanel(testPage: Page) {
  await testPage
    .getByRole("navigation")
    .getByRole("button", { name: /Changes$/ })
    .tap();
  await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
}

async function expandSection(testPage: Page, sectionTestId: string) {
  const toggle = testPage.getByTestId(`${sectionTestId}-collapse-toggle`);
  await expect(toggle).toBeVisible({ timeout: 10_000 });
  // Mirror session-page expandChangesSection: late defaultCollapsed resyncs
  // can re-collapse after the first tap, so retry until expanded sticks.
  await expect
    .poll(
      async () => {
        if ((await toggle.getAttribute("aria-expanded")) === "true") {
          return true;
        }
        await toggle.tap();
        return (await toggle.getAttribute("aria-expanded")) === "true";
      },
      { timeout: 15_000 },
    )
    .toBe(true);
}

async function expectDiffText(testPage: Page, text: string, timeout = 45_000) {
  await testPage.waitForFunction(
    (searchText: string) => {
      for (const container of document.querySelectorAll("diffs-container")) {
        const shadow = container.shadowRoot;
        if (shadow?.textContent?.includes(searchText)) return true;
      }
      return false;
    },
    text,
    { timeout },
  );
}

async function expectDiffTextAbsent(testPage: Page, text: string, timeout = 10_000) {
  await expect
    .poll(
      () =>
        testPage.evaluate((searchText: string) => {
          const visibleContainers = Array.from(document.querySelectorAll("diffs-container")).filter(
            (container) => {
              const bounds = container.getBoundingClientRect();
              const style = window.getComputedStyle(container);
              return (
                bounds.width > 0 &&
                bounds.height > 0 &&
                style.display !== "none" &&
                style.visibility !== "hidden"
              );
            },
          );
          return visibleContainers.every(
            (container) => !container.shadowRoot?.textContent?.includes(searchText),
          );
        }, text),
      { timeout },
    )
    .toBe(true);
}

test.describe("Mobile changes panel", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test.beforeEach(({ backend }) => {
    // The worker reuses its fixture repository across tests. Restore a clean
    // working tree so staged/untracked files from an earlier scenario cannot
    // move and remount the PR Changes section during async status hydration.
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.exec("git reset --hard HEAD");
    git.exec("git clean -fd");
  });

  // @covers AC-UI-CHANGES-FILE-ROW-CONTAINMENT-002.1 through 002.3
  test("compact file rows preserve names and expose touch actions", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    const initialLayout = (await apiClient.getUserSettings()).settings.changes_panel_layout;
    try {
      await apiClient.saveUserSettings({ changes_panel_layout: "tree" });
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Improve mobile changes",
        seedData.agentProfileId,
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

      const git = new GitHelper(
        path.join(backend.tmpDir, "repos", "e2e-repo"),
        makeGitEnv(backend.tmpDir),
      );
      const filePath = "apps/web/components/task/status-surface-metrics.test.ts";
      const longPath =
        "apps/web/components/task/mobile-resource-metrics-display-with-a-long-filename.test.ts";
      git.createFile(filePath, "READABLE_FILE_MARKER\n");
      git.createFile(longPath, "LONG_FILE_MARKER\n");
      git.createFile("docs/public/operations.md", "Operations\n");
      git.createFile("docs/plans/mobile-status/plan.md", "Mobile status plan\n");
      await openMobileChangesPanel(testPage);
      await expandSection(testPage, "unstaged-files-section");
      const section = testPage.getByTestId("unstaged-files-section");
      const rowId = `file-row-${filePath.replaceAll("/", "-")}`;
      const row = section.getByTestId(rowId);
      await expect(row).toBeVisible();
      await prCapture.screenshot("compact-changes-tree", {
        caption: "Phone Changes list gives filenames the primary width and groups file actions.",
      });

      const identity = row.getByTitle(filePath, { exact: true });
      const rowBox = (await row.boundingBox())!;
      const identityBox = (await identity.boundingBox())!;
      expect(identityBox.width / rowBox.width).toBeGreaterThan(0.65);
      expect(rowBox.height).toBeLessThanOrEqual(56);
      expect(identityBox.height).toBeGreaterThanOrEqual(44);
      await expect(identity.getByText("status-surface-metrics.test.ts", { exact: true })).toHaveCSS(
        "white-space",
        "normal",
      );
      const longName = section.getByText(
        "mobile-resource-metrics-display-with-a-long-filename.test.ts",
        { exact: true },
      );
      expect(await longName.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
      const more = row.getByRole("button", { name: "Show more actions" });
      const moreBox = (await more.boundingBox())!;
      expect(moreBox.width).toBeGreaterThanOrEqual(44);
      expect(moreBox.height).toBeGreaterThanOrEqual(44);
      expect(moreBox.x + moreBox.width).toBeLessThanOrEqual(rowBox.x + rowBox.width);
      expect(
        await more.evaluate((el) => {
          const box = el.getBoundingClientRect();
          return el.contains(
            document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
          );
        }),
      ).toBe(true);

      await more.tap();
      const actionsMenu = testPage.getByRole("menu");
      await waitForFiniteAnimations(actionsMenu);
      const copyPath = actionsMenu.getByRole("menuitem", { name: "Copy path" });
      await expect(copyPath).toBeVisible();
      const copyPathBox = (await copyPath.boundingBox())!;
      expect(copyPathBox.width).toBeGreaterThanOrEqual(44);
      expect(copyPathBox.height).toBeGreaterThanOrEqual(44);
      await copyPath.tap();
      await expect
        .poll(() => testPage.evaluate(() => navigator.clipboard.readText()))
        .toBe(filePath);
      await expect(testPage.locator("diffs-container")).toHaveCount(0);

      await apiClient.saveUserSettings({ changes_panel_layout: "flat" });
      await testPage.reload();
      await session.waitForLoad();
      await openMobileChangesPanel(testPage);
      await expandSection(testPage, "unstaged-files-section");
      await expect(row).toBeVisible();
      const folder = identity.getByText("apps/web/components/task", { exact: true });
      await expect(folder).toBeVisible();
      const basenameBox = (await identity
        .getByText("status-surface-metrics.test.ts", { exact: true })
        .boundingBox())!;
      expect((await folder.boundingBox())!.y).toBeGreaterThanOrEqual(
        basenameBox.y + basenameBox.height,
      );
      expect((await row.boundingBox())!.height).toBeLessThanOrEqual(56);
      await prCapture.screenshot("compact-changes-list", {
        caption: "Flat phone list with full basenames, directory context, and compact actions.",
      });
      for (const width of [767, 393]) {
        await testPage.setViewportSize({ width, height: 851 });
        if (width === 393) await openMobileChangesPanel(testPage);
        await expect(more).toBeVisible();
        expect((await more.boundingBox())!.width).toBeGreaterThanOrEqual(44);
        await expect(
          identity.getByText("status-surface-metrics.test.ts", { exact: true }),
        ).toHaveCSS("white-space", "normal");
        expect(
          await testPage.evaluate(
            () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
          ),
        ).toBe(true);
      }
      await more.tap();
      const menu = testPage.getByRole("menu");
      await expect(menu).toBeVisible();
      await expect(menu.getByText(filePath, { exact: true })).toBeVisible();
      await waitForFiniteAnimations(menu);
      const menuBox = (await menu.boundingBox())!;
      expect(menuBox.x).toBeGreaterThanOrEqual(0);
      expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
      await prCapture.screenshot("compact-file-actions", {
        caption: "Full file path and touch actions in the phone menu.",
      });
      await testPage.keyboard.press("Escape");
      await expect(more).toBeFocused();
      await more.tap();
      await menu.getByRole("menuitem", { name: "Stage file", exact: true }).tap();
      await expandSection(testPage, "staged-files-section");
      const stagedRow = testPage.getByTestId("staged-files-section").getByTestId(rowId);
      await expect(stagedRow).toBeVisible();
      await expect(testPage.getByTestId("mobile-diff-sheet-close")).not.toBeVisible();
      await stagedRow.getByRole("button", { name: "Show more actions" }).tap();
      await menu.getByRole("menuitem", { name: "Unstage file", exact: true }).tap();
      await expect(row).toBeVisible();
      await more.tap();
      await menu.getByRole("menuitem", { name: "Discard changes", exact: true }).tap();
      await expect(testPage.getByTestId("discard-local-changes-confirm")).toBeVisible();
      await testPage.getByRole("button", { name: "Cancel", exact: true }).tap();
      await expect(row).toBeVisible();
      expect(git.exec("git status --porcelain --untracked-files=all")).toContain(filePath);

      await identity.tap();
      await expectDiffText(testPage, "READABLE_FILE_MARKER");
      await testPage.getByTestId("mobile-diff-sheet-close").tap();
      await more.tap();
      await menu.getByRole("menuitem", { name: "Edit", exact: true }).tap();
      await expect(testPage.getByTestId("mobile-changes-panel")).not.toBeVisible();
      const viewer = testPage.getByTestId("mobile-file-viewer-panel");
      await expect(viewer).toBeVisible();
      await expect(viewer.getByText(filePath, { exact: true })).toBeVisible();
      expect((await apiClient.getUserSettings()).settings.changes_panel_layout).toBe("flat");
    } finally {
      await apiClient.saveUserSettings({ changes_panel_layout: initialLayout });
    }
  });

  // @covers AC-UI-CHANGES-FILE-ROW-CONTAINMENT-002.1
  test("deep tree rows keep filenames and actions within the panel", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const initialLayout = (await apiClient.getUserSettings()).settings.changes_panel_layout;
    try {
      await apiClient.saveUserSettings({ changes_panel_layout: "tree" });
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Deep changes tree",
        seedData.agentProfileId,
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
      const git = new GitHelper(
        path.join(backend.tmpDir, "repos", "e2e-repo"),
        makeGitEnv(backend.tmpDir),
      );
      let folder = "src";
      for (let depth = 0; depth < 22; depth++) {
        git.createFile(`${folder}/sibling.ts`, "sibling\n");
        folder += `/level-${depth}`;
      }
      const name = "deeply-nested-file.test.ts";
      const filePath = `${folder}/${name}`;
      git.createFile(filePath, "DEEP_FILE_MARKER\n");
      await openMobileChangesPanel(testPage);
      await expandSection(testPage, "unstaged-files-section");
      const row = testPage.getByTestId(`file-row-${filePath.replaceAll("/", "-")}`);
      await row.scrollIntoViewIfNeeded();
      const rowBox = (await row.boundingBox())!;
      const filenameBox = (await row.getByText(name, { exact: true }).boundingBox())!;
      expect(filenameBox.width).toBeGreaterThan(rowBox.width * 0.5);
      const statusBox = (await row.locator("[data-file-status]").boundingBox())!;
      const menuButton = row.getByRole("button", { name: "Show more actions" });
      const menuBox = (await menuButton.boundingBox())!;
      expect(statusBox.x + statusBox.width).toBeLessThanOrEqual(menuBox.x);
      expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(rowBox.x + rowBox.width);
      await menuButton.tap();
      await expect(testPage.getByRole("menu").getByText(filePath, { exact: true })).toBeVisible();
      await testPage.keyboard.press("Escape");
      await row.getByTitle(filePath, { exact: true }).tap();
      await expectDiffText(testPage, "DEEP_FILE_MARKER");
    } finally {
      await apiClient.saveUserSettings({ changes_panel_layout: initialLayout });
    }
  });

  test("renders timeline surface and opens Diff/Review/file/commit overlays", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Changes Surface",
      seedData.agentProfileId,
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

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("mobile-committed.txt", "base commit marker");
    git.stageAll();
    git.commit("mobile commit seed");
    git.createFile("mobile-unstaged.txt", "UNSTAGED_MARKER");
    git.createFile("mobile-staged.txt", "STAGED_MARKER");
    git.stageFile("mobile-staged.txt");

    await openMobileChangesPanel(testPage);

    // Regression: mobile Changes tab should be timeline summary, not inline merged diff.
    await expect(testPage.locator("diffs-container")).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-changes-panel").getByRole("tab")).toHaveCount(0);
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByTestId("staged-files-section")).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });

    const mobileChangesPanel = testPage.getByTestId("mobile-changes-panel");

    await mobileChangesPanel.getByRole("button", { name: "Diff" }).tap();
    const closeButton = testPage.getByTestId("mobile-diff-sheet-close");
    await expect(closeButton).toBeVisible({ timeout: 10_000 });

    const committedTab = testPage.getByRole("tab", { name: /^Committed/i });
    await expect(testPage.getByRole("tab", { name: /^Uncommitted/i })).toBeVisible();
    await expect(committedTab).toBeVisible();
    await committedTab.tap();
    await expect(committedTab).toHaveAttribute("aria-selected", "true");
    await closeButton.tap();

    await mobileChangesPanel.getByRole("button", { name: "Review" }).tap();
    const reviewDialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
    await testPage.keyboard.press("Escape");
    await expect(reviewDialog).not.toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("file-row-mobile-unstaged.txt").tap();
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "UNSTAGED_MARKER");

    // Regression: mobile diffs must use word-wrap (overflow="wrap") so long
    // lines are readable without horizontal scroll on touch devices.
    // @pierre/diffs 1.1.x renamed the attribute on the rendered <pre> from
    // `data-diffs` to `data-diff` — match either to keep the test stable
    // across the upgrade.
    const overflowAttr = await testPage.waitForFunction(
      () =>
        document
          .querySelector("diffs-container")
          ?.shadowRoot?.querySelector("[data-diff], [data-diffs]")
          ?.getAttribute("data-overflow"),
      { timeout: 10_000 },
    );
    expect(await overflowAttr.jsonValue()).toBe("wrap");

    await closeButton.tap();

    await expandSection(testPage, "commits-section");
    await testPage
      .locator("[data-testid^='commit-row-']")
      .first()
      .getByRole("button", { name: "Open commit" })
      .tap();
    await expect(testPage.getByText("Commit Changes")).toBeVisible({ timeout: 10_000 });
    await closeButton.tap();
  });

  test("tapping a staged file row opens file diff sheet", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Staged File Diff",
      seedData.agentProfileId,
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

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("mobile-staged-only.txt", "STAGED_ONLY_MARKER");
    git.stageFile("mobile-staged-only.txt");

    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "staged-files-section");

    await testPage.getByTestId("file-row-mobile-staged-only.txt").tap();
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "STAGED_ONLY_MARKER");
    await testPage.getByTestId("mobile-diff-sheet-close").tap();
  });

  // @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.12
  test("shows same path in staged and unstaged sections", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Mixed Change",
      seedData.agentProfileId,
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

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("mobile-mixed-layer.txt", "base\n");
    git.stageFile("mobile-mixed-layer.txt");
    git.commit("Add mobile mixed layer fixture");
    git.modifyFile("mobile-mixed-layer.txt", "base\nMOBILE_STAGED_LAYER_MARKER\n");
    git.stageFile("mobile-mixed-layer.txt");
    git.modifyFile(
      "mobile-mixed-layer.txt",
      "base\nMOBILE_STAGED_LAYER_MARKER\nMOBILE_UNSTAGED_LAYER_MARKER\n",
    );

    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "unstaged-files-section");
    await expandSection(testPage, "staged-files-section");

    const unstagedRow = testPage
      .getByTestId("unstaged-files-section")
      .getByTestId("file-row-mobile-mixed-layer.txt");
    const stagedRow = testPage
      .getByTestId("staged-files-section")
      .getByTestId("file-row-mobile-mixed-layer.txt");
    await expect(unstagedRow).toBeVisible();
    await expect(stagedRow).toBeVisible();
    await expect(unstagedRow.getByText("+1", { exact: true })).toBeVisible();
    await expect(stagedRow.getByText("+1", { exact: true })).toBeVisible();

    await unstagedRow.tap();
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "MOBILE_UNSTAGED_LAYER_MARKER");
    await testPage.getByTestId("mobile-diff-sheet-close").tap();

    await stagedRow.tap();
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "MOBILE_STAGED_LAYER_MARKER");
    await expectDiffTextAbsent(testPage, "MOBILE_UNSTAGED_LAYER_MARKER");
    await testPage.getByTestId("mobile-diff-sheet-close").tap();

    await stagedRow.getByRole("button", { name: "Show more actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Unstage file", exact: true }).tap();
    await expect(stagedRow).toHaveCount(0);
    await expect(unstagedRow).toBeVisible();
    await expect(unstagedRow.getByText("+2", { exact: true })).toBeVisible();
  });

  test("tapping a PR file row opens file diff sheet with diff content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const checkoutBranch = "main";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile PR File Diff",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, checkout_branch: checkoutBranch }],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 42,
        title: "Mobile PR file diff test",
        state: "open",
        head_branch: checkoutBranch,
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 3,
        deletions: 0,
      },
    ]);
    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 42, [
      {
        filename: "mobile-pr-fix.txt",
        status: "added",
        additions: 3,
        deletions: 0,
        patch:
          "@@ -0,0 +1,3 @@\n+PR_FILE_MARKER_LINE_ONE\n+PR_FILE_MARKER_LINE_TWO\n+PR_FILE_MARKER_LINE_THREE",
      },
    ]);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 42,
      pr_url: "https://github.com/testorg/testrepo/pull/42",
      pr_title: "Mobile PR file diff test",
      head_branch: checkoutBranch,
      base_branch: "main",
      author_login: "test-user",
    });

    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "pr-changes-section");

    const prFilesList = testPage.getByTestId("pr-files-list");
    await expect(prFilesList).toBeVisible({ timeout: 20_000 });
    await prFilesList.getByText("mobile-pr-fix.txt").tap();

    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "PR_FILE_MARKER_LINE_ONE");

    await testPage.getByTestId("mobile-diff-sheet-close").tap();
  });

  test("PR-only commit opens the remote commit sheet when local history is stale", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const checkoutBranch = "main";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile PR-only Commit Detail",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, checkout_branch: checkoutBranch }],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("mobile-pr-shared-marker.ts", "shared provider checkout commit");
    git.stageFile("mobile-pr-shared-marker.ts");
    const sharedSha = git.commit("Shared provider checkout commit");

    const remoteSha = "e".repeat(40);
    const remoteMessage = "Mobile force-pushed commit";
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("mobile-remote-author");
    await apiClient.mockGitHubAddPRs([
      {
        number: 2254,
        title: "Mobile force-pushed PR",
        state: "open",
        head_branch: checkoutBranch,
        base_branch: "main",
        author_login: "mobile-remote-author",
        repo_owner: "testorg",
        repo_name: "testrepo",
        head_sha: remoteSha,
      },
    ]);
    await apiClient.mockGitHubAddPRCommits("testorg", "testrepo", 2254, [
      {
        sha: sharedSha,
        message: "Shared provider checkout commit",
        author_login: "mobile-remote-author",
        author_date: "2026-08-03T12:00:00Z",
      },
      {
        sha: remoteSha,
        message: remoteMessage,
        author_login: "mobile-remote-author",
        author_date: "2026-08-04T12:00:00Z",
        stats_available: false,
      },
    ]);
    await apiClient.mockGitHubAddPRCommitDetail("testorg", "testrepo", remoteSha, {
      message: remoteMessage,
      author_login: "mobile-remote-author",
      author_name: "Mobile Remote Author",
      author_date: "2026-08-04T12:00:00Z",
      additions: 1,
      deletions: 0,
      files_changed: 1,
      files: [
        {
          filename: "mobile-pr-only.ts",
          status: "added",
          additions: 1,
          deletions: 0,
          patch: "@@ -0,0 +1 @@\n+MOBILE_PR_ONLY_REMOTE_MARKER",
        },
      ],
    });
    await apiClient.mockGitHubSetPRCommitsFailures("testorg", "testrepo", 2254, 1);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 2254,
      pr_url: "https://github.com/testorg/testrepo/pull/2254",
      pr_title: "Mobile force-pushed PR",
      head_branch: checkoutBranch,
      base_branch: "main",
      author_login: "mobile-remote-author",
    });

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "commits-section");

    const row = testPage.getByTestId(`commit-row-${remoteSha.slice(0, 7)}`);
    await expect(row).toBeVisible({ timeout: 20_000 });
    const sharedRow = testPage.getByTestId(`commit-row-${sharedSha.slice(0, 7)}`);
    await expect(sharedRow).toBeVisible({ timeout: 20_000 });
    await expect(sharedRow.getByTestId("commit-provenance")).toHaveAttribute(
      "data-commit-provenance",
      "pushed",
    );
    await expect(testPage.getByTestId("header-remote-contribution-warning")).toHaveCount(0);
    await expect(row.getByText("+0", { exact: true })).toHaveCount(0);
    await expect(row.getByText("-0", { exact: true })).toHaveCount(0);
    await row.getByTestId(`commit-open-${remoteSha.slice(0, 7)}`).tap();

    await expect(testPage.getByText("Commit Changes")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByLabel("Commit Changes").getByText(remoteMessage)).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByText("Mobile Remote Author")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "MOBILE_PR_ONLY_REMOTE_MARKER");
    await expect(testPage.getByTestId("mobile-diff-sheet-close")).toBeVisible();
    await expect(testPage.getByTestId("mobile-diff-sheet-close")).toHaveCount(1);
    await expect(testPage.locator("body")).toHaveCSS("overflow-x", "hidden");
    await testPage.getByTestId("mobile-diff-sheet-close").tap();
    await expect(testPage.getByTestId("mobile-diff-sheet-close")).not.toBeVisible({
      timeout: 10_000,
    });
  });

  test("tapping PR file row shows PR diff when same file also has local changes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const checkoutBranch = "main";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile PR Overlap Diff",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, checkout_branch: checkoutBranch }],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 44,
        title: "Mobile overlap PR diff test",
        state: "open",
        head_branch: checkoutBranch,
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 2,
        deletions: 0,
      },
    ]);
    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 44, [
      {
        filename: "overlap-mobile.txt",
        status: "added",
        additions: 2,
        deletions: 0,
        patch: "@@ -0,0 +1,2 @@\n+MOBILE_OVERLAP_PR_MARKER_A\n+MOBILE_OVERLAP_PR_MARKER_B",
      },
    ]);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 44,
      pr_url: "https://github.com/testorg/testrepo/pull/44",
      pr_title: "Mobile overlap PR diff test",
      head_branch: checkoutBranch,
      base_branch: "main",
      author_login: "test-user",
    });

    // Create local change to the same file — this causes allFiles to deduplicate
    // the PR entry, which was the bug: tapping the PR row showed "No changes".
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("overlap-mobile.txt", "local change LOCAL_CHANGE_MARKER");

    await openMobileChangesPanel(testPage);

    // Wait for the local status refresh triggered by createFile before opening
    // the PR section. Otherwise that refresh can remount the overlapping PR row
    // while Playwright is dispatching the tap.
    await expect(testPage.getByTestId("file-row-overlap-mobile.txt")).toBeVisible({
      timeout: 20_000,
    });
    await expandSection(testPage, "pr-changes-section");

    const prFilesList = testPage.getByTestId("pr-files-list");
    await expect(prFilesList).toBeVisible({ timeout: 20_000 });
    await prFilesList.getByText("overlap-mobile.txt").tap();

    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    // PR diff content must appear — not "No changes"
    await expectDiffText(testPage, "MOBILE_OVERLAP_PR_MARKER_A");

    await testPage.getByTestId("mobile-diff-sheet-close").tap();

    git.exec("git clean -fd");
    try {
      git.exec("git checkout -- .");
    } catch {
      // Ignore checkout errors if repo has no tracked files
    }
  });

  test("Diff sheet auto-selects Committed tab when no uncommitted changes exist", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Auto-Select Source",
      seedData.agentProfileId,
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

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("committed-only.txt", "committed content");
    git.stageAll();
    git.commit("committed-only seed");

    await openMobileChangesPanel(testPage);
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });

    const mobileChangesPanel = testPage.getByTestId("mobile-changes-panel");
    await mobileChangesPanel.getByRole("button", { name: "Diff" }).tap();
    const closeButton = testPage.getByTestId("mobile-diff-sheet-close");
    await expect(closeButton).toBeVisible({ timeout: 10_000 });

    // Single source → no tab bar rendered; title shows source name instead.
    await expect(testPage.getByRole("tab", { name: /^Uncommitted/i })).not.toBeVisible();
    await expect(testPage.getByRole("tab", { name: /^Committed/i })).not.toBeVisible();
    await expect(testPage.getByText("Committed Changes")).toBeVisible({ timeout: 10_000 });

    await closeButton.tap();
  });
});
