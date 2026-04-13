import { expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import type { ApiClient } from "./api-client";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

export class GitHelper {
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

  modifyFile(name: string, content: string) {
    this.createFile(name, content);
  }

  deleteFile(name: string) {
    const filePath = path.join(this.repoDir, name);
    if (fs.existsSync(filePath)) fs.unlinkSync(filePath);
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

  getCurrentSha(): string {
    return this.exec("git rev-parse HEAD").trim();
  }
}

export function makeGitEnv(tmpDir: string): NodeJS.ProcessEnv {
  return {
    ...process.env,
    HOME: tmpDir,
    GIT_AUTHOR_NAME: "E2E Test",
    GIT_AUTHOR_EMAIL: "e2e@test.local",
    GIT_COMMITTER_NAME: "E2E Test",
    GIT_COMMITTER_EMAIL: "e2e@test.local",
  };
}

export async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
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

export async function createStandardProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const agentId = agents[0]?.id;
  if (!agentId) throw new Error("No agent available");
  return apiClient.createAgentProfile(agentId, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: false,
  });
}
