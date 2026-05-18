import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const START_AGENT_TEST_ID = "submit-start-agent";
const START_ENABLED_TIMEOUT = 30_000;
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type TaskWithRepos = {
  id: string;
  repositories?: Array<{ repository_id: string; base_branch?: string }>;
};

type SessionInfo = {
  id: string;
  agent_profile_id: string;
  executor_profile_id: string;
  state: string;
};

test.describe("Subtask basics", () => {
  test("subtask badge visible on kanban card", async ({ testPage, apiClient, seedData }) => {
    // Create parent task via API
    const parent = await apiClient.createTask(seedData.workspaceId, "Parent Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // Create subtask via API with parent_id
    await apiClient.createTask(seedData.workspaceId, "Child Subtask", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Parent card is visible but does NOT have subtask badge
    const parentCard = kanban.taskCardByTitle("Parent Task");
    await expect(parentCard).toBeVisible({ timeout: 10_000 });
    await expect(parentCard.getByText("Parent Task", { exact: true }).first()).toBeVisible();

    // Subtask card is visible and HAS badge showing parent title
    const subtaskCard = kanban.taskCardByTitle("Child Subtask");
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
    // Badge now shows parent task title instead of generic "Subtask"
    await expect(subtaskCard.getByText("Parent Task")).toBeVisible();
  });

  test("create subtask from sidebar header button", async ({ testPage, apiClient, seedData }) => {
    // Create a task with an agent so we have a session to navigate to
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Subtask Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Navigate to the session page
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for agent to complete
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Open the Task split-button chevron and click "New Subtask"
    const primary = testPage.getByTestId("new-task-primary");
    const chevron = testPage.getByTestId("new-task-chevron");
    await expect(primary).toBeVisible({ timeout: 5_000 });
    await expect(chevron).toBeVisible({ timeout: 5_000 });
    await expect(primary).toHaveCSS("border-top-right-radius", "0px");
    await expect(primary).toHaveCSS("border-bottom-right-radius", "0px");
    await expect(chevron).toHaveCSS("border-top-left-radius", "0px");
    await expect(chevron).toHaveCSS("border-bottom-left-radius", "0px");
    await chevron.click();
    await testPage.getByTestId("new-subtask-button").click();

    // The compact NewSubtaskDialog should open with pre-filled title containing numeric suffix
    const titleInput = testPage.getByTestId("subtask-title-input");
    await expect(titleInput).toBeVisible({ timeout: 5_000 });
    await expect(titleInput).toHaveValue(/Subtask Parent \/ Subtask \d+/);

    // Fill prompt and submit
    const promptInput = testPage.getByTestId("subtask-prompt-input");
    await expect(promptInput).toBeVisible();
    await promptInput.fill("/e2e:simple-message");

    const submitBtn = testPage.getByRole("button", { name: "Create Subtask" });
    await submitBtn.click();
    await expect(titleInput).not.toBeVisible({ timeout: 10_000 });

    // After creation, we navigate to the new subtask's session
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // Verify the subtask card appears on the kanban board with "Subtask" badge
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const subtaskCard = kanban.taskCardByTitle(/Subtask Parent \/ Subtask \d+/);
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
  });
});

test.describe("MCP subtask creation", () => {
  test("agent creates subtask via MCP create_task with parent_id", async ({ testPage }) => {
    const subtaskTitle = "MCP-subtask-e2e-verify";

    const script = [
      'e2e:thinking("Planning subtasks...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify MCP create_task with parent_id"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 1. Create parent task via UI dialog
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("MCP Subtask Parent");
    await testPage.getByTestId("task-description-input").fill(script);

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // 2. Click the parent task card to navigate to its session
    const parentCard = kanban.taskCardByTitle("MCP Subtask Parent");
    await expect(parentCard).toBeVisible({ timeout: 10_000 });
    await parentCard.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // 3. Wait for the agent to complete — the MCP create_task call happens during execution
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 4. Go back to kanban — subtask card should be visible with parent badge
    await kanban.goto();

    const subtaskCard = kanban.taskCardByTitle(subtaskTitle);
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
    await expect(subtaskCard.getByText("MCP Subtask Parent")).toBeVisible();
  });

  test("MCP-created subtask inherits parent task repositories", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const subtaskTitle = "Repo-Inherit Subtask E2E";

    const script = [
      'e2e:thinking("Creating subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify repository inheritance"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 1. Create parent task via API with an explicit repository. Without the
    //    fix, the MCP handler would not copy repositories to the subtask and
    //    it would run as a repository-less "quick chat" session.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Repo Inherit Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    expect(parentTask.id).toBeTruthy();

    // 2. Navigate to parent task so the agent session is visible and the
    //    backend keeps the execution alive for the duration of the test.
    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // 3. Poll the API until the subtask appears. The agent starts
    //    asynchronously when created via the HTTP API, so we cannot rely on
    //    the idle-input indicator alone — it may appear before the agent has
    //    finished executing the MCP script.
    type TaskEntry = { id: string; title: string };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const allTasks = await apiClient.listTasks(seedData.workspaceId);
          subtask = allTasks.tasks.find((t: TaskEntry) => t.title === subtaskTitle);
          return subtask;
        },
        { timeout: 60_000, message: "Subtask should be created by the agent's MCP call" },
      )
      .toBeDefined();

    // 4. Verify the subtask inherited the parent's repository.
    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}`);
    const subtaskData = (await subtaskRaw.json()) as TaskWithRepos;
    expect(
      subtaskData.repositories?.length,
      `subtask should have at least 1 repository; got: ${JSON.stringify(subtaskData.repositories)}`,
    ).toBeGreaterThanOrEqual(1);
    expect(
      subtaskData.repositories?.[0]?.repository_id,
      `subtask repository_id should match parent's (${seedData.repositoryId})`,
    ).toBe(seedData.repositoryId);
  });

  test("user creates subtask via sidebar button", async ({ testPage }) => {
    // 1. Create parent task via UI dialog with a simple agent script
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Subtask Button Parent");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // 2. Click the parent card to navigate to its session
    const parentCard = kanban.taskCardByTitle("Subtask Button Parent");
    await expect(parentCard).toBeVisible({ timeout: 10_000 });
    await parentCard.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // 3. Wait for the agent to finish
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 4. Open the Task split-button chevron and click "New Subtask"
    await testPage.getByTestId("new-task-chevron").click();
    await testPage.getByTestId("new-subtask-button").click();
    const subtaskTitleInput = testPage.getByTestId("subtask-title-input");
    await expect(subtaskTitleInput).toBeVisible();

    // Title should be pre-filled with "Parent / Subtask N" pattern
    await expect(subtaskTitleInput).toHaveValue(/Subtask Button Parent \/ Subtask \d+/);

    // 5. Fill the prompt and submit
    const parentUrl = testPage.url();
    await testPage.getByTestId("subtask-prompt-input").fill("/e2e:simple-message");
    await testPage.getByRole("button", { name: "Create Subtask" }).click();

    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await expect(testPage).not.toHaveURL(parentUrl);

    // 6. Go back to kanban — subtask card should be visible with parent badge
    await kanban.goto();
    const subtaskCard = kanban.taskCardByTitle(/Subtask Button Parent \/ Subtask \d+/);
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
  });

  test("user creates subtask in a different repository via the repo chooser", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // 1. Add a second workspace repository so the New Subtask dialog's
    //    repo chip can be switched to a different repo.
    const otherRepoName = "UI Subtask Override Target";
    const otherRepoDir = path.join(backend.tmpDir, "repos", "ui-subtask-override");
    fs.mkdirSync(otherRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: otherRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: otherRepoDir, env: gitEnv });
    const otherRepo = await apiClient.createRepository(seedData.workspaceId, otherRepoDir, "main", {
      name: otherRepoName,
    });

    // 2. Create a parent task on repo A with a simple agent script so we can
    //    open its session and reach the subtask split-button.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "UI Override Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 3. Open the New Subtask dialog from the split-button.
    await testPage.getByTestId("new-task-chevron").click();
    await testPage.getByTestId("new-subtask-button").click();

    const titleInput = testPage.getByTestId("subtask-title-input");
    await expect(titleInput).toBeVisible({ timeout: 5_000 });

    // 4. Open the repo chip and switch the parent's repo for repo B. The
    //    subtask dialog now uses the same chip-row UX as the create-task
    //    dialog (RepoChipsRow), so we drive it via repo-chip-trigger and
    //    the combobox option role.
    const repoChip = testPage.getByTestId("repo-chip-trigger").first();
    await expect(repoChip).toBeVisible({ timeout: 5_000 });
    await repoChip.click();
    // The combobox option label includes a truncated-path badge alongside the
    // repo name, so match by name substring rather than exact.
    await testPage.getByRole("option", { name: new RegExp(otherRepoName) }).click();
    // After selection, the chip should now show repo B's name.
    await expect(repoChip).toContainText(otherRepoName);

    // 5. Submit. Use a unique title so the listTasks poll can find this row.
    const subtaskTitle = `UI Override Subtask ${Date.now()}`;
    await titleInput.fill(subtaskTitle);
    await testPage.getByTestId("subtask-prompt-input").fill("/e2e:simple-message");
    await testPage.getByRole("button", { name: "Create Subtask" }).click();
    await expect(titleInput).not.toBeVisible({ timeout: 10_000 });

    // 6. Find the subtask via API and verify it landed on the override repo,
    //    not the parent's repo.
    type TaskEntry = { id: string; title: string };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const allTasks = await apiClient.listTasks(seedData.workspaceId);
          subtask = allTasks.tasks.find((t: TaskEntry) => t.title === subtaskTitle);
          return subtask;
        },
        { timeout: 30_000, message: "Subtask should be created from the dialog" },
      )
      .toBeDefined();

    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}`);
    const subtaskData = (await subtaskRaw.json()) as TaskWithRepos;
    const repoIds = subtaskData.repositories?.map((r) => r.repository_id) ?? [];
    expect(
      repoIds,
      `subtask should target the override repository (${otherRepo.id}), not the parent's (${seedData.repositoryId}); got: ${JSON.stringify(repoIds)}`,
    ).toContain(otherRepo.id);
    expect(repoIds).not.toContain(seedData.repositoryId);
  });

  test("MCP-created subtask can override parent task repository", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // 1. Create a second workspace repository so the subtask has somewhere
    //    different to point at than the parent's repository.
    const otherRepoDir = path.join(backend.tmpDir, "repos", "e2e-cross-repo");
    fs.mkdirSync(otherRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: otherRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: otherRepoDir, env: gitEnv });
    const otherRepo = await apiClient.createRepository(seedData.workspaceId, otherRepoDir, "main", {
      name: "E2E Cross-Repo Target",
    });

    const subtaskTitle = "Cross-Repo Subtask E2E";
    const script = [
      'e2e:thinking("Creating cross-repo subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify repository override","repository_id":"${otherRepo.id}"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 2. Parent task lives in repoA (seedData.repositoryId). The agent script
    //    asks the MCP tool to create a subtask in repoB instead.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Cross-Repo Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    expect(parentTask.id).toBeTruthy();

    // 3. Navigate so the parent execution stays alive while the agent runs.
    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // 4. Poll for the subtask the agent is about to create.
    type TaskEntry = { id: string; title: string };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const allTasks = await apiClient.listTasks(seedData.workspaceId);
          subtask = allTasks.tasks.find((t: TaskEntry) => t.title === subtaskTitle);
          return subtask;
        },
        { timeout: 60_000, message: "Subtask should be created by the agent's MCP call" },
      )
      .toBeDefined();

    // 5. Verify the subtask landed on the OTHER repository, not the parent's.
    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}`);
    const subtaskData = (await subtaskRaw.json()) as TaskWithRepos;
    const repoIds = subtaskData.repositories?.map((r) => r.repository_id) ?? [];
    expect(
      repoIds,
      `subtask should target the override repository (${otherRepo.id}), not the parent's (${seedData.repositoryId}); got: ${JSON.stringify(repoIds)}`,
    ).toContain(otherRepo.id);
    expect(repoIds).not.toContain(seedData.repositoryId);
  });

  test("MCP-created same-repo subtask inherits parent's base_branch (stacked PR ergonomics)", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // 1. Set up a repo with a non-default branch the parent will be pinned to.
    //    A same-repo subtask should inherit that branch so its work stacks on
    //    the same starting point as the parent (sibling PRs off the same
    //    base, useful for stacked PR workflows).
    const repoDir = path.join(backend.tmpDir, "repos", "subtask-same-repo-inherit");
    fs.mkdirSync(repoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
    execSync("git branch feature/parent-branch", { cwd: repoDir, env: gitEnv });
    const repo = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "Subtask Same Repo Inherit",
    });

    const subtaskTitle = `Same-Repo Subtask ${Date.now()}`;
    const script = [
      'e2e:thinking("Creating subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify base_branch is inherited from parent in same repo"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 2. Create the parent pinned to a non-default branch.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Same-Repo Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: repo.id, base_branch: "feature/parent-branch" }],
      },
    );

    // Sanity: parent persisted the pin.
    const parentRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${parentTask.id}`);
    const parentData = (await parentRaw.json()) as TaskWithRepos;
    expect(parentData.repositories?.[0]?.base_branch).toBe("feature/parent-branch");

    // 3. Open the parent so the execution stays alive while the script runs.
    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // 4. Poll for the subtask.
    type TaskEntry = { id: string; title: string };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const allTasks = await apiClient.listTasks(seedData.workspaceId);
          subtask = allTasks.tasks.find((t: TaskEntry) => t.title === subtaskTitle);
          return subtask;
        },
        { timeout: 60_000, message: "Subtask should be created by the agent's MCP call" },
      )
      .toBeDefined();

    // 5. The same-repo subtask should inherit the parent's base_branch so the
    //    agent can stack work on top of the parent's PR. The worktree
    //    manager's fallback (covered separately in worktree unit tests) is
    //    the safety net if the inherited branch later goes stale on the
    //    remote — it is not the primary mechanism.
    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}`);
    const subtaskData = (await subtaskRaw.json()) as TaskWithRepos;
    expect(
      subtaskData.repositories?.[0]?.repository_id,
      "same-repo subtask should inherit the parent's repository_id",
    ).toBe(repo.id);
    expect(
      subtaskData.repositories?.[0]?.base_branch,
      `same-repo subtask base_branch should inherit parent's 'feature/parent-branch'; got: ${subtaskData.repositories?.[0]?.base_branch}`,
    ).toBe("feature/parent-branch");
  });

  test("MCP-created cross-repo subtask uses the new repo's default_branch", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // Parent lives in the seed repo on a non-default branch. The subtask
    // explicitly retargets a different repo via repository_id and supplies
    // no base_branch. The subtask must anchor to the new repo's
    // default_branch ("trunk") and never carry the parent's branch over.
    const otherRepoDir = path.join(backend.tmpDir, "repos", "subtask-cross-repo");
    fs.mkdirSync(otherRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b trunk", { cwd: otherRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: otherRepoDir, env: gitEnv });
    const otherRepo = await apiClient.createRepository(
      seedData.workspaceId,
      otherRepoDir,
      "trunk",
      {
        name: "Subtask Cross Repo",
      },
    );

    const subtaskTitle = `Cross-Repo Subtask ${Date.now()}`;
    const script = [
      'e2e:thinking("Creating cross-repo subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"${subtaskTitle}","description":"E2E cross-repo subtask","repository_id":"${otherRepo.id}"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Cross-Repo Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    type TaskEntry = { id: string; title: string };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const allTasks = await apiClient.listTasks(seedData.workspaceId);
          subtask = allTasks.tasks.find((t: TaskEntry) => t.title === subtaskTitle);
          return subtask;
        },
        {
          timeout: 60_000,
          message: "Cross-repo subtask should be created by the agent's MCP call",
        },
      )
      .toBeDefined();

    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}`);
    const subtaskData = (await subtaskRaw.json()) as TaskWithRepos;
    expect(subtaskData.repositories?.[0]?.repository_id).toBe(otherRepo.id);
    expect(
      subtaskData.repositories?.[0]?.base_branch,
      `cross-repo subtask base_branch should anchor to new repo's default 'trunk'; got: ${subtaskData.repositories?.[0]?.base_branch}`,
    ).toBe("trunk");
  });
});

