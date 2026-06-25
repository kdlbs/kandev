import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";
import { makeGitEnv } from "../../helpers/git-helper";
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

function changesTab(testPage: Page) {
  return testPage.locator(".dv-tab", {
    has: testPage.locator(".dv-default-tab").filter({ hasText: /^Changes/ }),
  });
}

async function moveChangesToTerminalGroupAndFocusTerminal(testPage: Page) {
  await testPage.evaluate(() => {
    type Group = { id: string };
    type PanelApi = {
      moveTo: (opts: { group: Group }) => void;
      setActive: () => void;
    };
    type Panel = { id: string; api: PanelApi; group?: Group };
    type Api = { getPanel: (id: string) => Panel | undefined };
    const dockview = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    const changes = dockview?.getPanel("changes");
    const terminal = dockview?.getPanel("terminal-default");
    if (!changes || !terminal?.group) {
      throw new Error("Dockview changes or terminal panel was not available");
    }
    changes.api.moveTo({ group: terminal.group });
    terminal.api.setActive();
  });
}

async function moveChangesToChatGroupAndFocusChat(testPage: Page) {
  await testPage.evaluate(() => {
    type Group = { id: string };
    type PanelApi = {
      moveTo: (opts: { group: Group }) => void;
      setActive: () => void;
    };
    type Panel = { id: string; api: PanelApi; group?: Group };
    type Api = { panels: Panel[]; getPanel: (id: string) => Panel | undefined };
    const dockview = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    const changes = dockview?.getPanel("changes");
    const chat =
      dockview?.getPanel("chat") ?? dockview?.panels.find((p) => p.id.startsWith("session:"));
    if (!changes || !chat?.group) {
      throw new Error("Dockview changes or chat panel was not available");
    }
    changes.api.moveTo({ group: chat.group });
    chat.api.setActive();
  });
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
    const gitEnv = makeGitEnv(backend.tmpDir);
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
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    await testPage.goto(`/t/${task.id}?sessionId=${task.session_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    // Create a file and commit so there are existing changes
    git.createFile("test-file.txt", "hello world");
    git.stageAll();
    git.commit("test commit");

    // Wait for the changes panel to show the commit
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await session.expandCommitsSection();
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

  test("changes panel does not auto-focus when grouped with agent session panels", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = makeGitEnv(backend.tmpDir);
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
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    await testPage.goto(`/t/${task.id}?sessionId=${task.session_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.waitForDockviewReady();

    await moveChangesToChatGroupAndFocusChat(testPage);
    await expect(session.chat).toBeVisible();

    await expect(changesTab(testPage)).not.toHaveClass(/dv-active-tab/, { timeout: 5_000 });

    git.createFile("new-file.txt", "new content");
    git.stageAll();
    git.commit("new commit");

    // Wait for the changes badge to update
    await expect(testPage.locator(".dv-default-tab:has-text('Changes')")).toBeVisible({
      timeout: 15_000,
    });

    // Wait a bit for any async auto-activate to fire
    await testPage.waitForTimeout(2_000);

    await expect(changesTab(testPage)).not.toHaveClass(/dv-active-tab/, { timeout: 5_000 });

    await expect(session.chat).toBeVisible();
  });

  test("new git updates focus the changes tab in its current non-agent group", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = makeGitEnv(backend.tmpDir);
    const git = new GitHelper(repoDir, gitEnv);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Moved changes panel focus test",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    await testPage.goto(`/t/${task.id}?sessionId=${task.session_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.waitForDockviewReady();

    git.createFile("first-change.txt", "one");
    await expect(
      testPage.locator(".dv-default-tab").filter({ hasText: /^Changes \(1\)$/ }),
    ).toBeVisible({ timeout: 15_000 });

    await moveChangesToTerminalGroupAndFocusTerminal(testPage);
    await expect(changesTab(testPage)).not.toHaveClass(/dv-active-tab/, { timeout: 5_000 });

    git.createFile("second-change.txt", "two");
    await expect(
      testPage.locator(".dv-default-tab").filter({ hasText: /^Changes \(2\)$/ }),
    ).toBeVisible({ timeout: 15_000 });

    await expect(changesTab(testPage)).toHaveClass(/dv-active-tab/, { timeout: 5_000 });
    await expect(session.changesFileRow("second-change.txt")).toBeVisible({ timeout: 10_000 });
  });
});
