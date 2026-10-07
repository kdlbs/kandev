import { test, expect } from "../../fixtures/test-base";
import {
  createStandardProfile,
  GitHelper,
  makeGitEnv,
  openTaskSession,
} from "../../helpers/git-helper";
import { expectTouchSquareControl } from "../../helpers/control-sizing";
import { createGitEnrichmentGate, routeGitStatusRefresh } from "./git-status-refresh-helpers";
import path from "node:path";

test.describe("Mobile Changes panel Git refresh recovery", () => {
  test.describe.configure({ timeout: 120_000 });

  test("retries automatically after failure and keeps the selected pending diff open", async ({
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
    const filePath = "mobile-refresh-recovery.txt";
    git.createFile(filePath, "mobile refresh recovery content\n");

    const bridge = await routeGitStatusRefresh(testPage);
    const gate = createGitEnrichmentGate(backend.tmpDir);
    gate.arm();

    try {
      const profile = await createStandardProfile(apiClient, "Mobile Git Refresh Recovery Profile");
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Mobile Git Refresh Recovery",
        profile.id,
        {
          description: "e2e:delay(120000)",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      const { sessions } = await apiClient.listTaskSessions(task.id);
      const environmentId = sessions[0]?.task_environment_id;
      if (!environmentId) throw new Error("The task session should have an environment identity");
      bridge.setFailureEnvironmentId(environmentId);
      const priorFreshResponses = bridge.responseCount("fresh");
      await openTaskSession(testPage, "Mobile Git Refresh Recovery");
      await gate.waitUntilStarted();

      await testPage.getByRole("button", { name: "Changes" }).tap();
      const panel = testPage.getByTestId("mobile-changes-panel");
      await expect(panel).toBeVisible();
      const initialResponse = await bridge.waitForResponse("fresh", priorFreshResponses);
      expect(initialResponse.success).toBe(true);
      expect(bridge.responseIncludesFile("fresh", filePath)).toBe(true);
      expect(bridge.responseHasPendingDetails("fresh", filePath)).toBe(true);
      await bridge.waitForDroppedPendingStatus();

      const refreshStatus = testPage.getByTestId("changes-refresh-status");
      const fileRow = testPage.getByTestId(`file-row-${filePath}`);
      await expect(fileRow).toBeVisible();
      await expect(refreshStatus).toContainText("Loading changes...");
      await expectTouchSquareControl(refreshStatus);

      const priorRefreshResponses = bridge.responseCount("fresh");
      const priorRecoveryResponses = bridge.responseCount("recover");
      await testPage.getByRole("button", { name: "Chat" }).tap();
      await expect(panel).toHaveCount(0);
      bridge.forceFailures(["fresh", "recover"]);
      await testPage.getByRole("button", { name: "Changes" }).tap();
      await expect(panel).toBeVisible();
      await bridge.waitForResponse("fresh", priorRefreshResponses);
      await bridge.waitForResponse("recover", priorRecoveryResponses);

      await expect(refreshStatus).toBeVisible();
      await expect(refreshStatus).toContainText(
        /Changes did not refresh\. Retrying automatically\./,
      );
      await expect(fileRow).toBeVisible();
      await expect(panel.getByText("Git status unavailable", { exact: true })).toHaveCount(0);
      await expect(panel.getByRole("button", { name: "Retry", exact: true })).toHaveCount(0);
      await prCapture.screenshot("git-refresh-recovery-mobile-unavailable", {
        caption: "The phone Changes toolbar shows automatic recovery while preserving prior files",
      });

      await fileRow.tap();
      const diffSheet = testPage.getByTestId("mobile-diff-sheet");
      await expect(diffSheet).toBeVisible();
      await expect(diffSheet.getByTestId("mobile-diff-sheet-close")).toBeVisible();
      await expect(diffSheet.getByText("Diff is unavailable", { exact: true })).toBeVisible();
      const viewportHeight = testPage.viewportSize()?.height ?? 0;
      expect(viewportHeight).toBeGreaterThan(0);
      await expect
        .poll(async () => (await diffSheet.boundingBox())?.height ?? 0)
        .toBeGreaterThanOrEqual(viewportHeight * 0.95);
      await expect
        .poll(async () => (await diffSheet.boundingBox())?.y ?? Infinity)
        .toBeLessThanOrEqual(viewportHeight * 0.05);
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
        ),
      ).toBe(false);

      const retryFreshResponses = bridge.responseCount("fresh");
      const priorPendingEvents = bridge.droppedPendingCount();
      bridge.allowResponses();
      bridge.holdFreshGitRefreshRequests();
      await bridge.waitForHeldFreshGitRefreshRequests(1);
      await expect(refreshStatus).toContainText("Loading changes...");
      bridge.releaseFreshGitRefreshRequests();
      const response = await bridge.waitForResponse("fresh", retryFreshResponses);
      expect(response.success).toBe(true);
      expect(response.forced).toBe(false);
      expect(bridge.responseIncludesFile("fresh", filePath)).toBe(true);
      expect(bridge.responseHasPendingDetails("fresh", filePath)).toBe(true);
      await expect.poll(() => bridge.droppedPendingCount()).toBeGreaterThan(priorPendingEvents);
      await expect(refreshStatus).toContainText("Loading changes...");

      await expect(fileRow).toBeVisible();
      await expect(diffSheet.getByText("Diff is loading", { exact: true })).toBeVisible();
      await prCapture.screenshot("git-refresh-recovery-mobile-pending", {
        caption: "The selected pending diff stays full-height while automatic Git recovery runs",
      });

      const priorReadyNotifications = bridge.readyNotificationCount();
      gate.release();
      await expect
        .poll(() => bridge.readyNotificationCount())
        .toBeGreaterThan(priorReadyNotifications);
      expect(bridge.notificationHasReadyFile(filePath)).toBe(true);
      await expect(testPage.getByTestId("mobile-diff-sheet-close")).toBeVisible();
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

    const profile = await createStandardProfile(apiClient, "Mobile Inline Commit Loading Profile");
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Inline Commit Loading",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    git.createFile("mobile-commit-first.txt", "first mobile commit detail\n");
    git.stageAll();
    const firstSha = git.commit("Add first mobile inline detail");
    git.createFile("mobile-commit-second.txt", "second mobile commit detail\n");
    git.stageAll();
    const secondSha = git.commit("Add second mobile inline detail");

    const bridge = await routeGitStatusRefresh(testPage);
    bridge.holdCommitDiffRequests();
    try {
      await openTaskSession(testPage, "Mobile Inline Commit Loading");
      await testPage.getByRole("button", { name: "Changes" }).tap();
      const panel = testPage.getByTestId("mobile-changes-panel");
      await expect(panel).toBeVisible();
      await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });
      const commitsToggle = testPage.getByTestId("commits-section-collapse-toggle");
      await expect
        .poll(async () => {
          if ((await commitsToggle.getAttribute("aria-expanded")) !== "true") {
            await commitsToggle.tap();
          }
          return commitsToggle.getAttribute("aria-expanded");
        })
        .toBe("true");
      await expect(panel.getByTestId("changes-refresh-status")).toHaveCount(0);

      const firstRow = testPage.getByTestId(`commit-row-${firstSha.slice(0, 7)}`);
      const secondRow = testPage.getByTestId(`commit-row-${secondSha.slice(0, 7)}`);
      await expect(firstRow).toBeVisible();
      await expect(secondRow).toBeVisible();
      await firstRow.getByTestId("commit-toggle").tap();
      await secondRow.getByTestId("commit-toggle").tap();
      await bridge.waitForCommitDiffRequests(2);

      const status = panel.getByTestId("changes-refresh-status");
      await expect(status).toContainText("Loading changes...");
      await expectTouchSquareControl(status);
      await expect(status).toHaveCount(1);
      await expect(panel.getByText("Diff is loading")).toHaveCount(0);
      await prCapture.screenshot("git-refresh-recovery-mobile-commit-details-pending", {
        caption: "The phone toolbar shows one spinner for two held commit detail reads",
      });

      bridge.releaseNextCommitDiffRequest();
      await bridge.waitForCommitDiffResponses(1);
      await expect(panel.getByTestId("commit-file-mobile-commit-first.txt")).toBeVisible();
      await expect(status).toBeVisible();

      bridge.releaseCommitDiffRequests();
      await bridge.waitForCommitDiffResponses(2);
      await expect(panel.getByTestId("commit-file-mobile-commit-second.txt")).toBeVisible();
      await expect(status).toHaveCount(0);
      const documentOverflow = await testPage.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      );
      expect(documentOverflow).toBe(false);
      await prCapture.screenshot("git-refresh-recovery-mobile-commit-details-ready", {
        caption: "The phone timeline keeps expanded commit details within the panel",
      });
    } finally {
      bridge.releaseCommitDiffRequests();
    }
  });
});
