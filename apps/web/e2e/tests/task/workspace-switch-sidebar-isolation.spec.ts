// Switching the active workspace must not leak tasks from the previous workspace into the sidebar.
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";

// Reads the port-scoped active workspace cookie first, falling back to the
// legacy unprefixed name (read-only fallback for pre-upgrade selections).
async function activeWorkspaceCookie(page: Page, port: string): Promise<string | null> {
  return page.evaluate(
    ({ baseName, port }) => {
      const names = port ? [`${baseName}_${port}`, baseName] : [baseName];
      for (const name of names) {
        const prefix = `${name}=`;
        const match = document.cookie
          .split(";")
          .map((part) => part.trim())
          .find((part) => part.startsWith(prefix));
        if (match) return decodeURIComponent(match.slice(prefix.length));
      }
      return null;
    },
    { baseName: ACTIVE_WORKSPACE_COOKIE, port },
  );
}

async function gotoTaskList(page: Page): Promise<void> {
  await page.goto("/tasks");
  await page.getByTestId("display-button").waitFor();
}

function taskInList(page: Page, title: string): Locator {
  return page.getByTestId("tasks-list").getByText(title);
}

// The production default has Office disabled. Kanban workspace navigation must
// still leave an open task route and land on the selected workspace's board.
useRegularMode();

