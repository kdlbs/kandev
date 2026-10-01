import { test, expect } from "../../fixtures/test-base";
import {
  createStandardProfile,
  GitHelper,
  makeGitEnv,
  openTaskSession,
} from "../../helpers/git-helper";
import { expectTouchControl } from "../../helpers/control-sizing";
import { createGitEnrichmentGate, routeGitStatusRefresh } from "./git-status-refresh-helpers";
import path from "node:path";

test.describe("Mobile Changes panel Git refresh recovery", () => {
  test.describe.configure({ timeout: 120_000 });

  test("shows Retry on failure and keeps the selected pending diff open through recovery", async ({
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
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      const { sessions } = await apiClient.listTaskSessions(task.id);
      const environmentId = sessions[0]?.task_environment_id;
      if (!environmentId) throw new Error("The task session should have an environment identity");
      bridge.setFailureEnvironmentId(environmentId);
      await openTaskSession(testPage, "Mobile Git Refresh Recovery");
      await gate.waitUntilStarted();

      bridge.forceFailures(["fresh", "recover"]);
      await testPage.getByRole("button", { name: "Changes" }).tap();
      await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
      await bridge.waitForResponse("fresh");
      await bridge.waitForResponse("recover");

      const unavailable = testPage.getByText("Git status unavailable", { exact: true });
      await expect(unavailable).toBeVisible();
      const retry = testPage.getByRole("button", { name: "Retry", exact: true });
      await expect(retry).toBeVisible();
      await expectTouchControl(retry);
      await prCapture.screenshot("git-refresh-recovery-mobile-unavailable", {
        caption:
          "The phone Changes panel identifies unavailable Git status and offers a touch-sized Retry",
      });

      const priorFreshResponses = bridge.responseCount("fresh");
      bridge.allowResponses();
      await retry.tap();
      const response = await bridge.waitForResponse("fresh", priorFreshResponses);
      expect(response.success).toBe(true);
      expect(response.forced).toBe(false);
      expect(bridge.responseIncludesFile("fresh", filePath)).toBe(true);
      expect(bridge.responseHasPendingDetails("fresh", filePath)).toBe(true);
      await bridge.waitForDroppedPendingStatus();

      const fileRow = testPage.getByTestId(`file-row-${filePath}`);
      await expect(fileRow).toBeVisible();
      await fileRow.tap();
      const diffSheet = testPage.getByTestId("mobile-diff-sheet");
      await expect(diffSheet).toBeVisible();
      await expect(diffSheet.getByTestId("mobile-diff-sheet-close")).toBeVisible();
      const viewportHeight = testPage.viewportSize()?.height ?? 0;
      expect(viewportHeight).toBeGreaterThan(0);
      await expect
        .poll(async () => (await diffSheet.boundingBox())?.height ?? 0)
        .toBeGreaterThanOrEqual(viewportHeight * 0.95);
      await expect
        .poll(async () => (await diffSheet.boundingBox())?.y ?? Infinity)
        .toBeLessThanOrEqual(viewportHeight * 0.05);
      await expect(diffSheet.getByText("Diff is loading")).toBeVisible();
      const documentOverflow = await testPage.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      );
      expect(documentOverflow).toBe(false);
      await prCapture.screenshot("git-refresh-recovery-mobile-pending", {
        caption: "A selected pending file keeps its full-height phone diff surface open",
      });

      gate.release();
      await bridge.waitForReadyNotification();
      expect(bridge.notificationHasReadyFile(filePath)).toBe(true);
      await expect(testPage.getByTestId("mobile-diff-sheet-close")).toBeVisible();
    } finally {
      gate.dispose();
    }
  });
});
