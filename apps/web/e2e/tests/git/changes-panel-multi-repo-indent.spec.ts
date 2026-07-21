import { expect, test } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

async function expectTreeNestedUnderRepository(panel: Locator) {
  const group = panel.getByTestId("changes-repo-group").first();
  const sectionHeader = panel.getByTestId("unstaged-files-section-collapse-toggle");
  const repositoryLabel = group.getByTestId("changes-repo-header").locator("span").first();
  const directoryLabel = group.locator("[data-testid^='tree-dir-'] span").first();
  const [sectionHeaderBox, repositoryBox, directoryBox] = await Promise.all([
    sectionHeader.boundingBox(),
    repositoryLabel.boundingBox(),
    directoryLabel.boundingBox(),
  ]);

  expect(sectionHeaderBox).not.toBeNull();
  expect(repositoryBox).not.toBeNull();
  expect(directoryBox).not.toBeNull();
  expect(repositoryBox!.x - sectionHeaderBox!.x).toBeLessThanOrEqual(14);
  expect(directoryBox!.x).toBeGreaterThan(repositoryBox!.x);
}

test("nests multi-repository changes below each repository on desktop and mobile", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);

  const gitEnv = makeGitEnv(backend.tmpDir);
  const extraRepoDir = path.join(backend.tmpDir, "repos", "indent-extra-repo");
  fs.mkdirSync(extraRepoDir, { recursive: true });
  execSync("git init -b main", { cwd: extraRepoDir, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: extraRepoDir, env: gitEnv });
  const extraRepo = await apiClient.createRepository(seedData.workspaceId, extraRepoDir, "main", {
    name: "indent-extra-repo",
  });

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Multi-Repo Changes Indentation",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId, extraRepo.id],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  const sessionData = (await apiClient.listTaskSessions(task.id)) as {
    sessions: Array<{ worktrees?: Array<{ worktree_path?: string }> }>;
  };
  const repoPaths = (sessionData.sessions[0]?.worktrees ?? []).flatMap((worktree) =>
    worktree.worktree_path ? [worktree.worktree_path] : [],
  );
  expect(repoPaths).toHaveLength(2);
  for (const [index, repoPath] of repoPaths.entries()) {
    new GitHelper(repoPath, gitEnv).createFile(`src/repo-${index}.ts`, `repo ${index}\n`);
  }

  await session.clickTab("Changes");
  await expect(session.changes.getByTestId("changes-repo-group")).toHaveCount(2, {
    timeout: 30_000,
  });
  await expectTreeNestedUnderRepository(session.changes);

  await testPage.setViewportSize({ width: 393, height: 851 });
  await testPage.getByRole("button", { name: "Changes" }).click();
  const mobilePanel = testPage.getByTestId("mobile-changes-panel");
  await expect(mobilePanel).toBeVisible();
  await expectTreeNestedUnderRepository(mobilePanel);
});
