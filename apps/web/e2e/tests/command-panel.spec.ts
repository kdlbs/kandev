import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";
import { waitForSessionDone } from "../helpers/session";
import { createSubmoduleReviewFixture } from "./review/submodule-review-helpers";

type CommandAliasExpectation = {
  query: string;
  label: string;
};

const HOME_COMMAND_ALIASES: CommandAliasExpectation[] = [
  { query: "general settings", label: "Go to Settings" },
  { query: "statistics", label: "Go to Stats" },
  { query: "issues", label: "Go to GitHub Dashboard" },
  { query: "pull request", label: "Go to GitHub Dashboard" },
  { query: "agent profiles", label: "Agents" },
  { query: "claude", label: "Agents" },
  { query: "execution environment", label: "Executors" },
  { query: "custom prompts", label: "Prompts" },
  { query: "prompt templates", label: "Prompts" },
  { query: "color theme", label: "Switch to Dark Mode" },
  { query: "new quick chat", label: "Quick Chat" },
  { query: "configure kandev", label: "Configuration Chat" },
  { query: "mcp configuration", label: "Configuration Chat" },
  { query: "recent tasks", label: "Open Recent Task Switcher" },
];

const SESSION_COMMAND_ALIASES: CommandAliasExpectation[] = [
  { query: "pull request", label: "Create PR" },
  { query: "push to remote", label: "Push" },
  { query: "upload changes", label: "Push" },
  { query: "download changes", label: "Pull" },
  { query: "start agent", label: "New Agent" },
  { query: "child task", label: "Create Subtask" },
  { query: "open browser preview", label: "Add Browser Panel" },
  { query: "command line", label: "Add Terminal Panel" },
  { query: "implementation plan", label: "Add Plan Panel" },
  { query: "source control", label: "Add Changes Panel" },
];

/**
 * Helper: open the command panel via Cmd/Ctrl+K.
 */
async function openCommandPanel(page: import("@playwright/test").Page) {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+k`);
}

/**
 * Helper: open the file search panel via Cmd/Ctrl+Shift+K.
 */
async function openFileSearch(page: import("@playwright/test").Page) {
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+k`);
}

/** The command dialog (Radix Dialog with role="dialog"). */
function commandDialog(page: import("@playwright/test").Page) {
  return page.getByRole("dialog");
}

function commandOption(page: import("@playwright/test").Page, label: string) {
  return commandDialog(page)
    .getByRole("option")
    .filter({ has: page.getByText(label, { exact: true }) });
}

async function expectCommandAliases(
  page: import("@playwright/test").Page,
  aliases: CommandAliasExpectation[],
) {
  const dialog = commandDialog(page);
  const input = dialog.getByRole("combobox");
  for (const { query, label } of aliases) {
    await input.fill(query);
    await expect(commandOption(page, label)).toBeVisible();
  }
}

