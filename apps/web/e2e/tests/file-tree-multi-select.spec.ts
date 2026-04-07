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
    const dir = path.dirname(filePath);
    fs.mkdirSync(dir, { recursive: true });
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

test.describe("File Tree Multi-Select", () => {
  test("ctrl-click toggles individual files in selection", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    // Commit files so they appear in the tree
    git.createFile("alpha.ts", "a");
    git.createFile("beta.ts", "b");
    git.createFile("gamma.ts", "c");
    git.stageAll();
    git.commit("add test files");

    const profile = await createStandardProfile(apiClient, "file-tree-select");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "File Tree Select Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "File Tree Select Test");
    await session.clickTab("Files");

    // Wait for file tree to load
    const alphaNode = session.fileTreeNode("alpha.ts");
    await expect(alphaNode).toBeVisible({ timeout: 15_000 });

    // Ctrl-click first file to select it
    const mod = MODIFIER === "Meta" ? "Meta" : ("Control" as const);
    await alphaNode.click({ modifiers: [mod] });
    await expect(alphaNode).toHaveAttribute("data-selected", "true");

    // Ctrl-click second file - both should be selected
    const betaNode = session.fileTreeNode("beta.ts");
    await betaNode.click({ modifiers: [mod] });
    await expect(alphaNode).toHaveAttribute("data-selected", "true");
    await expect(betaNode).toHaveAttribute("data-selected", "true");

    // Ctrl-click first file again to deselect
    await alphaNode.click({ modifiers: [mod] });
    await expect(alphaNode).toHaveAttribute("data-selected", "false");
    await expect(betaNode).toHaveAttribute("data-selected", "true");
  });

  test("escape clears selection", async ({ testPage, apiClient, seedData, backend }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    git.createFile("file1.ts", "1");
    git.createFile("file2.ts", "2");
    git.stageAll();
    git.commit("add files for escape test");

    const profile = await createStandardProfile(apiClient, "file-tree-escape");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "File Tree Escape Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "File Tree Escape Test");
    await session.clickTab("Files");

    const file1 = session.fileTreeNode("file1.ts");
    await expect(file1).toBeVisible({ timeout: 15_000 });

    // Select a file
    await file1.click({ modifiers: [MODIFIER === "Meta" ? "Meta" : "Control"] });
    await expect(file1).toHaveAttribute("data-selected", "true");

    // Press Escape to clear — use the file node to ensure panel has focus
    await file1.press("Escape");
    await expect(session.fileTreeSelectedNodes()).toHaveCount(0, { timeout: 5_000 });
  });
});
