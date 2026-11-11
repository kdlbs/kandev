import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { ApiClient } from "./api-client";
import { makeGitEnv } from "./git-helper";

export type LiveAttachmentRepository = {
  repositoryPath: string;
  repositoryName: string;
  readmeContent: string;
  workOrderPath: string;
};

type LiveAttachmentSession = {
  id: string;
  workspace_path?: string | null;
  worktree_path?: string | null;
  agent_workspace_path?: string | null;
  worktrees?: Array<{ worktree_path?: string | null }>;
};

export const LIVE_ATTACHMENT_README = "secondary checkout readme\n";
export const LIVE_ATTACHMENT_WORK_ORDER = "primary checkout work order\n";

/** Creates an offline local repository the legacy add-branch tool can attach. */
export function createLiveAttachmentRepository(root: string): LiveAttachmentRepository {
  const gitEnv = makeGitEnv(root);
  const repositoryName = "secondary-repository";
  const repositoryPath = path.join(root, "sources", repositoryName);
  const originPath = path.join(root, "sources", `${repositoryName}-origin.git`);
  fs.mkdirSync(repositoryPath, { recursive: true });
  fs.writeFileSync(path.join(repositoryPath, "README.md"), LIVE_ATTACHMENT_README);
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["add", "."], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["commit", "-m", "secondary fixture"], {
    cwd: repositoryPath,
    env: gitEnv,
  });
  execFileSync("git", ["init", "--bare", "-b", "main", originPath], { env: gitEnv });
  execFileSync("git", ["remote", "add", "origin", originPath], {
    cwd: repositoryPath,
    env: gitEnv,
  });
  execFileSync("git", ["push", "--set-upstream", "origin", "main"], {
    cwd: repositoryPath,
    env: gitEnv,
  });
  return {
    repositoryPath,
    repositoryName,
    readmeContent: LIVE_ATTACHMENT_README,
    workOrderPath: path.join(repositoryPath, "README.md"),
  };
}

/** Builds the in-turn MCP attachment prompt for the mock agent scenario. */
export function liveAttachmentPrompt(repositoryPath: string): string {
  return `/e2e:live-repository-attachment ${JSON.stringify({
    local_path: repositoryPath,
    base_branch: "main",
  })}`;
}

/** Writes a primary work order and change file inside the session checkout. */
export function writePrimaryWorkOrder(primaryPath: string): void {
  const workOrderDir = path.join(primaryPath, "implementation_plans");
  fs.mkdirSync(workOrderDir, { recursive: true });
  fs.writeFileSync(path.join(workOrderDir, "task-00.md"), LIVE_ATTACHMENT_WORK_ORDER);
  fs.writeFileSync(path.join(primaryPath, "README.md"), "primary checkout readme\n");
  fs.writeFileSync(path.join(primaryPath, "primary-change.txt"), "primary change\n");
}

export function primaryCheckoutPath(session: LiveAttachmentSession): string {
  return session.worktree_path || session.worktrees?.[0]?.worktree_path || "";
}

/** Waits until the live attachment promotes the root while the agent stays put. */
export async function waitForPromotedSession(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  primaryPath: string,
  timeout = 90_000,
): Promise<LiveAttachmentSession> {
  let promoted: LiveAttachmentSession | null = null;
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = (sessions as LiveAttachmentSession[]).find((item) => item.id === sessionId);
        if (!session) return false;
        const worktrees = (session.worktrees ?? []).flatMap((worktree) =>
          worktree.worktree_path ? [worktree.worktree_path] : [],
        );
        const promotedRoot = session.workspace_path ?? "";
        const ready =
          worktrees.length >= 2 &&
          promotedRoot !== "" &&
          promotedRoot !== primaryPath &&
          session.agent_workspace_path === primaryPath &&
          worktrees.every((worktree) => worktree.startsWith(`${promotedRoot}${path.sep}`));
        if (ready) promoted = session;
        return ready;
      },
      {
        timeout,
        message: "live attachment should promote the Files root and preserve the agent cwd",
      },
    )
    .toBe(true);
  if (!promoted) throw new Error("promoted session was not captured");
  return promoted;
}
