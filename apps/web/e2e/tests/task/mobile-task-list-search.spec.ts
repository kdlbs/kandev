import { test, expect } from "../../fixtures/test-base";
import { repositorySlug } from "../../../lib/repository-slug";
import { openPhoneList, SIDEBAR_QUERY_PATH, waitForSidebarQuery } from "./mobile-list-helpers";
import {
  readPreviousSidebarViewState,
  restoreSidebarViewState,
} from "./sidebar-running-first-activity-sort-helpers";

test.use({ trace: "retain-on-failure" });

test.describe("Mobile task list search", () => {
  test("search icon filters and clears results without writing a saved view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const token = `Phone list search ${Date.now()}`;
    const matching = await apiClient.createTask(seedData.workspaceId, `${token} Alpha`, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const other = await apiClient.createTask(seedData.workspaceId, `${token} Beta`, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const savedView = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);

    try {
      await openPhoneList(testPage);
      const list = testPage.getByTestId("mobile-tasks-list-scroll");
      const matchingRow = list.getByTestId("sidebar-task-item").filter({ hasText: matching.title });
      const otherRow = list.getByTestId("sidebar-task-item").filter({ hasText: other.title });
      await expect(matchingRow).toBeVisible();
      await expect(otherRow).toBeVisible();
      await expect(testPage.getByTestId("mobile-list-search-row")).toHaveCount(0);

      const toggle = testPage.getByTestId("mobile-list-search-toggle");
      await toggle.tap();
      const input = testPage.getByTestId("mobile-list-search-input");
      await expect(input).toBeFocused();
      const searched = waitForSidebarQuery(testPage, (query) => Boolean(query.query));
      await input.fill(`${token} Alpha`);
      const searchedResponse = await searched;
      expect(searchedResponse.request().postDataJSON()).toMatchObject({ query: `${token} Alpha` });
      await expect(matchingRow).toBeVisible();
      await expect(otherRow).toHaveCount(0);
      expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(
        savedView,
      );

      await testPage.getByRole("button", { name: "Clear search", exact: true }).tap();
      await expect(input).toHaveValue("");
      await expect(matchingRow).toBeVisible();
      await expect(otherRow).toBeVisible();
      expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(
        savedView,
      );
    } finally {
      await restoreSidebarViewState(apiClient, seedData.workspaceId, savedView);
    }
  });

  test("keeps the settled clear-search target inside the input and clears on tap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const title = `Phone settled clear ${Date.now()}`;
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await openPhoneList(testPage);
    await testPage.getByTestId("mobile-list-search-toggle").tap();
    const input = testPage.getByTestId("mobile-list-search-input");
    const searched = waitForSidebarQuery(testPage, (query) => query.query === title);
    await input.fill(title);
    await searched;
    const row = testPage
      .getByTestId("mobile-tasks-list-scroll")
      .getByTestId("sidebar-task-item")
      .filter({ hasText: task.title });
    await expect(row).toBeVisible();

    const clear = testPage.getByRole("button", { name: "Clear search", exact: true });
    await expect(clear).toBeVisible();
    const [inputBox, clearBox] = await Promise.all([input.boundingBox(), clear.boundingBox()]);
    expect(inputBox).not.toBeNull();
    expect(clearBox).not.toBeNull();
    expect(clearBox!.x).toBeGreaterThanOrEqual(inputBox!.x);
    expect(clearBox!.x + clearBox!.width).toBeLessThanOrEqual(inputBox!.x + inputBox!.width);
    expect(clearBox!.y).toBeGreaterThanOrEqual(inputBox!.y);
    expect(clearBox!.y + clearBox!.height).toBeLessThanOrEqual(inputBox!.y + inputBox!.height);

    await clear.tap();
    await expect(input).toHaveValue("");
    await expect(row).toBeVisible();
  });

  test("closing search clears its refinement and returns focus to the toggle", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const title = `Phone close search ${Date.now()}`;
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await openPhoneList(testPage);
    const toggle = testPage.getByTestId("mobile-list-search-toggle");
    await toggle.tap();
    const input = testPage.getByTestId("mobile-list-search-input");
    const searched = waitForSidebarQuery(testPage, (query) => query.query === title);
    await input.fill(title);
    await searched;
    await toggle.tap();
    await expect(testPage.getByTestId("mobile-list-search-row")).toHaveCount(0);
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toBeFocused();
    await expect(
      testPage
        .getByTestId("mobile-tasks-list-scroll")
        .getByTestId("sidebar-task-item")
        .filter({ hasText: task.title }),
    ).toBeVisible();
  });

  test("clear cancels queued phone search and keeps the input visible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Queued phone task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await openPhoneList(testPage);
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: task.title }),
    ).toBeVisible();
    await testPage.getByTestId("mobile-list-search-toggle").tap();
    const bar = testPage.getByTestId("mobile-list-search-row");
    const input = testPage.getByTestId("mobile-list-search-input");
    await expect(input).toBeFocused();
    await testPage.clock.install();
    await testPage.clock.pauseAt(new Date());
    const queries: string[] = [];
    testPage.on("request", (request) => {
      if (!SIDEBAR_QUERY_PATH.test(new URL(request.url()).pathname)) return;
      try {
        const query = request.postDataJSON() as { query?: string };
        if (query.query) queries.push(query.query);
      } catch {
        // Requests without a JSON query body do not carry a search refinement.
      }
    });
    await input.fill("Cancelled phone input");
    await testPage.getByRole("button", { name: "Clear search", exact: true }).tap();
    await testPage.clock.runFor(32);
    await expect(bar).toBeVisible();
    await testPage.clock.runFor(1000);
    expect(queries).not.toContain("Cancelled phone input");
    await testPage.clock.resume();
    await expect(input).toHaveValue("");
    await input.tap();
    await expect(input).toBeFocused();
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: task.title }),
    ).toBeVisible();
  });

  test("latest search resets pagination and retains temporary filters", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const token = `Phone page search ${Date.now()}`;
    const previousViews = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
    const viewId = `${token}-view`;
    try {
      for (let start = 0; start < 110; start += 10) {
        await Promise.all(
          Array.from({ length: 10 }, (_, index) =>
            apiClient.createTask(
              seedData.workspaceId,
              `${token} ${String(start + index).padStart(3, "0")}`,
              {
                workflow_id: seedData.workflowId,
                workflow_step_id: seedData.startStepId,
                repository_ids: [seedData.repositoryId],
              },
            ),
          ),
        );
      }
      await apiClient.saveUserSettings({
        sidebar_view_state: {
          workspace_id: seedData.workspaceId,
          views: [
            {
              id: "view-all-tasks",
              name: "All tasks",
              filters: [],
              sort: { key: "state", direction: "asc" },
              group: "none",
              group_indent: true,
              collapsed_groups: [],
            },
            {
              id: viewId,
              name: `${token} view`,
              filters: [
                { id: `${viewId}-filter`, dimension: "titleMatch", op: "matches", value: token },
              ],
              sort: { key: "title", direction: "asc" },
              group: "none",
              group_indent: true,
              collapsed_groups: [],
            },
          ],
          active_view_id: viewId,
          draft: null,
        },
      });
      const savedViewState = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);
      await openPhoneList(testPage);
      const activeView = testPage
        .getByTestId("sidebar-view-chip-row")
        .getByTestId("sidebar-view-chip")
        .filter({ hasText: `${token} view` });
      await expect(activeView).toHaveAttribute("data-active", "true");
      const list = testPage.getByTestId("mobile-tasks-list-scroll");
      const pagination = list.getByTestId("sidebar-page-controls");
      await expect(pagination).toContainText("Page 1 of 2");
      const nextPage = pagination.getByRole("button", { name: "Next" });
      await expect(nextPage).toBeEnabled();
      const nextPageBounds = await nextPage.boundingBox();
      expect(nextPageBounds?.height).toBeGreaterThanOrEqual(44);
      await nextPage.click();
      await expect(pagination).toContainText("Page 2 of 2");

      await testPage.getByTestId("mobile-list-search-toggle").tap();
      await testPage.getByTestId("mobile-list-result-filters").tap();
      const drawer = testPage.getByTestId("mobile-list-result-filters-drawer");
      const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
      const workflowName = workflows.find((workflow) => workflow.id === seedData.workflowId)!.name;
      const { repositories } = await apiClient.listRepositories(seedData.workspaceId);
      const repositoryName = repositorySlug(
        repositories.find((repo) => repo.id === seedData.repositoryId)!,
      );
      await drawer.getByTestId("mobile-list-filter-workflow").tap();
      await testPage.getByRole("option", { name: workflowName, exact: true }).tap();
      await drawer.getByTestId("mobile-list-filter-repository").tap();
      await testPage.getByRole("option", { name: repositoryName, exact: true }).tap();
      await drawer.getByTestId("mobile-list-filters-apply").tap();
      const appliedFilters = testPage.getByTestId("mobile-list-temporary-filter-summary");
      await expect(appliedFilters).toContainText(workflowName);
      await expect(appliedFilters).toContainText(repositoryName);
      await expect(pagination).toContainText("Page 1 of 2");
      expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(
        savedViewState,
      );

      const input = testPage.getByTestId("mobile-list-search-input");
      const searched = waitForSidebarQuery(testPage, (query) => Boolean(query.query));
      await input.fill(`${token} 001`);
      const request = await searched;
      const body = request.request().postDataJSON() as {
        page: number;
        sort: { key: string; direction: string };
        filters: Array<{ dimension: string; value: unknown }>;
      };
      expect(request.request().postDataJSON()).toMatchObject({ query: `${token} 001` });
      expect(body.page).toBe(1);
      expect(body.sort).toMatchObject({ key: "title", direction: "asc" });
      expect(body.filters.some((filter) => filter.dimension === "workflow")).toBe(true);
      expect(body.filters.some((filter) => filter.dimension === "repository")).toBe(true);
      await expect(pagination).toHaveCount(0);
      await expect(list.getByTestId("sidebar-task-item")).toHaveCount(1);
      await expect(list.getByTestId("sidebar-task-item")).toContainText(`${token} 001`);
      expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(
        savedViewState,
      );
    } finally {
      await restoreSidebarViewState(apiClient, seedData.workspaceId, previousViews);
    }
  });
});
