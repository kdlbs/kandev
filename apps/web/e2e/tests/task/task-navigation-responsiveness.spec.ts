import { test, expect } from "../../fixtures/test-base";
import { randomUUID } from "node:crypto";
import {
  assertProgressiveNavigation,
  seedNavigationBranch,
  seedNavigationTasks,
  showNavigationFiles,
} from "./task-navigation-helpers";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { routeNavigationResponses } from "../../helpers/navigation-response-hold";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";
import { expandDisplaySettingsGroup } from "../../helpers/display-settings";

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
test("navigation branch setup leaves a dirty shared checkout untouched", async ({
  backend,
  seedData,
}) => {
  const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
  const originalBranch = git.exec("git branch --show-current").trim();
  const dirtyBranch = `e2e-navigation-dirty-${randomUUID()}`;
  const mainFile = git.exec("git show origin/main:walkthrough_base.txt");
  git.exec(`git checkout -b ${dirtyBranch} origin/main`);
  const committedBranchFile = `${mainFile}committed navigation branch state\n`;
  git.modifyFile("walkthrough_base.txt", committedBranchFile);
  git.stageFile("walkthrough_base.txt");
  git.commit("seed divergent navigation checkout");
  git.modifyFile(
    "walkthrough_base.txt",
    `${committedBranchFile}navigation fixture dirty-checkout sentinel\n`,
  );
  let seededBranch: string | undefined;
  try {
    seededBranch = seedNavigationBranch(backend);
    expect(git.exec("git branch --show-current").trim()).toBe(dirtyBranch);
    expect(git.exec("git diff -- walkthrough_base.txt")).toContain(
      "navigation fixture dirty-checkout sentinel",
    );
    expect(git.exec(`git ls-remote --heads origin ${seededBranch}`)).toContain(seededBranch);
  } finally {
    git.exec("git restore -- walkthrough_base.txt");
    git.exec(`git checkout ${originalBranch}`);
    git.exec(`git branch -D ${dirtyBranch}`);
    if (seededBranch) {
      git.exec(`git push origin --delete ${seededBranch}`);
      if (git.exec(`git branch --list ${seededBranch}`).trim())
        git.exec(`git branch -D ${seededBranch}`);
    }
  }
});

test("Files stays usable during restoration, retry, and return navigation", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(90_000);
  await assertProgressiveNavigation(testPage, apiClient, seedData, backend, false);
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
test("mounted task panels share pending shell, commit, and diff requests", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const [a] = await seedNavigationTasks(apiClient, seedData, backend);
  const gate = await routeNavigationResponses(testPage);
  const actions = ["user_shell.list", "session.git.commits", "session.cumulative_diff"];
  gate.hold((r) => actions.includes(r.action));
  await testPage.goto(`/t/${a.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await showNavigationFiles(testPage, false);
  await session.clickTab("Changes");
  await expect
    .poll(
      () =>
        new Set(gate.requests.filter((r) => actions.includes(r.action)).map((r) => r.action)).size,
    )
    .toBe(3);
  await dwell(
    testPage,
    400,
    "negative-assertion",
    "observe duplicate initial reads while all matching replies remain held and panels finish mounting",
  );
  const counts = new Map<string, number>();
  for (const request of gate.requests.filter((r) => actions.includes(r.action))) {
    const key = JSON.stringify([request.action, request.payload]);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  expect([...counts.values()]).toEqual([...counts.values()].map(() => 1));
  gate.release();
});

// The real-store hook test supplies RED for the missing projection; boot hydration
// avoids that race in this fixture. This browser case protects workflow filtering.
// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.1
test("overview keeps workflow filters scoped", async ({ testPage, apiClient, seedData }) => {
  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Second navigation workflow",
    "simple",
  );
  const steps = (await apiClient.listWorkflowSteps(workflow.id)).steps;
  const delayed = await apiClient.createTask(seedData.workspaceId, "Second navigation task", {
    workflow_id: workflow.id,
    workflow_step_id: steps[0].id,
  });
  const available = await apiClient.createTask(seedData.workspaceId, "Available navigation task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.saveUserSettings({ workspace_id: seedData.workspaceId, workflow_filter_id: "" });
  try {
    const kanban = new KanbanPage(testPage);
    await testPage.goto("/?home=overview");
    await expect(kanban.taskCard(available.id)).toBeVisible();
    await expect(kanban.taskCard(delayed.id)).toBeVisible();
    await expect(testPage.getByTestId("display-button")).toBeEnabled();
    await testPage.getByTestId("display-button").click();
    await expandDisplaySettingsGroup(testPage, "filters");
    await testPage.getByTestId("display-workflow-filter").click();
    await testPage
      .getByRole("listbox")
      .getByRole("option", { name: workflow.name, exact: true })
      .click();
    await testPage.keyboard.press("Escape");
    await expect(kanban.taskCard(delayed.id)).toBeVisible();
    await expect(kanban.taskCard(available.id)).toHaveCount(0);
  } finally {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
    });
    await apiClient.deleteWorkflow(workflow.id);
  }
});
