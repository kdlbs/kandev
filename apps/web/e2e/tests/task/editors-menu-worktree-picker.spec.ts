import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { makeGitEnv } from "../../helpers/git-helper";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT", "IDLE"];

/**
 * Multi-repo tasks create one worktree per repository. The IDE button in the
 * task top bar must offer a worktree picker instead of silently opening the
 * first worktree (regression: it always opened Worktrees[0]).
 */
test.describe("Editors menu worktree picker", () => {
  test("multi-repo task offers a worktree choice on the IDE button", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Two disposable repos so the session materializes two worktrees.
    const gitEnv = makeGitEnv(backend.tmpDir);
    const repoIds: string[] = [];
    for (const name of ["orders-api", "storefront-web"]) {
      const repoDir = path.join(backend.tmpDir, "repos", name);
      fs.mkdirSync(repoDir, { recursive: true });
      execSync("git init -b main", { cwd: repoDir, env: gitEnv });
      execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
      const repo = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
        name,
      });
      repoIds.push(repo.id);
    }

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sync order totals across services",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: repoIds,
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 60_000, message: "Waiting for the agent session to finish" },
      )
      .toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    // Let the chat composer finish its delayed autofocus — a focus steal
    // after the menu opens would dismiss the Radix dropdown.
    await testPage.waitForTimeout(750);

    // The IDE button now opens a worktree picker instead of launching directly.
    await testPage.getByTestId("editors-menu-open").click();
    const items = testPage.getByTestId("editors-menu-worktree-item");
    await expect(items).toHaveCount(2);
    await expect(items.filter({ hasText: "orders-api" })).toBeVisible();
    await expect(items.filter({ hasText: "storefront-web" })).toBeVisible();
  });
});
