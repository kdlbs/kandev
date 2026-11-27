import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone, waitForWorkspacePath } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Locator, Page } from "@playwright/test";
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
    const lockPath = path.join(this.repoDir, ".git", "index.lock");
    for (let attempt = 0; attempt < 3; attempt++) {
      if (fs.existsSync(lockPath)) fs.unlinkSync(lockPath);
      try {
        return execSync(cmd, { cwd: this.repoDir, env: this.env, encoding: "utf8" });
      } catch (err) {
        const msg = (err as Error).message ?? "";
        if (msg.includes("index.lock") && attempt < 2) {
          execSync("sleep 0.2");
          continue;
        }
        throw err;
      }
    }
    throw new Error(`git exec failed after 3 attempts: ${cmd}`);
  }

  createFile(name: string, content: string) {
    fs.writeFileSync(path.join(this.repoDir, name), content);
  }

  stageAll() {
    this.exec("git add -A");
  }

  getLastCommitMessage(): string {
    return this.exec("git log -1 --format=%B").trim();
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
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Git commit body", () => {
  test("commit dialog includes body textarea and commits with title + body", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Git Commit Body Profile");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Git Commit Body Test",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    expect(task.session_id).toBeTruthy();
    await waitForSessionDone(apiClient, task.id, task.session_id!, "Commit body fixture settled");
    const repoDir = await waitForWorkspacePath(apiClient, task.id, task.session_id!);

    const session = await openTaskSession(testPage, "Git Commit Body Test");

    // Wait for agent to finish initial turn
    await session.waitForChatIdle({ timeout: 30_000 });

    // Set up git helper
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const git = new GitHelper(repoDir, gitEnv);

    // Open Changes tab first so WS subscription is active
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Create a file - it should appear as unstaged via git status polling
    git.createFile("commit-body-test.txt", "test content for commit body");
    await expect(session.changes.getByText("commit-body-test.txt")).toBeVisible({
      timeout: 30_000,
    });

    // Stage the file via the UI "Stage all" button
    await session.changes.getByRole("button", { name: "Stage all", exact: true }).click();

    // Wait for the "Commit" button to appear (indicates staged section is ready)
    const commitButton = session.changes.getByRole("button", { name: "Commit", exact: true });
    await expect(commitButton).toBeVisible({ timeout: 15_000 });
    await commitButton.click();

    // The dialog should open
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 10_000 });

    // Verify both title input and body textarea are visible
    const titleInput = dialog.getByTestId("commit-title-input");
    const bodyInput = dialog.getByTestId("commit-body-input");
    await expect(titleInput).toBeVisible();
    await expect(bodyInput).toBeVisible();

    // Fill title and body
    await titleInput.fill("feat: add commit body support");
    await bodyInput.fill("Detailed description of the change");

    // Click Commit in dialog
    const dialogCommitBtn = dialog.getByRole("button", { name: "Commit", exact: true });
    await expect(dialogCommitBtn).toBeEnabled();
    await dialogCommitBtn.click();

    // Dialog should close
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });

    // Verify the commit message in the git repository includes both title and body
    await expect
      .poll(
        () => {
          try {
            return git.getLastCommitMessage();
          } catch {
            return "";
          }
        },
        { timeout: 15_000, message: "Expected commit with title and body" },
      )
      .toContain("feat: add commit body support");

    const fullMessage = git.getLastCommitMessage();
    expect(fullMessage).toContain("feat: add commit body support");
    expect(fullMessage).toContain("Detailed description of the change");

    // Verify title and body are separated by a blank line (git convention)
    const lines = fullMessage.split("\n");
    expect(lines[0]).toBe("feat: add commit body support");
    expect(lines[1]).toBe("");
    expect(lines[2]).toBe("Detailed description of the change");
  });
});

async function expectFailedDraftRetry(
  page: Page,
  commitButton: Locator,
  title: string,
  body: string,
) {
  const dialog = page.getByRole("dialog");
  const cancel = dialog.getByRole("button", { name: "Cancel", exact: true });
  await expect(page.getByTestId("toast-message").filter({ hasText: /Commit.*failed/ })).toBeVisible(
    {
      timeout: 30_000,
    },
  );
  await expect(dialog).toBeVisible();
  await expect(dialog.getByTestId("commit-title-input")).toHaveValue(title);
  await expect(dialog.getByTestId("commit-body-input")).toHaveValue(body);
  await expect(cancel).toBeEnabled();
  await cancel.click();
  await expect(dialog).toBeHidden();
  await commitButton.click();
  await expect(dialog).toBeVisible();
  await expect(dialog.getByTestId("commit-title-input")).toHaveValue(title);
  await expect(dialog.getByTestId("commit-body-input")).toHaveValue(body);
  await expect(cancel).toBeEnabled();
  await cancel.click();
  await expect(dialog).toBeHidden();
}

