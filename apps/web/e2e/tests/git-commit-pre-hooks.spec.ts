import { test, expect } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";

// ---------------------------------------------------------------------------
// Helpers (same patterns as git-changes-panel.spec.ts)
// ---------------------------------------------------------------------------

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

  installFailingPreCommitHook(message: string) {
    const hooksDir = path.join(this.repoDir, ".git", "hooks");
    fs.mkdirSync(hooksDir, { recursive: true });
    const hookPath = path.join(hooksDir, "pre-commit");
    fs.writeFileSync(hookPath, `#!/bin/sh\necho "${message}"\nexit 1\n`);
    fs.chmodSync(hookPath, "755");
  }

  removePreCommitHook() {
    const hookPath = path.join(this.repoDir, ".git", "hooks", "pre-commit");
    if (fs.existsSync(hookPath)) {
      fs.unlinkSync(hookPath);
    }
  }
}

async function createStandardProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const agentId = agents[0]?.id;
  if (!agentId) {
    throw new Error(`E2E setup failed: no agent available for profile "${name}"`);
  }
  return apiClient.createAgentProfile(agentId, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: false,
  });
}

async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/s\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Git commit pre-hooks", () => {
  test("failed commit shows error in chat with Fix button", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Hook Test Profile");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Hook Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Hook Test");

    // Wait for agent to finish initial turn
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Set up git helper
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

    try {
      // Install a pre-commit hook that always fails
      git.installFailingPreCommitHook("lint check failed: missing semicolons");

      // Open Changes tab first so WS subscription is active
      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible({ timeout: 10_000 });

      // Create a file - it should appear as unstaged via git status polling
      git.createFile("hook-test.txt", "test content");
      await expect(session.changes.getByText("hook-test.txt")).toBeVisible({ timeout: 30_000 });

      // Stage the file via the UI "Stage all" button
      await session.changes.getByRole("button", { name: "Stage all" }).click();

      // Wait for the "Commit" button to appear (indicates staged section is ready)
      const commitButton = session.changes.getByRole("button", { name: "Commit" });
      await expect(commitButton).toBeVisible({ timeout: 15_000 });
      await commitButton.click();

      // Fill commit message and submit
      const dialog = testPage.getByRole("dialog");
      await expect(dialog).toBeVisible({ timeout: 5_000 });
      await dialog.getByRole("textbox").fill("test commit message");
      await dialog.getByRole("button", { name: "Commit" }).click();

      // The error message should appear in the chat
      await expect(session.gitOperationErrorMessage()).toBeVisible({ timeout: 30_000 });

      // Verify the error output contains the pre-commit hook message
      await expect(
        session.gitOperationErrorMessage().getByText("lint check failed: missing semicolons"),
      ).toBeVisible({ timeout: 5_000 });

      // Click the Fix button
      await session.gitFixButton().click();

      // The fix prompt should appear in the chat as a user message
      await expect(
        session.chat.getByText("git commit command failed", { exact: false }),
      ).toBeVisible({ timeout: 10_000 });

      // The agent should process the fix prompt and respond
      await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

      // Verify the agent responded (mock agent echoes back part of the prompt)
      await expect(session.chat.getByText("completed the analysis", { exact: false })).toBeVisible({
        timeout: 5_000,
      });
    } finally {
      git.removePreCommitHook();
    }
  });
});
