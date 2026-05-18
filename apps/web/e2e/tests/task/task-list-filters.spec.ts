import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { WorkflowStep } from "../../../lib/types/http";

const TASK_VISIBLE_TIMEOUT = 10_000;

function findStartStep(steps: WorkflowStep[]): WorkflowStep {
  return steps.find((s) => s.is_start_step) ?? steps[0];
}

async function gotoTasksPage(page: Page): Promise<void> {
  await page.goto("/tasks");
  // Wait for a header landmark instead of networkidle — persistent WebSocket
  // connections can keep the network "active" and make networkidle unreliable.
  await page.getByTestId("display-button").waitFor();
}

async function pickListboxOption(page: Page, optionLabel: string): Promise<void> {
  // Radix Select renders the currently-selected item twice (once in the trigger
  // for SelectValue display, once in the listbox). Scope to the listbox so we
  // click the real option.
  const listbox = page.getByRole("listbox");
  await listbox.getByRole("option", { name: optionLabel, exact: true }).click();
  // Wait for the Select to fully unmount — until then Radix's pointer-events
  // overlay intercepts subsequent clicks on the <html> element.
  await expect(listbox).toHaveCount(0);
}

async function closeDisplayDropdown(page: Page): Promise<void> {
  // The Display DropdownMenu does NOT auto-close when an inner Select option
  // is picked. Click the trigger to toggle closed. Radix leaves a brief
  // pointer-events overlay on <html> after the inner Select closes, so use
  // force: true to bypass Playwright's interception check.
  const trigger = page.getByTestId("display-button");
  if ((await trigger.getAttribute("data-state")) === "open") {
    await trigger.click({ force: true });
  }
  await expect(trigger).not.toHaveAttribute("data-state", "open");
  // Wait for the menu portal to fully unmount before any subsequent open;
  // otherwise re-clicking the trigger lands while Radix is mid-cleanup and
  // the menu content fails to render.
  await expect(page.getByRole("menu")).toHaveCount(0);
}

async function selectWorkflowFilter(page: Page, optionLabel: string): Promise<void> {
  await page.getByTestId("display-button").click();
  await page.getByTestId("display-workflow-filter").click();
  await pickListboxOption(page, optionLabel);
  await closeDisplayDropdown(page);
}

async function selectRepositoryFilter(page: Page, optionLabel: string): Promise<void> {
  await page.getByTestId("display-button").click();
  await page.getByTestId("display-repository-filter").click();
  await pickListboxOption(page, optionLabel);
  await closeDisplayDropdown(page);
}