/**
 * Verifies the New Subtask dialog mirrors the create-task dialog's repo/URL
 * features: GitHub URL paste, on-disk discovered repos, and multi-repo rows.
 * These tests exercise the shared `RepoChipsRow` + handlers/effects through
 * the subtask form-state shim in `new-subtask-form-state.ts`.
 */
test.describe("Subtask dialog feature parity", () => {
  test("user creates subtask via pasted GitHub URL", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // 1. Pre-seed a GitHub-backed repo + branches so the GitHub URL flow
    //    resolves to a known repository_id without performing a real clone.
    const repoDir = path.join(backend.tmpDir, "repos", "subtask-gh-url-target");
    fs.mkdirSync(repoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
    const ghRepo = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "subtask-owner/subtask-repo",
      provider: "github",
      provider_owner: "subtask-owner",
      provider_name: "subtask-repo",
    });
    await apiClient.mockGitHubAddBranches("subtask-owner", "subtask-repo", [{ name: "main" }]);

    // 2. Parent task on the seed repo.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Subtask GH URL Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 3. Open the subtask dialog and toggle to GitHub URL mode.
    await testPage.getByTestId("new-task-chevron").click();
    await testPage.getByTestId("new-subtask-button").click();
    const titleInput = testPage.getByTestId("subtask-title-input");
    await expect(titleInput).toBeVisible({ timeout: 5_000 });

    await testPage.getByTestId("toggle-github-url").click();
    const urlInput = testPage.getByTestId("github-url-input");
    await expect(urlInput).toBeVisible({ timeout: 5_000 });
    await urlInput.fill("https://github.com/subtask-owner/subtask-repo");

    // Wait for branch list to resolve so the payload carries `base_branch`.
    // The branch pill is disabled while branches are loading and re-enables
    // once the resolver returns options, so this is a positive signal that
    // the URL was parsed and branches were fetched.
    await expect(testPage.getByTestId("branch-chip-trigger")).toBeEnabled({ timeout: 10_000 });

    // 4. Submit.
    const subtaskTitle = `Subtask GH URL ${Date.now()}`;
    await titleInput.fill(subtaskTitle);
    await testPage.getByTestId("subtask-prompt-input").fill("/e2e:simple-message");
    await testPage.getByRole("button", { name: "Create Subtask" }).click();
    await expect(titleInput).not.toBeVisible({ timeout: 10_000 });

    // 5. Verify the subtask resolved to the GitHub-backed repo, not the parent's.
    type TaskEntry = { id: string; title: string };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const allTasks = await apiClient.listTasks(seedData.workspaceId);
          subtask = allTasks.tasks.find((t: TaskEntry) => t.title === subtaskTitle);
          return subtask;
        },
        { timeout: 30_000, message: "Subtask should be created from a GitHub URL" },
      )
      .toBeDefined();

    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}`);
    const subtaskData = (await subtaskRaw.json()) as TaskWithRepos;
    const repoIds = subtaskData.repositories?.map((r) => r.repository_id) ?? [];
    expect(
      repoIds,
      `subtask should resolve to the GitHub URL's repository (${ghRepo.id}); got: ${JSON.stringify(repoIds)}`,
    ).toContain(ghRepo.id);
    expect(repoIds).not.toContain(seedData.repositoryId);
  });

  test("subtask repo dropdown lists on-disk discovered repositories", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(60_000);

    // 1. Create a git repo on disk (under HOME=tmpDir, where the backend's
    //    discovery roots default scans) but do NOT register it as a workspace
    //    repo. The chip dropdown's "on disk" section is fed by the discovery
    //    endpoint and should pick this up.
    const discoveredName = "subtask-discovered-only";
    const discoveredDir = path.join(backend.tmpDir, "repos", discoveredName);
    fs.mkdirSync(discoveredDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: discoveredDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: discoveredDir, env: gitEnv });

    // 2. Parent task so we can open the subtask dialog from its session.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Subtask Discovered Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 3. Sanity-check via the API that backend discovery actually finds the
    //    on-disk directory. If this fails, the issue is the backend scan, not
    //    the dialog wiring — tells us where to look when debugging.
    const discoverRaw = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/repositories/discover`,
    );
    const discoverData = (await discoverRaw.json()) as {
      repositories: Array<{ path: string; name: string }>;
    };
    expect(
      discoverData.repositories.some((r) => r.path === discoveredDir),
      `backend discovery should include ${discoveredDir}; got: ${JSON.stringify(discoverData.repositories.map((r) => r.path))}`,
    ).toBe(true);

    // 4. Open the subtask dialog. The discover effect fires on mount via a
    //    Next.js server action, so we can't await a direct backend response
    //    here. Use poll-then-open: poll for the chip option to appear,
    //    reopening the popover each tick because cmdk's listbox snapshots
    //    options at open time and won't update if discovery resolves while
    //    the popover is already showing.
    await testPage.getByTestId("new-task-chevron").click();
    await testPage.getByTestId("new-subtask-button").click();
    await expect(testPage.getByTestId("subtask-title-input")).toBeVisible({ timeout: 5_000 });

    const discoveredOption = testPage
      .getByRole("option", { name: new RegExp(discoveredName, "i") })
      .first();
    const repoChip = testPage.getByTestId("repo-chip-trigger").first();
    await expect
      .poll(
        async () => {
          await repoChip.click();
          const visible = await discoveredOption.isVisible().catch(() => false);
          if (!visible) {
            // Close so the next tick reopens against fresh state.
            await testPage.keyboard.press("Escape");
          }
          return visible;
        },
        { timeout: 15_000, intervals: [500, 1000, 1500] },
      )
      .toBe(true);
    await expect(discoveredOption).toContainText("on disk");
  });

  test("user creates subtask spanning multiple repositories", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    // 1. Add a second workspace repository so the chip row can hold two rows.
    const otherRepoName = "Subtask MultiRepo Target";
    const otherRepoDir = path.join(backend.tmpDir, "repos", "subtask-multi-repo");
    fs.mkdirSync(otherRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: otherRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: otherRepoDir, env: gitEnv });
    const otherRepo = await apiClient.createRepository(seedData.workspaceId, otherRepoDir, "main", {
      name: otherRepoName,
    });

    // 2. Parent task on repo A so we can open the subtask dialog from there.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Subtask MultiRepo Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 3. Open the subtask dialog. The first chip is seeded with the parent's
    //    repo. Click the "+ add repository" button to append a second chip,
    //    then point that chip at repo B.
    await testPage.getByTestId("new-task-chevron").click();
    await testPage.getByTestId("new-subtask-button").click();
    const titleInput = testPage.getByTestId("subtask-title-input");
    await expect(titleInput).toBeVisible({ timeout: 5_000 });

    await testPage.getByTestId("add-repository").click();
    const chipTriggers = testPage.getByTestId("repo-chip-trigger");
    await expect(chipTriggers).toHaveCount(2, { timeout: 5_000 });
    await chipTriggers.nth(1).click();
    await testPage.getByRole("option", { name: new RegExp(otherRepoName) }).click();
    await expect(chipTriggers.nth(1)).toContainText(otherRepoName);

    // 4. Submit.
    const subtaskTitle = `Subtask MultiRepo ${Date.now()}`;
    await titleInput.fill(subtaskTitle);
    await testPage.getByTestId("subtask-prompt-input").fill("/e2e:simple-message");
    await testPage.getByRole("button", { name: "Create Subtask" }).click();
    await expect(titleInput).not.toBeVisible({ timeout: 10_000 });

    // 5. Verify the subtask carries BOTH repos.
    type TaskEntry = { id: string; title: string };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const allTasks = await apiClient.listTasks(seedData.workspaceId);
          subtask = allTasks.tasks.find((t: TaskEntry) => t.title === subtaskTitle);
          return subtask;
        },
        { timeout: 30_000, message: "Multi-repo subtask should be created from the dialog" },
      )
      .toBeDefined();

    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}`);
    const subtaskData = (await subtaskRaw.json()) as TaskWithRepos;
    const repoIds = subtaskData.repositories?.map((r) => r.repository_id) ?? [];
    expect(
      repoIds,
      `subtask should target both the parent's repo and the override repo; got: ${JSON.stringify(repoIds)}`,
    ).toEqual(expect.arrayContaining([seedData.repositoryId, otherRepo.id]));
    expect(repoIds).toHaveLength(2);
  });
});