for (const width of [1280, 393, 767]) {
  test.describe(`Git commit pre-hooks (${width}px)`, () => {
    test("failed commit shows error in chat with Fix button", async ({
      testPage,
      apiClient,
      seedData,
      backend,
    }) => {
      await testPage.setViewportSize({ width, height: 851 });
      const profile = await createStandardProfile(apiClient, "Git Hook Test Profile");

      await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Hook Test", profile.id, {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });

      const session = await openTaskSession(testPage, "Git Hook Test");

      // Wait for agent to finish initial turn
      await session.waitForChatIdle({ timeout: 30_000 });

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
        const isPhone = width < 768;
        if (isPhone) {
          await testPage
            .getByRole("navigation")
            .getByRole("button", { name: /Changes$/ })
            .click();
        } else {
          await session.clickTab("Changes");
        }
        const changes = isPhone ? testPage.getByTestId("mobile-changes-panel") : session.changes;
        await expect(changes).toBeVisible({ timeout: 10_000 });

        // Create a file - it should appear as unstaged via git status polling
        const hookFile = `hook-test-${width}.txt`;
        git.createFile(hookFile, "test content");
        await expect(changes.getByText(hookFile)).toBeVisible({ timeout: 30_000 });

        // Stage the file via the UI "Stage all" button
        await changes.getByRole("button", { name: "Stage all", exact: true }).click();

        // Wait for the "Commit" button to appear (indicates staged section is ready)
        // Use exact match to avoid matching the "Commits" section toggle
        const commitButton = changes.getByRole("button", { name: "Commit", exact: true });
        await expect(commitButton).toBeVisible({ timeout: 15_000 });
        await commitButton.click();

        // Fill commit message and submit
        const dialog = testPage.getByRole("dialog");
        await expect(dialog).toBeVisible({ timeout: 10_000 });
        const rawTitle = "  test commit message  ";
        const rawBody = "  Hook rejection details\n\nKeep this draft.\n  ";
        await dialog.getByTestId("commit-title-input").fill(rawTitle);
        await dialog.getByTestId("commit-body-input").fill(rawBody);
        const dialogCommitBtn = dialog.getByRole("button", { name: "Commit", exact: true });
        await expect(dialogCommitBtn).toBeEnabled();
        await dialogCommitBtn.click();

        await expectFailedDraftRetry(testPage, commitButton, rawTitle, rawBody);
        if (isPhone) {
          await testPage
            .getByRole("navigation")
            .getByRole("button", { name: "Chat", exact: true })
            .click();
        }

        // The error message should appear in the chat
        const errorMessage = session.gitOperationErrorMessage();
        await expect(errorMessage).toBeVisible({ timeout: 30_000 });
        await expect(errorMessage.getByText("Git commit failed", { exact: true })).toBeVisible();
        await expect(session.gitFixButton()).toBeVisible();

        // The concise summary and Fix action remain available while command
        // output stays collapsed until the user asks to inspect it.
        const hookOutput = errorMessage.getByText("lint check failed: missing semicolons");
        await expect(hookOutput).toBeHidden();
        await errorMessage.locator("summary", { hasText: "Technical details" }).click();
        await expect(hookOutput).toBeVisible({ timeout: 15_000 });

        // Click the Fix button
        await session.gitFixButton().click();

        // The fix prompt should appear in the chat as a user message
        await expect(
          session.chat.getByText("git commit command failed", { exact: false }).first(),
        ).toBeVisible({ timeout: 10_000 });

        // The agent should process the fix prompt and respond
        await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

        // Verify the agent responded (mock agent echoes back part of the prompt)
        await expect(
          session.chat.getByText("completed the analysis", { exact: false }),
        ).toBeVisible({
          timeout: 5_000,
        });
      } finally {
        git.removePreCommitHook();
      }
    });
  });
}
