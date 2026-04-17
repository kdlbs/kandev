// Switching the active workspace must not leak tasks from the previous workspace into the sidebar.
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Sidebar — cross-workspace isolation", () => {
  test("tasks from the previous workspace do not leak into the sidebar after switching", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // --- Seed workspace A artifacts ---
    const taskA = await apiClient.createTask(seedData.workspaceId, "Workspace A Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // --- Create workspace B with its own workflow, repo, and task ---
    const workspaceB = await apiClient.createWorkspace("Workspace B");
    const workflowB = await apiClient.createWorkflow(workspaceB.id, "Workflow B", "simple");
    const { steps: stepsB } = await apiClient.listWorkflowSteps(workflowB.id);
    const startStepB = [...stepsB]
      .sort((a, b) => a.position - b.position)
      .find((s) => s.is_start_step);
    if (!startStepB) throw new Error("workspace B workflow has no start step");

    const repoBDir = path.join(backend.tmpDir, "repos", "e2e-repo-b");
    fs.mkdirSync(repoBDir, { recursive: true });
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    execSync("git init -b main", { cwd: repoBDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoBDir, env: gitEnv });
    const repoB = await apiClient.createRepository(workspaceB.id, repoBDir);

    const taskB = await apiClient.createTask(workspaceB.id, "Workspace B Task", {
      workflow_id: workflowB.id,
      workflow_step_id: startStepB.id,
      repository_ids: [repoB.id],
    });

    // --- Land on kanban with workspace A active; task A visible, task B not ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(taskA.id)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCard(taskB.id)).not.toBeVisible();

    // --- Switch to workspace B via the display dropdown (SPA, no full reload) ---
    await testPage.getByTestId("display-button").click();
    await testPage.getByTestId("workspace-select-trigger").click();
    await testPage.getByTestId(`workspace-select-item-${workspaceB.id}`).click();

    // Close the dropdown so task cards are interactable.
    await testPage.keyboard.press("Escape");

    await expect(kanban.taskCard(taskB.id)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCard(taskA.id)).not.toBeVisible();

    // --- Open task B; verify sidebar shows only workspace B's task ---
    await kanban.taskCard(taskB.id).click();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    await expect(session.sidebar.getByText("Workspace B Task", { exact: true })).toBeVisible({
      timeout: 10_000,
    });

    // The core regression assertion: task A must not appear anywhere in the
    // sidebar — not under "Unassigned", not under any repo group.
    await expect(session.sidebar.getByText("Workspace A Task", { exact: true })).toHaveCount(0);
    await expect(session.sidebar.getByTestId("sidebar-repo-group-Unassigned")).toHaveCount(0);
  });
});
