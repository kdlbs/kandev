import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { waitForHttp } from "../../helpers/causal-waits";

const VIEW_STORAGE_KEY = "kandev.taskListing.view.v1";
const TASK_TITLE = "Mobile rich task";
const TASK_DESCRIPTION = "Mobile details must stay readable without page overflow.";
const SEEDED_REPOSITORY_LABEL = "E2E Repo";

test.describe("Mobile task listing display preferences", () => {
  test("selects List from the drawer, restores Home, and navigates a rich row without overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, TASK_TITLE, {
      description: TASK_DESCRIPTION,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await expect
      .poll(
        async () => {
          const persistedTask = await apiClient.getTask(task.id);
          return persistedTask.repositories?.some(
            (repository) => repository.repository_id === seedData.repositoryId,
          );
        },
        {
          message: "task repository association was not persisted before listing",
          timeout: 15_000,
        },
      )
      .toBe(true);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "kandev",
      repo: "e2e-repo",
      pr_number: 43,
      pr_url: "https://github.com/kandev/e2e-repo/pull/43",
      pr_title: "Mobile details",
      head_branch: "mobile-details",
      base_branch: "main",
      author_login: "e2e",
      state: "open",
      checks_state: "success",
      mergeable_state: "clean",
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileMenuButton.click();
    const menu = testPage.getByRole("dialog", { name: "Menu" });
    await menu.getByRole("radio", { name: "List", exact: true }).click();
    await expect(testPage).toHaveURL(/\/tasks/);
    await expect(testPage.getByTestId("tasks-list")).toBeVisible();

    const taskListLoaded = waitForHttp(testPage, "GET", /\/api\/v1\/workspaces\/[^/]+\/tasks$/, {
      predicate: async (response) => {
        if (response.status() !== 200) return false;
        const body = (await response.json()) as {
          tasks?: Array<{
            id?: string;
            repositories?: Array<{ repository_id?: string }>;
          }>;
        };
        return (
          body.tasks?.some(
            (candidate) =>
              candidate.id === task.id &&
              candidate.repositories?.some(
                (repository) => repository.repository_id === seedData.repositoryId,
              ),
          ) ?? false
        );
      },
    });
    await testPage.goto("/");
    await taskListLoaded;
    await expect(testPage).toHaveURL(/\/tasks/);
    await testPage.getByRole("button", { name: "Open menu" }).click();
    const tasksMenu = testPage.getByRole("dialog", { name: "Menu" });
    await tasksMenu.getByText("Show task details", { exact: true }).click();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.tasks_list_show_details, {
        message: "task detail preference was not persisted",
      })
      .toBe(true);
    await testPage.keyboard.press("Escape");
    await expect(tasksMenu).toHaveCount(0);

    const row = testPage.getByTestId("tasks-list-row").filter({ hasText: TASK_TITLE });
    await expect(row).toContainText(SEEDED_REPOSITORY_LABEL);
    await expect(row).toContainText(TASK_DESCRIPTION);
    await expect(row.getByTestId(`pr-task-icon-${task.id}`)).toBeVisible();
    await expect(
      testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).resolves.toBe(true);

    await row.getByTestId("tasks-list-row-content").click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
  });

  test("uses Kanban on a phone without overwriting a saved Pipeline preference", async ({
    testPage,
  }) => {
    await testPage.addInitScript(
      ([key, value]) => window.localStorage.setItem(key, value),
      [VIEW_STORAGE_KEY, JSON.stringify("pipeline")],
    );
    await testPage.goto("/");
    await expect(testPage.getByTestId("mobile-kanban-layout")).toBeVisible();
    await expect(testPage.getByTestId("app-sidebar-layout")).toBeHidden();
    await expect(
      testPage.evaluate((key) => window.localStorage.getItem(key), VIEW_STORAGE_KEY),
    ).resolves.toBe(JSON.stringify("pipeline"));
  });
});
