import { test, expect } from "../../fixtures/test-base";
import {
  readPreviousSidebarViewState,
  restoreSidebarViewState,
} from "./sidebar-running-first-activity-sort-helpers";
import { openPhoneList } from "./mobile-list-helpers";

const VIEW_STORAGE_KEY = "kandev.taskListing.view.v1";
const TASK_TITLE = "Mobile rich task";
const SEEDED_REPOSITORY_LABEL = "E2E Repo";

test.describe("Mobile task listing display preferences", () => {
  test("selects List from the drawer, restores Home, and navigates a rich row without overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, TASK_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      priority: "critical",
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      startedAt: new Date(Date.now() - 60_000).toISOString(),
    });
    await apiClient.setPrimarySession(sessionId);
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
    const previousViewState = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
    const viewId = `mobile-rich-row-${Date.now()}`;
    const view = {
      id: viewId,
      name: "Mobile rich row",
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "none",
      group_indent: true,
      collapsed_groups: [],
      task_row: {
        details_enabled: false,
        detail_order: ["relative_time", "repository", "pull_request_number"],
        visible_details: ["relative_time", "repository", "pull_request_number"],
        trailing: "git_changes",
      },
    };
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seedData.workspaceId,
        views: [...(previousViewState?.views ?? []), view],
        active_view_id: viewId,
        draft: null,
      },
    });

    try {
      await openPhoneList(testPage);
      const row = testPage.getByTestId("sidebar-task-item").filter({ hasText: TASK_TITLE });
      await expect(row).toBeVisible();
      await expect(row.getByTestId("sidebar-task-repository")).toHaveCount(0);

      await testPage.getByTestId("mobile-topbar-page-context").tap();
      const options = testPage.getByRole("dialog", { name: "View options" });
      const editor = options.getByTestId("mobile-list-view-editor");
      await expect(editor.getByTestId("sidebar-filter-active-view-name")).toHaveText(view.name);
      await editor.getByTestId("task-row-settings-toggle").tap();
      const detailsToggle = editor.getByTestId("task-row-details-toggle");
      await detailsToggle.tap();
      await expect(detailsToggle.getByRole("switch")).toHaveAttribute("aria-checked", "true");
      await editor.getByTestId("view-save-button").tap();
      await expect(editor.getByTestId("mobile-list-view-draft-footer")).toHaveCount(0);
      await expect
        .poll(async () => {
          const saved = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
          return saved?.views.find((candidate) => candidate.id === viewId)?.task_row;
        })
        .toMatchObject({ details_enabled: true });
      await testPage.keyboard.press("Escape");
      await expect(options).toHaveCount(0);

      await testPage.reload();
      const restoredRow = testPage.getByTestId("sidebar-task-item").filter({ hasText: TASK_TITLE });
      await expect(restoredRow).toBeVisible({ timeout: 15_000 });
      await expect(restoredRow.getByTestId("sidebar-task-repository")).toHaveText(
        SEEDED_REPOSITORY_LABEL,
      );
      await expect(restoredRow).toContainText("#43");
      await expect(
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).resolves.toBe(true);

      await restoredRow.getByTestId("task-item-title").tap();
      await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
    } finally {
      await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViewState);
    }
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
