import { test, expect } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";

const MOD = process.platform === "darwin" ? ("Meta" as const) : ("Control" as const);

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

  stageAll() {
    this.exec("git add -A");
  }

  commit(message: string): string {
    this.exec(`git commit -m "${message}"`);
    return this.exec("git rev-parse HEAD").trim();
  }
}

function makeGitEnv(tmpDir: string): NodeJS.ProcessEnv {
  return {
    ...process.env,
    HOME: tmpDir,
    GIT_AUTHOR_NAME: "E2E Test",
    GIT_AUTHOR_EMAIL: "e2e@test.local",
    GIT_COMMITTER_NAME: "E2E Test",
    GIT_COMMITTER_EMAIL: "e2e@test.local",
  };
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

test.describe("Git Panel Multi-Select", () => {
  test("ctrl-click selects multiple unstaged files and shows bulk actions", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    const profile = await createStandardProfile(apiClient, "git-multi-select");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Multi-Select Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Multi-Select Test");
    await session.clickTab("Changes");

    // Create files AFTER session is open so WS detects the changes
    git.createFile("file-a.ts", "a");
    git.createFile("file-b.ts", "b");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    const fileA = session.changesFileRow("file-a.ts");
    const fileB = session.changesFileRow("file-b.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });
    await expect(fileB).toBeVisible({ timeout: 15_000 });

    await fileA.click({ modifiers: [MOD] });
    await expect(fileA).toHaveAttribute("data-selected", "true");

    await fileB.click({ modifiers: [MOD] });
    await expect(fileA).toHaveAttribute("data-selected", "true");
    await expect(fileB).toHaveAttribute("data-selected", "true");

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
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    const profile = await createStandardProfile(apiClient, "git-bulk-stage");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Bulk Stage Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Bulk Stage Test");
    await session.clickTab("Changes");

    git.createFile("stage-a.ts", "a");
    git.createFile("stage-b.ts", "b");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    const fileA = session.changesFileRow("stage-a.ts");
    const fileB = session.changesFileRow("stage-b.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });
    await expect(fileB).toBeVisible({ timeout: 15_000 });

    await fileA.click({ modifiers: [MOD] });
    await fileB.click({ modifiers: [MOD] });

    await session.changesBulkStageButton().click();

    const stagedList = session.changes.getByTestId("staged-file-list");
    await expect(stagedList.getByText("stage-a.ts")).toBeVisible({ timeout: 15_000 });
    await expect(stagedList.getByText("stage-b.ts")).toBeVisible({ timeout: 15_000 });
  });

  test("escape clears selection in git panel", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    const profile = await createStandardProfile(apiClient, "git-escape");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Escape Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Escape Test");
    await session.clickTab("Changes");

    git.createFile("esc-a.ts", "a");
    git.createFile("esc-b.ts", "b");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    const fileA = session.changesFileRow("esc-a.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });

    await fileA.click({ modifiers: [MOD] });
    await expect(fileA).toHaveAttribute("data-selected", "true");

    const bulkBar = session.changesBulkActionBar("unstaged");
    await expect(bulkBar).toBeVisible({ timeout: 5_000 });

    const fileList = testPage.getByTestId("unstaged-file-list");
    await fileList.focus();
    await testPage.keyboard.press("Escape");

    await expect(session.changesSelectedRows()).toHaveCount(0, { timeout: 5_000 });
    await expect(bulkBar).not.toBeVisible({ timeout: 5_000 });
  });
});
