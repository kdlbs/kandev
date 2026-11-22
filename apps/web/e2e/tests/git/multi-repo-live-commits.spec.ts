import { devices } from "@playwright/test";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import type { GitCommitCreatedEvent } from "../../../lib/types/git-events";

// @covers AC-TASKS-MULTI-BRANCH-001.2
test("keeps live commits from both repositories visible on desktop and phone", async ({
  testPage,
  browser,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  test.setTimeout(90_000);
  const gitEnv = makeGitEnv(backend.tmpDir);
  const git = (cwd: string, ...args: string[]) =>
    execFileSync("git", args, { cwd, env: gitEnv, encoding: "utf8" }).trim();
  const sourcePaths = ["account-api", "account-web"].map((name) =>
    path.join(backend.tmpDir, "repos", name),
  );
  fs.mkdirSync(sourcePaths[0], { recursive: true });
  git(sourcePaths[0], "init", "-b", "main");
  fs.writeFileSync(path.join(sourcePaths[0], "README.md"), "Account settings\n");
  git(sourcePaths[0], "add", "README.md");
  git(sourcePaths[0], "commit", "-m", "initial account settings");
  git(backend.tmpDir, "clone", "--no-hardlinks", sourcePaths[0], sourcePaths[1]);
  git(sourcePaths[1], "remote", "remove", "origin");

  const repositories = [];
  for (const sourcePath of sourcePaths) {
    repositories.push(
      await apiClient.createRepository(seedData.workspaceId, sourcePath, "main", {
        name: path.basename(sourcePath),
        pull_before_worktree: false,
      }),
    );
  }
  const watcher = watchWs(testPage);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Update account settings",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: repositories.map((repository) => repository.id),
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const desktop = new SessionPage(testPage);
  await desktop.waitForLoad();
  await desktop.waitForChatIdle({ timeout: 30_000 });
  await desktop.clickTab("Changes");

  const sessions = (await apiClient.listTaskSessions(task.id)) as {
    sessions: Array<{ worktrees?: Array<{ repository_id: string; worktree_path: string }> }>;
  };
  const repoPaths = repositories.map((repository) => {
    const worktree = sessions.sessions[0]?.worktrees?.find(
      (entry) => entry.repository_id === repository.id,
    );
    expect(worktree?.worktree_path).toBeTruthy();
    return worktree!.worktree_path;
  });
  expect(git(repoPaths[0], "rev-parse", "HEAD")).toBe(git(repoPaths[1], "rev-parse", "HEAD"));

  const mobileContext = await browser.newContext({
    ...devices["Pixel 5"],
    baseURL: backend.baseUrl,
  });
  try {
    const mobile = await mobileContext.newPage();
    await mobile.goto(`/t/${task.id}`);
    await new SessionPage(mobile).waitForLoad();
    await mobile
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .tap();
    const mobilePanel = mobile.getByTestId("mobile-changes-panel");
    await expect(mobilePanel).toBeVisible();

    const commitEvents = repoPaths.map((repoPath) =>
      watcher.waitForEvent("session.git.event", {
        where: (payload) => {
          const event = payload as GitCommitCreatedEvent | undefined;
          return (
            event?.type === "commit_created" &&
            event.commit.repository_name === path.basename(repoPath)
          );
        },
      }),
    );
    const messages = ["fix: validate account settings", "fix: refresh account settings form"];
    const commits = repoPaths.map((repoPath, index) => {
      fs.writeFileSync(path.join(repoPath, "README.md"), `Account settings revision ${index}\n`);
      git(repoPath, "add", "README.md");
      git(repoPath, "commit", "-m", messages[index]);
      return git(repoPath, "rev-parse", "HEAD");
    });
    const received = await Promise.all(commitEvents);
    expect(
      received.map((frame) => (frame.payload as GitCommitCreatedEvent).commit.commit_sha).sort(),
    ).toEqual([...commits].sort());
    for (const sha of commits) {
      const row = `commit-row-${sha.slice(0, 7)}`;
      await expect(desktop.changes.getByTestId(row)).toBeVisible();
      await expect(mobilePanel.getByTestId(row)).toBeVisible();
    }
    if (prCapture.capturing) {
      const terminalTab = testPage.locator(".dv-tab").filter({ hasText: /^Terminal$/ });
      await terminalTab.locator(".dv-default-tab-action").click();
      const confirmation = testPage.getByRole("dialog", { name: "Close terminal?" });
      await expect(confirmation).toBeVisible();
      await confirmation.getByRole("button", { name: "Close terminal" }).click();
      await expect(confirmation).not.toBeVisible();
    }
    await prCapture.screenshot("desktop", {
      caption: "Both repository commits remain visible after live updates, without a PR or reload.",
    });
    await prCapture.screenshot("mobile", {
      page: mobile,
      caption: "Phone Changes retains both repository histories from the same live updates.",
    });
  } finally {
    await mobileContext.close();
  }
});
