import { test, expect } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";

const MODIFIER = process.platform === "darwin" ? "Meta" : "Control";

class GitHelper {
  constructor(
    private repoDir: string,
    private env: NodeJS.ProcessEnv,
  ) {}

  exec(cmd: string): string {
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        return execSync(cmd, { cwd: this.repoDir, env: this.env, encoding: "utf8" });
      } catch (err) {
        const msg = (err as Error).message ?? "";
        if (msg.includes("index.lock") && attempt < 2) {
          execSync("sleep 0.3");
          continue;
        }
        throw err;
      }
    }
    throw new Error(`git exec failed after 3 attempts: ${cmd}`);
  }

  createFile(name: string, content: string) {
    const filePath = path.join(this.repoDir, name);
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
    fs.writeFileSync(filePath, content);
  }

  stageFile(name: string) {
    this.exec(`git add "${name}"`);
  }

  stageAll() {
    this.exec("git add -A");
  }

  commit(message: string): string {
    this.exec(`git commit -m "${message}"`);
    return this.exec("git rev-parse HEAD").trim();
  }
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

async function createStandardProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const agentId = agents[0]?.id;
  if (!agentId) throw new Error("No agent available");
  return apiClient.createAgentProfile(agentId, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: false,
  });
}

test.describe("Changes Panel Multi-Select", () => {
  test("ctrl-click selects multiple unstaged files and shows bulk actions", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = { ...process.env, HOME: backend.tmpDir };
    const git = new GitHelper(repoDir, gitEnv);

    // Create files that will show as untracked/unstaged
    git.createFile("file-a.ts", "a");
    git.createFile("file-b.ts", "b");

    const profile = await createStandardProfile(apiClient, "changes-select");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Changes Select Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Changes Select Test");
    await session.clickTab("Changes");

    // Wait for files to appear in unstaged section
    const fileA = session.changesFileRow("file-a.ts");
    const fileB = session.changesFileRow("file-b.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });
    await expect(fileB).toBeVisible({ timeout: 15_000 });

    // Click first file
    await fileA.click();
    await expect(fileA).toHaveAttribute("data-selected", "true");

    // Ctrl-click second file
    await fileB.click({ modifiers: [MODIFIER === "Meta" ? "Meta" : "Control"] });
    await expect(fileA).toHaveAttribute("data-selected", "true");
    await expect(fileB).toHaveAttribute("data-selected", "true");

    // Bulk action bar should appear
    const bulkBar = session.changesBulkActionBar("unstaged");
    await expect(bulkBar).toBeVisible({ timeout: 5_000 });
    await expect(session.changesBulkStageButton()).toBeVisible();
    await expect(session.changesBulkDiscardButton()).toBeVisible();
  });

  test("bulk stage moves selected files to staged section", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = { ...process.env, HOME: backend.tmpDir };
    const git = new GitHelper(repoDir, gitEnv);

    git.createFile("stage-a.ts", "a");
    git.createFile("stage-b.ts", "b");

    const profile = await createStandardProfile(apiClient, "changes-bulk-stage");
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Changes Bulk Stage Test",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "Changes Bulk Stage Test");
    await session.clickTab("Changes");

    // Wait for files
    const fileA = session.changesFileRow("stage-a.ts");
    const fileB = session.changesFileRow("stage-b.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });
    await expect(fileB).toBeVisible({ timeout: 15_000 });

    // Select both via ctrl-click
    await fileA.click();
    await fileB.click({ modifiers: [MODIFIER === "Meta" ? "Meta" : "Control"] });

    // Click bulk stage
    await session.changesBulkStageButton().click();

    // Files should move to staged section
    const stagedList = session.changes.getByTestId("staged-file-list");
    await expect(stagedList.getByText("stage-a.ts")).toBeVisible({ timeout: 15_000 });
    await expect(stagedList.getByText("stage-b.ts")).toBeVisible({ timeout: 15_000 });
  });

  test("escape clears selection in changes panel", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = { ...process.env, HOME: backend.tmpDir };
    const git = new GitHelper(repoDir, gitEnv);

    git.createFile("esc-a.ts", "a");
    git.createFile("esc-b.ts", "b");

    const profile = await createStandardProfile(apiClient, "changes-escape");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Changes Escape Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Changes Escape Test");
    await session.clickTab("Changes");

    const fileA = session.changesFileRow("esc-a.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });

    // Select file
    await fileA.click({ modifiers: [MODIFIER === "Meta" ? "Meta" : "Control"] });
    await expect(fileA).toHaveAttribute("data-selected", "true");

    // Bulk bar visible
    const bulkBar = session.changesBulkActionBar("unstaged");
    await expect(bulkBar).toBeVisible({ timeout: 5_000 });

    // Press Escape
    await testPage.keyboard.press("Escape");

    // Selection should be cleared
    await expect(session.changesSelectedRows()).toHaveCount(0, { timeout: 5_000 });
    await expect(bulkBar).not.toBeVisible({ timeout: 5_000 });
  });
});