/**
 * Verifies that when an agent creates a subtask via the MCP create_task tool
 * with parent_id="self", the subtask session inherits BOTH the parent session's
 * agent_profile_id AND executor_profile_id.
 */
test.describe("Subtask inheritance", () => {
  test("MCP-created subtask inherits agent profile and executor profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const subtaskTitle = "Inherited Profile Subtask E2E";

    const script = [
      'e2e:thinking("Creating subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"${subtaskTitle}","description":"E2E subtask: verify executor and agent profile inheritance"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 1. Create an executor + profile so executor_profile_id is non-empty on
    //    the parent session — this makes the inheritance assertion meaningful.
    const executor = await apiClient.createExecutor("E2E Inherit Executor", "local_pc");
    const executorProfile = await apiClient.createExecutorProfile(
      executor.id,
      "E2E Inherit Profile",
    );

    // 2. Create parent task via API with explicit executor_profile_id so the
    //    parent session records it. The mock agent runs the MCP script which
    //    creates the subtask with parent_id="self". A repository is required
    //    so the parent (non-ephemeral) task gets a real workspace path.
    const parentTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Executor Inherit Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        executor_profile_id: executorProfile.id,
        repository_ids: [seedData.repositoryId],
      },
    );
    expect(parentTask.id).toBeTruthy();

    // 3. Navigate to the parent task page so the execution stays alive.
    await testPage.goto(`/t/${parentTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // 4. Poll until the subtask appears. The agent starts asynchronously
    //    when created via the HTTP API, so the idle-input indicator may
    //    appear before the MCP script has finished.
    type TaskEntry = { id: string; title: string };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const allTasks = await apiClient.listTasks(seedData.workspaceId);
          subtask = allTasks.tasks.find((t: TaskEntry) => t.title === subtaskTitle);
          return subtask;
        },
        { timeout: 60_000, message: "Subtask should be created by the agent's MCP call" },
      )
      .toBeDefined();

    // 5. Confirm the parent session has both profiles set as expected.
    const parentRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${parentTask.id}/sessions`);
    const parentData = (await parentRaw.json()) as { sessions: SessionInfo[] };
    const parentSession = parentData.sessions[0];
    expect(
      parentSession?.agent_profile_id,
      `agent_profile_id: got ${parentSession?.agent_profile_id}, want ${seedData.agentProfileId}`,
    ).toBe(seedData.agentProfileId);
    expect(
      parentSession?.executor_profile_id,
      `executor_profile_id: got ${parentSession?.executor_profile_id}, want ${executorProfile.id}; full session: ${JSON.stringify(parentSession)}`,
    ).toBe(executorProfile.id);

    // 6. Verify subtask was auto-started: must have at least one session.
    //    Without the fix, autoStartTask finds no agent profile and skips launch →
    //    the subtask remains sessionless.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(subtask!.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Subtask should be auto-started with an inherited session" },
      )
      .toBeGreaterThanOrEqual(1);

    // 7. Wait for the subtask session to reach a terminal state.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(subtask!.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for subtask session to complete" },
      )
      .toBe(true);

    // 8. Verify the subtask session inherited BOTH profiles from the parent —
    //    proving executor/agent profile inheritance worked correctly.
    const subtaskRaw = await apiClient.rawRequest("GET", `/api/v1/tasks/${subtask!.id}/sessions`);
    const subtaskData = (await subtaskRaw.json()) as { sessions: SessionInfo[] };
    const subtaskSession = subtaskData.sessions[0];

    expect(subtaskSession?.agent_profile_id).toBe(parentSession?.agent_profile_id);
    expect(subtaskSession?.executor_profile_id).toBe(parentSession?.executor_profile_id);
    // Sanity: both are non-empty (so the assertions above are not vacuously true)
    expect(subtaskSession?.agent_profile_id).toBeTruthy();
    expect(subtaskSession?.executor_profile_id).toBeTruthy();
  });
});
