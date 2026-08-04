// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("mobile: multi-repository session picker", () => {
  test("shows repository context and switches to a session on another repository", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const primaryRepoDir = path.join(backend.tmpDir, "repos", "mobile-session-primary");
    const secondaryRepoDir = path.join(backend.tmpDir, "repos", "mobile-session-secondary");
    fs.mkdirSync(primaryRepoDir, { recursive: true });
    fs.mkdirSync(secondaryRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: primaryRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: primaryRepoDir, env: gitEnv });
    execSync("git init -b main", { cwd: secondaryRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: secondaryRepoDir, env: gitEnv });
    const primaryRepo = await apiClient.createRepository(
      seedData.workspaceId,
      primaryRepoDir,
      "main",
      { name: "Mobile primary" },
    );
    const secondaryRepo = await apiClient.createRepository(
      seedData.workspaceId,
      secondaryRepoDir,
      "main",
      { name: "Mobile secondary" },
    );
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile repository sessions",
      seedData.agentProfileId,
      {
        description: 'e2e:message("primary repository session")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [primaryRepo.id, secondaryRepo.id],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("multi-repository task has no primary session");

    const websocketConnected = testPage.waitForEvent("websocket");
    await testPage.goto(`/t/${task.id}`);
    await websocketConnected;

    const layout = testPage.locator("[data-testid='mobile-task-layout']:visible");
    const pill = layout.getByTestId("mobile-sessions-pill");
    await expect(layout).toHaveCount(1);
    await expect(pill).toHaveAccessibleName(/^Active session: .+\. Tap to switch\.$/);
    await expect(pill).not.toHaveAccessibleName(/Repository:/);

    const secondarySession = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: `mobile-secondary-${task.id}`,
      repositoryId: secondaryRepo.id,
      startedAt: "2026-01-01T00:01:00Z",
    });

    await expect(pill).toHaveAccessibleName(/Repository: Mobile primary/);
    await expect(pill).toContainText("Mobile primary");

    await pill.tap();
    const primaryRow = testPage.getByTestId(`mobile-session-row-${task.session_id}`);
    const secondaryRow = testPage.getByTestId(`mobile-session-row-${secondarySession.session_id}`);
    await expect(primaryRow.getByText("Mobile primary", { exact: true })).toBeVisible();
    await expect(secondaryRow.getByText("Mobile secondary", { exact: true })).toBeVisible();

    await secondaryRow.tap();
    await expect(pill).toHaveAccessibleName(
      "Active session: Agent. Repository: Mobile secondary. Tap to switch.",
    );
    await expect(pill).toContainText("Agent · Mobile secondary");

    await pill.tap();
    await expect(secondaryRow).toHaveAttribute("aria-current", "true");
    await assertNoDocumentHorizontalOverflow(testPage, "multi-repository session picker");
  });
});
