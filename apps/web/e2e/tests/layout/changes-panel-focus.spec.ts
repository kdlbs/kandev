import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";

/** Minimal git helper for E2E tests - runs git commands in the test repository. */
class GitHelper {
  constructor(
    private repoDir: string,
    private env: NodeJS.ProcessEnv,
  ) {}

  exec(cmd: string): string {
    return execSync(cmd, { cwd: this.repoDir, env: this.env, encoding: "utf8" });
  }

  createFile(name: string, content: string) {
    fs.writeFileSync(path.join(this.repoDir, name), content);
  }

  stageAll() {
    this.exec("git add -A");
  }

  commit(message: string) {
    this.exec(`git commit -m "${message}"`);
  }
}

test.describe("Changes panel focus behavior", () => {
  /**
   * Verifies the changes panel does NOT steal focus from the chat tab
   * on page refresh when the task has existing git changes/commits.
   *
   * Root cause of the bug: the changes-tab component auto-activated the
   * changes panel whenever totalCount went from 0 → N.  On page refresh,
   * hooks start with totalCount=0 (no data loaded), then async git data
   * arrives making totalCount > 0 — triggering the auto-activate.
   */
  test("changes panel does not auto-focus on page refresh", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Create a task and wait for the agent to be ready
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Focus test task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Create a file and commit so there are existing changes
    git.createFile("test-file.txt", "hello world");
    git.stageAll();
    git.commit("test commit");

    // Wait for the changes panel to show the commit
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await expect(session.changes.getByText("test commit")).toBeVisible({ timeout: 10_000 });

    // Switch back to chat tab — this is the tab that should be active after refresh
    await session.clickSessionChatTab();
    await expect(session.chat).toBeVisible({ timeout: 5_000 });

    // Refresh the page
    await testPage.reload();
    await session.waitForLoad();

    // Wait for the git data to load (changes tab should show count)
    await expect(testPage.locator(".dv-default-tab:has-text('Changes')")).toBeVisible({
      timeout: 15_000,
    });

    // The chat/session panel should be the active tab, NOT changes
    const changesTab = testPage.locator(".dv-default-tab:has-text('Changes')");
    await expect(changesTab).not.toHaveClass(/dv-active-tab/, { timeout: 5_000 });

    // Chat should be visible (active in center group)
    await expect(session.chat).toBeVisible({ timeout: 5_000 });
  });

  /**
   * Verifies the changes panel does NOT auto-focus when it is in the center
   * group (e.g. plan mode layout). Even when new changes appear, the chat
   * panel should stay focused.
   */
  test("changes panel does not auto-focus when in center group", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Center group focus test",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Toggle plan mode — this moves changes into the center group
    await session.togglePlanMode();
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
    await expect(session.chat).toBeVisible();

    // Verify the chat/session tab is active, not changes
    const changesTab = testPage.locator(".dv-default-tab:has-text('Changes')");
    await expect(changesTab).not.toHaveClass(/dv-active-tab/, { timeout: 5_000 });

    // Create new changes — this would previously trigger auto-activate
    git.createFile("new-file.txt", "new content");
    git.stageAll();
    git.commit("new commit");

    // Wait for the changes badge to update
    await expect(testPage.locator(".dv-default-tab:has-text('Changes')")).toBeVisible({
      timeout: 15_000,
    });

    // Wait a bit for any async auto-activate to fire
    await testPage.waitForTimeout(2_000);

    // Changes tab should NOT have stolen focus
    await expect(changesTab).not.toHaveClass(/dv-active-tab/, { timeout: 5_000 });

    // Chat should still be visible (active)
    await expect(session.chat).toBeVisible();
  });
});