test.describe("Sidebar — cross-workspace isolation", () => {
  test("tasks from the previous workspace do not leak after switching", async ({
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
    await apiClient.updateWorkflow(workflowB.id, {
      agent_profile_id: seedData.agentProfileId,
    });
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

    // --- Open task A so the route and task/session context belong to workspace A ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(taskA.id)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCard(taskB.id)).not.toBeVisible();
    await kanban.taskCard(taskA.id).click();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskA.id}$`));

    // --- Switch to workspace B via the sidebar workspace picker ---
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${workspaceB.id}`).click();

    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === workspaceB.id,
    );
    await expect(kanban.board).toBeVisible();
    await expect(testPage.getByTestId("dockview-task-layout")).toHaveCount(0);
    await expect(kanban.taskCard(taskB.id)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCard(taskA.id)).not.toBeVisible();
    await expect
      .poll(() => activeWorkspaceCookie(testPage, String(backend.port)))
      .toBe(workspaceB.id);

    // A hard reload and the task-list route must bootstrap the same cookie-backed
    // workspace before client effects can fall back to the seed workspace.
    await testPage.reload();
    await kanban.board.waitFor({ state: "visible" });
    await expect(kanban.taskCard(taskB.id)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCard(taskA.id)).not.toBeVisible();

    await kanban.taskCard(taskB.id).click();
    await session.waitForLoad();
    await expect(session.sidebar.getByText("Workspace B Task", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.sidebar.getByText("Workspace A Task", { exact: true })).toHaveCount(0);
    // `sidebar-repo-group-Unassigned` was never a real testid, so this assertion
    // was passing on nothing. The group header carries `data-group-key`, the
    // untranslated identity — the label beside it is catalog-backed.
    await expect(
      session.sidebar.locator(
        "[data-testid='sidebar-group-header'][data-group-key='__unassigned__']",
      ),
    ).toHaveCount(0);

    await gotoTaskList(testPage);
    await expect(taskInList(testPage, "Workspace B Task")).toBeVisible({ timeout: 10_000 });
    await expect(taskInList(testPage, "Workspace A Task")).not.toBeVisible();
  });

  test("writes the port-scoped workspace cookie and preserves the legacy name", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    const workspaceB = await apiClient.createWorkspace("Workspace B Cookie Scope");

    // Simulate a pre-upgrade jar: the legacy unprefixed cookie exists before
    // any selection is made post-upgrade.
    await testPage.addInitScript(
      ({ baseName }) => {
        document.cookie = `${baseName}=pre-upgrade; path=/`;
      },
      { baseName: ACTIVE_WORKSPACE_COOKIE },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${workspaceB.id}`).click();
    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === workspaceB.id,
    );
    await expect(kanban.board).toBeVisible();

    // The selection is recorded under the port-scoped name. The legacy
    // unprefixed name must be left byte-for-byte untouched: on a host serving
    // a default-port instance it is that instance's live selection cookie,
    // and for upgraded ported instances it is the validated migration
    // fallback (spec: no proactive legacy cookie scrubbing).
    await expect
      .poll(() => activeWorkspaceCookie(testPage, String(backend.port)))
      .toBe(workspaceB.id);
    const cookieState = await testPage.evaluate(
      ({ baseName, port }) => ({
        unprefixedValue:
          document.cookie
            .split(";")
            .map((part) => part.trim())
            .find((part) => part.startsWith(`${baseName}=`))
            ?.slice(baseName.length + 1) ?? null,
        scoped: document.cookie.includes(`${baseName}_${port}=`),
      }),
      { baseName: ACTIVE_WORKSPACE_COOKIE, port: String(backend.port) },
    );
    expect(cookieState.scoped, "scoped cookie must be present").toBe(true);
    expect(cookieState.unprefixedValue, "legacy cookie must be preserved").toBe("pre-upgrade");
  });

  test("creates only in the selected workspace after switching from an open task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workspaceA = await apiClient.createWorkspace("Workspace A Create Source");
    const { workflows: workspaceAWorkflows } = await apiClient.listWorkflows(workspaceA.id);
    const workflowA = workspaceAWorkflows[0];
    if (!workflowA) throw new Error("workspace A has no bootstrap workflow");
    const { steps: workspaceASteps } = await apiClient.listWorkflowSteps(workflowA.id);
    const startStepA = workspaceASteps.find((step) => step.is_start_step) ?? workspaceASteps[0];
    if (!startStepA) throw new Error("workspace A workflow has no steps");
    const taskA = await apiClient.createTask(workspaceA.id, "Workspace A Create Source", {
      workflow_id: workflowA.id,
      workflow_step_id: startStepA.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${workspaceA.id}`).click();
    await expect(kanban.taskCard(taskA.id)).toBeVisible({ timeout: 10_000 });
    await kanban.taskCard(taskA.id).click();
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${seedData.workspaceId}`).click();
    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === seedData.workspaceId,
    );
    await expect(kanban.board).toBeVisible();
    await expect(testPage.getByTestId("dockview-task-layout")).toHaveCount(0);
    await expect(kanban.taskCard(taskA.id)).not.toBeVisible();

    const createdTitle = "Workspace B UI Task";
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("source-mode-scratch").click();
    await dialog.getByTestId("task-title-input").fill(createdTitle);
    await dialog
      .getByTestId("task-description-input")
      .fill("Created after switching away from an open task");
    const agentProfileSelector = dialog.getByTestId("agent-profile-selector");
    await expect(agentProfileSelector).toBeVisible({ timeout: 30_000 });
    await expect(agentProfileSelector).toBeEnabled({ timeout: 30_000 });
    await expect(agentProfileSelector).not.toContainText("Select agent", { timeout: 30_000 });
    await expect(dialog.getByTestId("submit-start-agent-chevron")).toBeEnabled({
      timeout: 30_000,
    });

    const createdResponse = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await dialog.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-create-without-agent").click();
    const createdTask = (await createdResponse).json() as Promise<{ id: string }>;
    const createdTaskId = (await createdTask).id;

    await expect(dialog).not.toBeVisible();
    await expect(testPage).toHaveURL(new RegExp(`/t/${createdTaskId}$`));
    await expect(session.sidebar.getByText(createdTitle, { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expect(
      session.sidebar.getByText("Workspace A Create Source", { exact: true }),
    ).toHaveCount(0);

    // The new task is on B's board and absent after switching back to A.
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}`);
    await kanban.board.waitFor({ state: "visible" });
    await expect(kanban.taskCard(createdTaskId)).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${workspaceA.id}`).click();
    await expect(kanban.taskCard(taskA.id)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCard(createdTaskId)).not.toBeVisible();
  });
});