async function createLocalRepo(tmpDir: string, name: string): Promise<string> {
  const repoDir = path.join(tmpDir, "repos", name);
  fs.mkdirSync(repoDir, { recursive: true });
  const gitEnv = {
    ...process.env,
    HOME: tmpDir,
    GIT_AUTHOR_NAME: "E2E Test",
    GIT_AUTHOR_EMAIL: "e2e@test.local",
    GIT_COMMITTER_NAME: "E2E Test",
    GIT_COMMITTER_EMAIL: "e2e@test.local",
  };
  execSync("git init -b main", { cwd: repoDir, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
  return repoDir;
}

test.describe("Task list display filters", () => {
  // The testPage fixture resets workflow_filter_id between tests but not
  // repository_ids — clear it so a leftover repo filter doesn't hide tasks
  // in subsequent test files (e.g. task-list.spec.ts).
  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      repository_ids: [],
    });
  });

  test("workflow filter narrows the list to the selected workflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Workflow B", "simple");
    const stepsB = (await apiClient.listWorkflowSteps(workflowB.id)).steps;
    const startB = findStartStep(stepsB);

    await apiClient.createTask(seedData.workspaceId, "Alpha task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Beta task", {
      workflow_id: workflowB.id,
      workflow_step_id: startB.id,
    });

    await gotoTasksPage(testPage);

    // testPage fixture pre-selects seedData.workflowId — only Alpha is visible initially.
    await expect(testPage.getByText("Alpha task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(testPage.getByText("Beta task")).not.toBeVisible();

    await selectWorkflowFilter(testPage, "Workflow B");

    await expect(testPage.getByText("Beta task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(testPage.getByText("Alpha task")).not.toBeVisible();
  });

  test("'All Workflows' shows tasks from every workflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Workflow B", "simple");
    const stepsB = (await apiClient.listWorkflowSteps(workflowB.id)).steps;
    const startB = findStartStep(stepsB);

    await apiClient.createTask(seedData.workspaceId, "Alpha task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Beta task", {
      workflow_id: workflowB.id,
      workflow_step_id: startB.id,
    });

    await gotoTasksPage(testPage);
    await expect(testPage.getByText("Alpha task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(testPage.getByText("Beta task")).not.toBeVisible();

    await selectWorkflowFilter(testPage, "All Workflows");

    await expect(testPage.getByText("Alpha task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(testPage.getByText("Beta task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
  });

  test("'All Workflows' is preserved when navigating with workspace query param", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Workflow B", "simple");
    const stepsB = (await apiClient.listWorkflowSteps(workflowB.id)).steps;
    const startB = findStartStep(stepsB);

    await apiClient.createTask(seedData.workspaceId, "Alpha task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Beta task", {
      workflow_id: workflowB.id,
      workflow_step_id: startB.id,
    });

    // Persist "All Workflows" (empty workflow_filter_id) before navigating.
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
    });

    // Navigate to /tasks with the workspace query param — the SSR resolver
    // must not silently fall back to the first workflow when "All Workflows"
    // is the saved selection.
    await testPage.goto(`/tasks?workspace=${seedData.workspaceId}`);
    await testPage.getByTestId("display-button").waitFor();

    await expect(testPage.getByText("Alpha task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(testPage.getByText("Beta task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
  });

  test("repository filter narrows the list to the selected repository", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Repository names are unique per test because the e2e reset endpoint
    // doesn't clear repositories — they persist across tests in the worker.
    const repoName = "Repo Filter T3";
    const repoPath = await createLocalRepo(backend.tmpDir, "e2e-repo-filter-t3");
    const otherRepo = await apiClient.createRepository(seedData.workspaceId, repoPath, "main", {
      name: repoName,
    });

    await apiClient.createTask(seedData.workspaceId, "Task on seed repo", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "Task on other repo", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [otherRepo.id],
    });
    await apiClient.createTask(seedData.workspaceId, "Task with no repo", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await gotoTasksPage(testPage);
    await expect(testPage.getByText("Task on seed repo")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(testPage.getByText("Task on other repo")).toBeVisible();
    await expect(testPage.getByText("Task with no repo")).toBeVisible();

    await selectRepositoryFilter(testPage, repoName);

    await expect(testPage.getByText("Task on other repo")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(testPage.getByText("Task on seed repo")).not.toBeVisible();
    await expect(testPage.getByText("Task with no repo")).not.toBeVisible();
  });

  test("'All repositories' restores tasks across all repositories", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoName = "Repo Filter T4";
    const repoPath = await createLocalRepo(backend.tmpDir, "e2e-repo-filter-t4");
    const otherRepo = await apiClient.createRepository(seedData.workspaceId, repoPath, "main", {
      name: repoName,
    });

    await apiClient.createTask(seedData.workspaceId, "Task on seed repo", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "Task on other repo", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [otherRepo.id],
    });
    await apiClient.createTask(seedData.workspaceId, "Task with no repo", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await gotoTasksPage(testPage);
    await selectRepositoryFilter(testPage, repoName);
    await expect(testPage.getByText("Task on seed repo")).not.toBeVisible();
    await expect(testPage.getByText("Task on other repo")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });

    await selectRepositoryFilter(testPage, "All repositories");

    await expect(testPage.getByText("Task on seed repo")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(testPage.getByText("Task on other repo")).toBeVisible();
    await expect(testPage.getByText("Task with no repo")).toBeVisible();
  });

  test("workflow + repository filters combine", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Workflow B", "simple");
    const stepsB = (await apiClient.listWorkflowSteps(workflowB.id)).steps;
    const startB = findStartStep(stepsB);
    const repoName = "Repo Filter T5";
    const repoPath = await createLocalRepo(backend.tmpDir, "e2e-repo-filter-t5");
    const otherRepo = await apiClient.createRepository(seedData.workspaceId, repoPath, "main", {
      name: repoName,
    });

    // Only t1 matches workflow=seed AND repo=seed.
    await apiClient.createTask(seedData.workspaceId, "T1 wf-A repo-1", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "T2 wf-A repo-2", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [otherRepo.id],
    });
    await apiClient.createTask(seedData.workspaceId, "T3 wf-B repo-1", {
      workflow_id: workflowB.id,
      workflow_step_id: startB.id,
      repository_ids: [seedData.repositoryId],
    });

    await gotoTasksPage(testPage);
    // Initial: workflow=seed, repo=all → T1 + T2.
    await expect(testPage.getByText("T1 wf-A repo-1")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(testPage.getByText("T2 wf-A repo-2")).toBeVisible();
    await expect(testPage.getByText("T3 wf-B repo-1")).not.toBeVisible();

    await selectRepositoryFilter(testPage, "E2E Repo");

    // Workflow=seed AND repo=seed → only T1.
    await expect(testPage.getByText("T1 wf-A repo-1")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
    await expect(testPage.getByText("T2 wf-A repo-2")).not.toBeVisible();
    await expect(testPage.getByText("T3 wf-B repo-1")).not.toBeVisible();
  });
});
