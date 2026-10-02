import { test, expect } from "../../fixtures/test-base";
import fs from "node:fs";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import { routeGitStatusRefresh } from "./git-status-refresh-helpers";
import {
  cleanupResumeWorkspaceBindingFixture,
  readRawWorkspaceBinding,
  resumeFailedSessionAndReload,
  seedResumeWorkspaceBindingFixture,
} from "./resume-workspace-binding-helpers";

test.describe("Changes after resume workspace binding recovery", () => {
  test.describe.configure({ timeout: 180_000 });

  test("shows the dirty worktree file from a real fresh response after reload", async ({
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
      `Resume workspace binding desktop ${Date.now()}`,
    );
    const session = new SessionPage(testPage);
    const bridge = await routeGitStatusRefresh(testPage);

    try {
      const freshResponsesBeforeReload = await resumeFailedSessionAndReload(
        testPage,
        session,
        apiClient,
        fixture,
        { bridge, touch: false },
      );

      await test.step("open the desktop Changes panel", async () => {
        await session.clickTab("Changes");
        await expect(session.changes).toBeVisible();
      });
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

      const fileRow = session.changes.getByTestId(`file-row-${fixture.dirtyFileName}`);
      await expect(fileRow).toBeVisible();
      await expect(session.changes.getByTestId("changes-git-status-retry")).toHaveCount(0);
      expect(fs.readFileSync(fixture.dirtyFilePath, "utf8")).toBe(`${fixture.dirtyFileContent}\n`);

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
      const git = new GitHelper(fixture.worktreePath, makeGitEnv(backend.tmpDir));
      expect(git.getCurrentSha()).toBe(fixture.head);
      expect(git.exec("git branch --show-current").trim()).toBe(fixture.branch);
    } finally {
      await cleanupResumeWorkspaceBindingFixture(apiClient, seedData, fixture);
    }
  });
});
