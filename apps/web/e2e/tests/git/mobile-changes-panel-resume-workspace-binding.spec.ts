import { test, expect } from "../../fixtures/test-base";
import fs from "node:fs";
import { expectTouchControl } from "../../helpers/control-sizing";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { routeGitStatusRefresh } from "./git-status-refresh-helpers";
import {
  cleanupResumeWorkspaceBindingFixture,
  readRawWorkspaceBinding,
  resumeFailedSessionAndReload,
  seedResumeWorkspaceBindingFixture,
} from "./resume-workspace-binding-helpers";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile Changes after resume workspace binding recovery", () => {
  test.describe.configure({ timeout: 180_000 });

  test("opens the recovered dirty file from a real fresh response after reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fixture = await seedResumeWorkspaceBindingFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      `Resume workspace binding phone ${Date.now()}`,
    );
    const session = new SessionPage(testPage);
    const bridge = await routeGitStatusRefresh(testPage);

    try {
      const freshResponsesBeforeReload = await resumeFailedSessionAndReload(
        testPage,
        session,
        apiClient,
        fixture,
        { bridge, touch: true },
      );

      await test.step("open the phone Changes panel", async () => {
        const changesButton = testPage.getByTestId("mobile-session-nav-changes");
        await expect(changesButton).toHaveCount(1);
        await expectTouchControl(changesButton);
        await changesButton.tap();
      });
      const changes = testPage.getByTestId("mobile-changes-panel");
      await expect(changes).toBeVisible();

      const response = await bridge.waitForResponse("fresh", freshResponsesBeforeReload);
      expect(response.forced).toBe(false);
      expect(response.success).toBe(true);
      expect(response.sessionId).toBe(fixture.sessionId);
      expect(response.environmentId).toBe(fixture.environmentId);
      expect(
        response.snapshots.some((snapshot) =>
          Object.prototype.hasOwnProperty.call(
            snapshot.payload?.status?.files ?? {},
            fixture.dirtyFileName,
          ),
        ),
      ).toBe(true);

      const fileRow = changes.getByTestId(`file-row-${fixture.dirtyFileName}`);
      await expect(fileRow).toBeVisible();
      await expect(changes.getByTestId("changes-git-status-retry")).toHaveCount(0);
      await fileRow.tap();

      const diffSheet = testPage.getByTestId("mobile-diff-sheet");
      await expect(diffSheet).toBeVisible();
      await expect
        .poll(async () => (await diffSheet.boundingBox())?.height ?? 0)
        .toBeGreaterThanOrEqual((testPage.viewportSize()?.height ?? 0) * 0.95);
      await testPage.waitForFunction(
        (content) =>
          Array.from(document.querySelectorAll("diffs-container")).some((container) =>
            container.shadowRoot?.textContent?.includes(content),
          ),
        fixture.dirtyFileContent,
        { timeout: 45_000 },
      );
      expect(fs.readFileSync(fixture.dirtyFilePath, "utf8")).toBe(`${fixture.dirtyFileContent}\n`);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile resume workspace binding Changes");

      const afterEnvironment = await apiClient.getTaskEnvironment(fixture.taskId);
      expect(afterEnvironment?.id).toBe(fixture.environmentId);
      expect(afterEnvironment?.workspace_path).toBe(fixture.workspacePath);
      const repository = afterEnvironment?.repos?.find(
        (candidate) => candidate.repository_id === fixture.repositoryId,
      );
      expect(repository?.worktree_id).toBe(fixture.worktreeId);
      expect(repository?.worktree_path).toBe(fixture.worktreePath);
      expect(repository?.worktree_branch).toBe(fixture.branch);
      expect(readRawWorkspaceBinding(fixture.databasePath, fixture.sessionId)).toEqual({
        task_environment_id: fixture.environmentId,
        workspace_path: fixture.workspacePath,
        state: "WAITING_FOR_INPUT",
      });
    } finally {
      await cleanupResumeWorkspaceBindingFixture(apiClient, seedData, fixture);
    }
  });
});
