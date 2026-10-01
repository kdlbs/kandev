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
      await apiClient.createTaskWithAgent(
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
      const priorFreshResponses = bridge.responseCount("fresh");
      const session = await openTaskSession(testPage, "Git Refresh Recovery");
      await gate.waitUntilStarted();

      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible();
      const response = await bridge.waitForResponse("fresh", priorFreshResponses);
      expect(response.success).toBe(true);
      expect(bridge.responseIncludesFile("fresh", filePath)).toBe(true);
      expect(bridge.responseHasPendingDetails("fresh", filePath)).toBe(true);
      await bridge.waitForDroppedPendingStatus();

      const fileRow = session.changes.getByTestId(`file-row-${filePath}`);
      await expect(fileRow).toBeVisible();
      await expect(session.changes.getByText("Diff is loading")).toBeVisible();
      await prCapture.screenshot("git-refresh-recovery-desktop-pending", {
        caption: "Complete changed-file membership is visible while Git diff enrichment is held",
      });

      gate.release();
      await bridge.waitForReadyNotification();
      expect(bridge.notificationHasReadyFile(filePath)).toBe(true);
      await expect(fileRow).toBeVisible();
    } finally {
      gate.dispose();
    }
  });
});