test.describe("Command Panel", () => {
  test("Cmd+K opens command panel and shows commands (not files)", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Should show command groups like Navigation, Settings
    await expect(dialog.getByText("Navigation")).toBeVisible();
    await expect(dialog.getByText("Go to Home")).toBeVisible();

    // Type something that matches a navigation command
    await dialog.locator("input").fill("Settings");
    // Should find settings commands
    await expect(dialog.getByText("Go to Settings")).toBeVisible({ timeout: 5_000 });

    // Should NOT show a "Files" group — file search is now separate
    await expect(dialog.getByText("Files").first()).not.toBeVisible({ timeout: 2_000 });
  });

  test("common aliases find home commands", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await expectCommandAliases(testPage, HOME_COMMAND_ALIASES);

    await dialog.getByRole("combobox").fill("Environments Settings");
    await expect(commandOption(testPage, "Environments Settings")).toHaveCount(0);
  });

  test("settings discovery stays quiet until typing and lands on an exact control", async ({
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await expect(commandOption(testPage, "Terminal Font Size")).toHaveCount(0);

    const input = dialog.getByRole("combobox");
    await input.fill("terminal font size");
    await expect(dialog.locator("[cmdk-group-heading]")).toHaveText(["Settings"]);
    const option = commandOption(testPage, "Terminal Font Size");
    await expect(option).toBeVisible();
    await expect(option.getByText("Settings › Terminal & Editors", { exact: true })).toBeVisible();
    await expect(option).toHaveAttribute("data-selected", "true");
    await input.press("Enter");

    await expect(dialog).not.toBeVisible();
    await expect(testPage).toHaveURL(
      /\/settings\/preferences\/terminal-editors#setting-terminal-font-size$/,
    );
    const target = testPage.locator('[data-settings-target="setting-terminal-font-size"]');
    await expect(target).toHaveAttribute("data-settings-target-highlight", "true");
    await expect(testPage.getByTestId("terminal-font-size-input")).toBeFocused();
  });

  test("common aliases find worktree session commands", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Command Alias E2E",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("Command alias task did not start a session");
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "command alias task session did not become ready",
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status, {
        timeout: 30_000,
        message: "command alias task environment did not become ready",
      })
      .toBe("ready");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.showSessionContext();

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const input = dialog.getByRole("combobox");
    await input.fill("plan");
    await expect(dialog.getByRole("option").filter({ hasText: "Plan" })).toHaveCount(1);
    await expect(commandOption(testPage, "Add Plan Panel")).toBeVisible();

    await input.fill("pull request");
    await expect(commandOption(testPage, "Create PR")).toHaveAttribute("data-selected", "true");

    await input.fill("pr");
    await expect(commandOption(testPage, "Create PR")).toHaveAttribute("data-selected", "true");

    await expectCommandAliases(testPage, SESSION_COMMAND_ALIASES);

    await input.fill("pr");
    await expect(commandOption(testPage, "Create PR")).toHaveAttribute("data-selected", "true");
    await input.press("Enter");
    await expect(testPage.getByRole("heading", { name: /Create Pull Request/ })).toBeVisible();
  });

  test("Cmd+Shift+K opens file search inside a task workbench", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "File Search Shortcut E2E",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();

    await openFileSearch(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await expect(dialog.getByRole("tab", { name: "Files" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    // Should show empty state for file search
    await expect(dialog.getByText("Type to search files...")).toBeVisible();
  });

  test("Cmd+Shift+K finds root and submodule files", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const fixture = await createSubmoduleReviewFixture(
      apiClient,
      seedData,
      backend.tmpDir,
      "Submodule file search E2E",
    );

    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 45_000 });
      await fixture.waitForWorktree(apiClient);

      await openFileSearch(testPage);
      const dialog = commandDialog(testPage);
      await expect(dialog).toBeVisible({ timeout: 5_000 });
      await dialog.getByRole("combobox").fill("README.md");

      const groups = dialog.getByTestId("file-search-repo-group");
      await expect(groups).toHaveCount(3, { timeout: 45_000 });
      await expect(
        dialog.locator('[data-testid="file-search-repo-group"][data-repository=""]'),
      ).toBeVisible();
      await expect(
        dialog.locator('[data-testid="file-search-repo-group"][data-repository="vendor/outer"]'),
      ).toBeVisible();
      await expect(
        dialog.locator(
          '[data-testid="file-search-repo-group"][data-repository="vendor/outer/vendor/inner"]',
        ),
      ).toBeVisible();
    } finally {
      fixture.cleanup();
    }
  });

  test("Cmd+K inline task search shows matching tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create a task to search for
    await apiClient.createTask(seedData.workspaceId, "Searchable E2E Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Wait for the task to appear on the kanban board first
    await expect(kanban.taskCardByTitle("Searchable E2E Task")).toBeVisible({ timeout: 10_000 });

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Type the task name — task search requires ≥2 characters
    await dialog.locator("input").fill("Searchable");

    // Should show the task in a "Tasks" group (inline search, debounced).
    // Scope the heading lookup: "Tasks" is also the label of the Tasks tab.
    await expect(dialog.getByTestId("command-panel-task-preview")).toBeVisible({
      timeout: 10_000,
    });
    await expect(dialog.getByText("Searchable E2E Task")).toBeVisible({ timeout: 5_000 });
  });

  test("task activity icon distinguishes running and idle search results", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const runningTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Running Activity Icon E2E",
      seedData.agentProfileId,
      {
        description: "/e2e:steer-fold-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.createTask(seedData.workspaceId, "Idle Activity Icon E2E", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(runningTask.title)).toBeVisible({ timeout: 10_000 });

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    const input = dialog.getByRole("combobox");
    await input.fill("Running Activity Icon");
    const runningOption = dialog.getByRole("option").filter({ hasText: runningTask.title });
    await expect(runningOption).toBeVisible({ timeout: 10_000 });
    await expect(runningOption.getByTestId("task-state-running")).toBeVisible();
    await expect(runningOption.getByRole("img", { name: "In progress" })).toBeVisible();

    await input.fill("Idle Activity Icon");
    const idleOption = dialog.getByRole("option").filter({ hasText: "Idle Activity Icon E2E" });
    await expect(idleOption).toBeVisible({ timeout: 10_000 });
    await expect(idleOption.getByTestId("task-state-backlog")).toBeVisible();
    await expect(idleOption.getByRole("img", { name: "Created" })).toBeVisible();
  });

  // @covers AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.8
  test("inline task search shows workflow step badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Badged Task E2E", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Badged Task E2E")).toBeVisible({ timeout: 10_000 });

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await dialog.locator("input").fill("Badged Task");

    // The task should appear with a workflow step badge
    const taskRow = dialog.getByText("Badged Task E2E");
    await expect(taskRow).toBeVisible({ timeout: 10_000 });

    // The step badge should be present in the task result row
    const startStep = seedData.steps.find((s) => s.id === seedData.startStepId)!;
    const taskOption = dialog.getByRole("option", { name: /Badged Task E2E/ });
    await expect(taskOption).toBeVisible({ timeout: 5_000 });
    await expect(taskOption.getByText(startStep.name)).toBeVisible({ timeout: 5_000 });

    const reviewStep = seedData.steps.find((s) => s.name === "Review")!;
    await apiClient.moveTask(task.id, seedData.workflowId, reviewStep.id);
    await expect(taskOption.getByText(reviewStep.name)).toBeVisible({ timeout: 10_000 });
    await expect(taskOption.getByText(startStep.name)).not.toBeVisible({ timeout: 5_000 });
  });

  test("inline task search selects the first matching task after loading", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Palette Selection Alpha", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Palette Selection Beta", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Palette Selection Alpha")).toBeVisible({
      timeout: 10_000,
    });
    await expect(kanban.taskCardByTitle("Palette Selection Beta")).toBeVisible({
      timeout: 10_000,
    });

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const input = dialog.getByRole("combobox");
    await input.fill("Palette Selection");

    const taskOptions = dialog.getByRole("option").filter({ hasText: /Palette Selection/ });
    await expect(taskOptions.first()).toBeVisible({ timeout: 10_000 });
    await expect(taskOptions.first()).toHaveAttribute("data-selected", "true", {
      timeout: 5_000,
    });
  });

  test("Escape closes the command panel", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible({ timeout: 3_000 });
  });

  test("Backspace keeps the empty file-search scope active", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "File Search Backspace E2E",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();

    // Open file search mode
    await openFileSearch(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByText("Type to search files...")).toBeVisible();

    // Files is a peer scope, not a nested command mode, so an empty Backspace
    // should remain in place instead of acting as hidden "back" navigation.
    const input = dialog.getByRole("combobox");
    await input.focus();
    await input.press("Backspace");

    await expect(dialog.getByRole("tab", { name: "Files" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(dialog.getByText("Type to search files...")).toBeVisible();
    await expect(dialog.getByText("Navigation")).toHaveCount(0);
  });

  test("Cmd+K toggles the panel open and closed", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Open
    await openCommandPanel(testPage);
    const dialog = commandDialog(testPage);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Close by pressing Cmd+K again
    await openCommandPanel(testPage);
    await expect(dialog).not.toBeVisible({ timeout: 3_000 });
  });
});
