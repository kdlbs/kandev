import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { getSingleLineTextInVisualOrder } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import path from "node:path";

const MOBILE_FILE =
  ".agents/skills/review/surfaces/with-a-deliberately-long-directory/deeply/nested/mobile-review-status-added.ts";
const MOBILE_MOVED_FROM_FILE = "mobile-review-status-old-name.ts";
const MOBILE_MOVED_FILE = "mobile-review-status-new-name.ts";

test.describe("Review file status on mobile", () => {
  test.describe.configure({ timeout: 120_000 });

  test("uses a compact file header with a labelled mobile actions menu", async ({
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
    await expect(
      testPage.getByTestId(`file-row-${MOBILE_FILE.replace(/[/\\]/g, "-")}`),
    ).toBeVisible({ timeout: 20_000 });
    await changesPanel.getByRole("button", { name: "Review", exact: true }).tap();

    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();
    const header = dialog.locator(
      `[data-testid="review-file-header"][data-file-path="${MOBILE_FILE}"]`,
    );
    await expect(header).toBeVisible();
    const marker = header.getByRole("img", { name: "Added" });
    await expect(marker).toBeVisible();
    await expect(header.getByText("+1", { exact: true })).toBeVisible();

    const identityRow = header.getByTestId("review-file-identity");
    const fileName = identityRow.locator("[data-review-file-name]");
    await expect(fileName).toHaveText(path.basename(MOBILE_FILE));
    const directory = identityRow.locator("[data-review-file-directory]");
    await expect(directory).toHaveText(path.dirname(MOBILE_FILE));
    expect(await getSingleLineTextInVisualOrder(directory)).toBe(path.dirname(MOBILE_FILE));
    const directoryMetrics = await directory.evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
      direction: getComputedStyle(element).direction,
    }));
    expect(directoryMetrics.scrollWidth).toBeGreaterThan(directoryMetrics.clientWidth);
    expect(directoryMetrics.direction).toBe("rtl");
    await expect(header.getByTestId("review-file-actions")).toHaveCount(0);
    const moreActions = header.getByRole("button", {
      name: `More actions for ${MOBILE_FILE}`,
    });
    await expect(moreActions).toBeVisible();

    const movedHeader = dialog.locator(
      `[data-testid="review-file-header"][data-file-path="${MOBILE_MOVED_FILE}"]`,
    );
    await expect(movedHeader).toBeVisible();
    await expect(
      movedHeader.getByRole("img", { name: `Moved from ${MOBILE_MOVED_FROM_FILE}` }),
    ).toBeVisible();
    await expect(
      dialog.getByText(`Moved from ${MOBILE_MOVED_FROM_FILE}; no textual changes`),
    ).toBeVisible();

    const headerGeometry = await header.evaluate((element, filePath) => {
      const markerElement = element.querySelector<HTMLElement>('[data-file-status="added"]');
      const identityElement = element.querySelector<HTMLElement>(
        '[data-testid="review-file-identity"]',
      );
      const fileNameElement = element.querySelector<HTMLElement>("[data-review-file-name]");
      const moreActionsElement = element.querySelector<HTMLElement>(
        `[aria-label="More actions for ${filePath}"]`,
      );
      if (!markerElement || !identityElement || !fileNameElement || !moreActionsElement) {
        return null;
      }
      const headerBounds = element.getBoundingClientRect();
      const markerBounds = markerElement.getBoundingClientRect();
      const fileNameBounds = fileNameElement.getBoundingClientRect();
      const moreActionsBounds = moreActionsElement.getBoundingClientRect();
      return {
        scrollWidth: element.scrollWidth,
        clientWidth: element.clientWidth,
        height: headerBounds.height,
        headerLeft: headerBounds.left,
        headerRight: headerBounds.right,
        markerLeft: markerBounds.left,
        markerRight: markerBounds.right,
        fileNameLeft: fileNameBounds.left,
        fileNameRight: fileNameBounds.right,
        moreActionsLeft: moreActionsBounds.left,
        moreActionsRight: moreActionsBounds.right,
        moreActionsWidth: moreActionsBounds.width,
        moreActionsHeight: moreActionsBounds.height,
      };
    }, MOBILE_FILE);
    if (!headerGeometry) throw new Error("Mobile Review header geometry is unavailable");
    expect(headerGeometry.scrollWidth).toBeLessThanOrEqual(headerGeometry.clientWidth);
    expect(headerGeometry.height).toBeLessThanOrEqual(64);
    expect(headerGeometry.markerLeft).toBeGreaterThanOrEqual(headerGeometry.headerLeft);
    expect(headerGeometry.markerRight).toBeLessThanOrEqual(headerGeometry.headerRight);
    expect(headerGeometry.fileNameLeft).toBeGreaterThanOrEqual(headerGeometry.headerLeft);
    expect(headerGeometry.fileNameRight).toBeLessThanOrEqual(headerGeometry.headerRight);
    expect(headerGeometry.moreActionsLeft).toBeGreaterThanOrEqual(headerGeometry.headerLeft);
    expect(headerGeometry.moreActionsRight).toBeLessThanOrEqual(headerGeometry.headerRight);
    expect(headerGeometry.moreActionsWidth).toBeGreaterThanOrEqual(44);
    expect(headerGeometry.moreActionsHeight).toBeGreaterThanOrEqual(44);

    await moreActions.click();
    const actionsMenu = testPage.getByTestId("review-file-actions-menu");
    await expect(actionsMenu).toBeVisible();
    await actionsMenu.evaluate((element) =>
      Promise.all(
        element
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished.catch(() => undefined)),
      ),
    );
    const menuGeometry = await actionsMenu.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const items = Array.from(
        element.querySelectorAll<HTMLElement>('[role^="menuitem"]'),
        (item) => item.getBoundingClientRect(),
      );
      return {
        left: bounds.left,
        right: bounds.right,
        top: bounds.top,
        bottom: bounds.bottom,
        viewportWidth: window.innerWidth,
        viewportHeight: window.innerHeight,
        minItemHeight: Math.min(...items.map((item) => item.height)),
      };
    });
    expect(menuGeometry.left).toBeGreaterThanOrEqual(0);
    expect(menuGeometry.right).toBeLessThanOrEqual(menuGeometry.viewportWidth);
    expect(menuGeometry.top).toBeGreaterThanOrEqual(0);
    expect(menuGeometry.bottom).toBeLessThanOrEqual(menuGeometry.viewportHeight);
    expect(menuGeometry.minItemHeight).toBeGreaterThanOrEqual(44);

    await expect(actionsMenu.getByRole("menuitem", { name: "Copy diff" })).toBeVisible();
    await expect(actionsMenu.getByRole("menuitem", { name: "Edit file" })).toBeVisible();
    await expect(actionsMenu.getByRole("menuitem", { name: "Copy path" })).toBeVisible();
    await expect(actionsMenu.getByRole("menuitem", { name: "Open folder" })).toBeVisible();
    await expect(actionsMenu.getByRole("menuitem", { name: "Revert changes" })).toBeVisible();
    await expect(
      actionsMenu.getByRole("menuitemcheckbox", { name: "Expand unchanged lines" }),
    ).toBeVisible();
    const wrapLines = actionsMenu.getByRole("menuitemcheckbox", { name: "Wrap long lines" });
    const initialWrapState = await wrapLines.getAttribute("aria-checked");
    expect(["true", "false"]).toContain(initialWrapState);

    await wrapLines.click();
    await expect(actionsMenu).not.toBeVisible();
    await moreActions.click();
    await expect(
      testPage
        .getByTestId("review-file-actions-menu")
        .getByRole("menuitemcheckbox", { name: "Wrap long lines" }),
    ).toHaveAttribute("aria-checked", initialWrapState === "true" ? "false" : "true");
    await testPage.keyboard.press("Escape");

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
