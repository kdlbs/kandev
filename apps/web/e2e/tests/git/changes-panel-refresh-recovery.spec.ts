import { test, expect } from "../../fixtures/test-base";
import {
  createStandardProfile,
  GitHelper,
  makeGitEnv,
  openTaskSession,
} from "../../helpers/git-helper";
import { createGitEnrichmentGate, routeGitStatusRefresh } from "./git-status-refresh-helpers";
import path from "node:path";

test.describe("Changes panel Git refresh recovery", () => {
  test.describe.configure({ timeout: 120_000 });

  test("keeps initial pending feedback in a narrow toolbar until refresh completes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 900, height: 800 });
    const repositoryPath = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repositoryPath, makeGitEnv(backend.tmpDir));
    git.exec("git reset --hard HEAD");
    git.exec("git clean -fd");

    const profile = await createStandardProfile(apiClient, "Initial Git Loading Profile");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Initial Git Loading",
      profile.id,
      {
        description: "e2e:delay(120000)",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("created task has no session ID");
    const bridge = await routeGitStatusRefresh(testPage);
    try {
      const session = await openTaskSession(testPage, "Initial Git Loading");
      bridge.holdFreshGitRefreshRequests(task.session_id);
      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible();
      await bridge.waitForHeldFreshGitRefreshRequests(1, task.session_id);

      const status = session.changes.getByTestId("changes-refresh-status");
      await expect(status).toContainText("Loading changes...");
      await expect(session.changes.getByText("Your changed files will appear here")).toHaveCount(0);
      const toolbar = session.changes.locator(":scope > div").first();
      const pendingToolbarBox = await toolbar.boundingBox();
      const panelBox = await session.changes.boundingBox();
      const statusBox = await status.boundingBox();
      expect(pendingToolbarBox).not.toBeNull();
      expect(panelBox).not.toBeNull();
      expect(statusBox).not.toBeNull();
      expect(statusBox!.x).toBeGreaterThanOrEqual(panelBox!.x);
      expect(statusBox!.x + statusBox!.width).toBeLessThanOrEqual(panelBox!.x + panelBox!.width);
      await prCapture.screenshot("git-refresh-recovery-desktop-initial-pending", {
        caption:
          "Initial Git loading stays in the toolbar while the Changes body has no empty state",
      });

      const priorFreshResponses = bridge.responseCount("fresh");
      bridge.releaseFreshGitRefreshRequests();
      const response = await bridge.waitForResponse("fresh", priorFreshResponses);
      expect(response.success).toBe(true);
      await expect(status).toHaveCount(0);
      const settledToolbarBox = await toolbar.boundingBox();
      expect(settledToolbarBox?.height).toBe(pendingToolbarBox?.height);
      await prCapture.screenshot("git-refresh-recovery-desktop-completed", {
        caption: "The loading feedback clears without changing the narrow toolbar height",
      });
    } finally {
      bridge.releaseFreshGitRefreshRequests();
    }
  });

  test("shows complete membership from the response while real enrichment is held", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const repositoryPath = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repositoryPath, makeGitEnv(backend.tmpDir));
    git.exec("git reset --hard HEAD");
    git.exec("git clean -fd");
    const filePath = "refresh-recovery.txt";
    git.createFile(filePath, "refresh recovery content\n");

    const bridge = await routeGitStatusRefresh(testPage);
    const gate = createGitEnrichmentGate(backend.tmpDir);
    gate.arm();

    try {
      const profile = await createStandardProfile(apiClient, "Git Refresh Recovery Profile");
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Git Refresh Recovery",
        profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("created task has no session ID");
      const priorFreshResponses = bridge.responseCount("fresh", task.session_id);
      const session = await openTaskSession(testPage, "Git Refresh Recovery");
      await gate.waitUntilStarted();

      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible();
      const response = await bridge.waitForResponse("fresh", priorFreshResponses, task.session_id);
      expect(response.success).toBe(true);
      expect(bridge.responseIncludesFile("fresh", filePath, task.session_id)).toBe(true);
      expect(bridge.responseHasPendingDetails("fresh", filePath, task.session_id)).toBe(true);
      await bridge.waitForDroppedPendingStatus();

      const fileRow = session.changes.getByTestId(`file-row-${filePath}`);
      await expect(fileRow).toBeVisible();
      await expect(session.changes.getByText("Diff is loading")).toHaveCount(0);
      const refreshStatus = session.changes.getByTestId("changes-refresh-status");
      await expect(refreshStatus).toContainText("Loading changes...");
      await refreshStatus.hover();
      await expect(testPage.getByRole("tooltip", { name: "Loading changes..." })).toBeVisible();

      const reviewButton = session.changes.getByRole("button", { name: "Review", exact: true });
      const walkthroughButton = session.changes.getByTestId("changes-request-walkthrough");
      const statusBox = await refreshStatus.boundingBox();
      const lastActionBox = await (
        (await walkthroughButton.isVisible()) ? walkthroughButton : reviewButton
      ).boundingBox();
      expect(statusBox).not.toBeNull();
      expect(lastActionBox).not.toBeNull();
      expect(statusBox!.x).toBeGreaterThanOrEqual(lastActionBox!.x + lastActionBox!.width - 1);
      expect(statusBox!.height).toBe(24);

      await reviewButton.hover();
      await expect(testPage.getByRole("tooltip", { name: "Review" })).toBeVisible();
      await testPage.mouse.move(0, 0);
      await reviewButton.focus();
      await testPage.keyboard.press("Tab");
      for (let step = 0; step < 3; step += 1) {
        if (await refreshStatus.evaluate((element) => element === document.activeElement)) break;
        await testPage.keyboard.press("Tab");
      }
      await expect(refreshStatus).toBeFocused();
      const loadingTooltip = testPage.getByRole("tooltip", { name: "Loading changes..." });
      await expect(loadingTooltip).toBeVisible();
      await loadingTooltip.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      await prCapture.screenshot("git-refresh-recovery-desktop-pending", {
        caption: "Complete changed-file membership is visible while Git diff enrichment is held",
      });

      await reviewButton.click();
      const reviewDialog = testPage.getByRole("dialog", { name: "Review Changes" });
      await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
      await testPage.keyboard.press("Escape");
      await expect(reviewDialog).toBeHidden();

      gate.release();
      await bridge.waitForReadyNotification();
      expect(bridge.notificationHasReadyFile(filePath)).toBe(true);
      await expect(fileRow).toBeVisible();
      await expect(refreshStatus).toHaveCount(0);
    } finally {
      gate.dispose();
    }
  });

  test("shares one toolbar spinner across concurrent held inline commit reads", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const repositoryPath = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repositoryPath, makeGitEnv(backend.tmpDir));
    git.exec("git reset --hard HEAD");
    git.exec("git clean -fd");

    const profile = await createStandardProfile(apiClient, "Inline Commit Loading Profile");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Inline Commit Loading", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    git.createFile("commit-first.txt", "first commit detail\n");
    git.stageAll();
    const firstSha = git.commit("Add first inline detail");
    git.createFile("commit-second.txt", "second commit detail\n");
    git.stageAll();
    const secondSha = git.commit("Add second inline detail");

    const bridge = await routeGitStatusRefresh(testPage);
    bridge.holdCommitDiffRequests();
    try {
      const session = await openTaskSession(testPage, "Inline Commit Loading");
      await session.waitForChatIdle({ timeout: 30_000 });
      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible();
      await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });
      await session.expandCommitsSection();
      await expect(session.changes.getByTestId("changes-refresh-status")).toHaveCount(0);

      const firstRow = testPage.getByTestId(`commit-row-${firstSha.slice(0, 7)}`);
      const secondRow = testPage.getByTestId(`commit-row-${secondSha.slice(0, 7)}`);
      await expect(firstRow).toBeVisible();
      await expect(secondRow).toBeVisible();
      await firstRow.getByTestId("commit-toggle").click();
      await secondRow.getByTestId("commit-toggle").click();
      await bridge.waitForCommitDiffRequests(2);

      const status = session.changes.getByTestId("changes-refresh-status");
      await expect(status).toContainText("Loading changes...");
      await expect(status).toHaveCount(1);
      await expect(session.changes.getByText("Diff is loading")).toHaveCount(0);
      await prCapture.screenshot("git-refresh-recovery-desktop-commit-details-pending", {
        caption: "One toolbar spinner covers two pending inline commit detail requests",
      });

      bridge.releaseNextCommitDiffRequest();
      await bridge.waitForCommitDiffResponses(1);
      await expect(session.changes.getByTestId("commit-file-commit-first.txt")).toBeVisible();
      await expect(status).toBeVisible();

      bridge.releaseCommitDiffRequests();
      await bridge.waitForCommitDiffResponses(2);
      await expect(session.changes.getByTestId("commit-file-commit-second.txt")).toBeVisible();
      await expect(status).toHaveCount(0);
      await prCapture.screenshot("git-refresh-recovery-desktop-commit-details-ready", {
        caption: "Inline commit files appear without a padded loading row",
      });
    } finally {
      bridge.releaseCommitDiffRequests();
    }
  });
});
