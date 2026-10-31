import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { waitForHttp } from "../../helpers/causal-waits";
import { selectTaskListFilter } from "./task-list-archive-helpers";

test.use({ trace: "retain-on-failure" });

const listPath = /\/api\/v1\/workspaces\/[^/]+\/tasks$/;

test.describe("Mobile task list search", () => {
  test("visible search filters and clears the list", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "List Alpha Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "List Beta Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/tasks");
    await expect(testPage.getByTestId("mobile-topbar-page-context")).toBeVisible();

    const taskList = testPage.getByTestId("tasks-list");
    const searchBar = testPage.getByTestId("tasks-list-filters");

    await expect(taskList.getByText("List Alpha Task")).toBeVisible();
    await expect(taskList.getByText("List Beta Task")).toBeVisible();
    await expect(searchBar).toBeVisible();

    const searched = waitForHttp(testPage, "GET", listPath, {
      predicate: (r) => new URL(r.url()).searchParams.get("query") === "Alpha",
    });
    await searchBar.getByPlaceholder("Search tasks...").fill("Alpha");
    await searched;
    await expect(taskList.getByText("List Alpha Task")).toBeVisible();
    await expect(taskList.getByText("List Beta Task")).not.toBeVisible();

    const cleared = waitForHttp(testPage, "GET", listPath, {
      predicate: (r) => !new URL(r.url()).searchParams.has("query"),
    });
    await testPage.getByRole("button", { name: "Clear search", exact: true }).tap();
    await cleared;
    await expect(searchBar).toBeVisible();
    await expect(taskList.getByText("List Alpha Task")).toBeVisible();
    await expect(taskList.getByText("List Beta Task")).toBeVisible();
  });

  test("display menu configures the compact list", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Alpha mobile sort", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Zulu mobile sort", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const archivedTask = await apiClient.createTask(
      seedData.workspaceId,
      "Archived mobile sort task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    await apiClient.archiveTask(archivedTask.id);

    await testPage.goto("/tasks?group=none");
    await expect(testPage.getByTestId("mobile-topbar-page-context")).toBeVisible();
    const searched = waitForHttp(testPage, "GET", listPath, {
      predicate: (r) => new URL(r.url()).searchParams.get("query") === "mobile sort",
    });
    await testPage
      .getByTestId("tasks-list-filters")
      .getByPlaceholder("Search tasks...")
      .fill("mobile sort");
    await searched;

    await expect(testPage.getByTestId("tasks-list-sort")).not.toBeVisible();
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    const menu = testPage.getByRole("dialog", { name: "View options" });
    await waitForFiniteAnimations(menu);
    await menu.getByTestId("mobile-tasks-list-sort").tap();
    const listbox = testPage.getByRole("listbox");
    await expect(listbox).toBeVisible();
    await waitForFiniteAnimations(listbox);
    await listbox.getByRole("option", { name: "Title Z-A" }).tap();
    await expect(listbox).not.toBeVisible();

    await expect(testPage).toHaveURL((url) => url.searchParams.get("sort") === "title_desc");
    await expect
      .poll(() => testPage.getByTestId("tasks-list-row-title").allTextContents())
      .toEqual(["Zulu mobile sort", "Alpha mobile sort"]);

    await menu.getByTestId("mobile-tasks-list-group").tap();
    await expect(listbox).toBeVisible();
    await waitForFiniteAnimations(listbox);
    await listbox.getByRole("option", { name: "Workflow step" }).tap();
    await expect(listbox).not.toBeVisible();
    await expect(testPage).toHaveURL((url) => url.searchParams.get("group") === "workflow_step");

    await expect(
      testPage.getByTestId("tasks-list").getByText("Archived mobile sort task"),
    ).toHaveCount(0);
    await testPage.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await testPage.getByTestId("tasks-list-archive-all").tap();
    await expect(
      testPage.getByTestId("tasks-list").getByText("Archived mobile sort task"),
    ).toBeVisible();
    await testPage.reload();
    await expect(testPage.getByTestId("tasks-list-section")).toHaveCount(1);
    const { steps } = await apiClient.listWorkflowSteps(seedData.workflowId);
    const startStep = steps.find((step) => step.id === seedData.startStepId)!;
    await expect(testPage.getByTestId("tasks-list-section")).toContainText(startStep.name);
    await prCapture.screenshot("mobile-workflow-step-groups", {
      caption: "Phone task list uses configured workflow step headings.",
    });
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    await expect(menu.getByTestId("mobile-tasks-list-group")).toContainText("Workflow step");
    const groupBox = await menu.getByTestId("mobile-tasks-list-group").boundingBox();
    expect(groupBox?.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await menu.getByTestId("mobile-tasks-list-group").scrollIntoViewIfNeeded();
    await menu.evaluate(async (element) => {
      await Promise.all(
        element
          .getAnimations({ subtree: true })
          .filter((animation) => animation.effect?.getTiming().iterations !== Infinity)
          .map((animation) => animation.finished),
      );
    });
    await prCapture.screenshot("mobile-workflow-step-options", {
      caption: "Phone View options offers the same Workflow step grouping.",
    });
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.14
  test("clear cancels queued phone search and keeps the input visible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Queued phone task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto("/tasks?group=none");
    await expect(testPage.getByTestId("tasks-list-row-title")).toContainText("Queued phone task");
    const bar = testPage.getByTestId("tasks-list-filters");
    const input = testPage.getByTestId("tasks-list-search");
    await input.tap();
    await expect(input).toBeFocused();
    const clockStart = new Date();
    await testPage.clock.install({ time: clockStart });
    await testPage.clock.pauseAt(new Date(clockStart.getTime() + 60_000));
    const queries: string[] = [];
    testPage.on("request", (request) => {
      const url = new URL(request.url());
      if (listPath.test(url.pathname)) queries.push(url.searchParams.get("query") ?? "");
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
      testPage.getByTestId("tasks-list").getByText("Queued phone task", { exact: true }),
    ).toBeVisible();
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.13
  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.15
  test("phone latest search resets pagination and retains its filters", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { settings } = await apiClient.getUserSettings();
    try {
      for (let i = 0; i < 26; i++) {
        await apiClient.createTask(
          seedData.workspaceId,
          `Phone search ${String(i).padStart(2, "0")}`,
          {
            workflow_id: seedData.workflowId,
            workflow_step_id: seedData.startStepId,
            repository_ids: [seedData.repositoryId],
          },
        );
      }
      await testPage.goto("/tasks?sort=title_asc&group=none");
      await testPage.getByTestId("tasks-list-filters-trigger").tap();
      const drawer = testPage.getByTestId("tasks-list-filters-drawer");
      const workflowName = (await apiClient.listWorkflows(seedData.workspaceId)).workflows.find(
        (workflow) => workflow.id === seedData.workflowId,
      )!.name;
      await selectTaskListFilter(testPage, "workflow", workflowName, true);
      await selectTaskListFilter(testPage, "repository", "E2E Repo", true);
      await drawer.getByRole("button", { name: "Done", exact: true }).tap();
      await testPage.getByTestId("tasks-list-archive-all").tap();
      const input = testPage.getByTestId("tasks-list-search");
      const loaded = waitForHttp(testPage, "GET", listPath, {
        predicate: (r) => new URL(r.url()).searchParams.get("query") === "Phone search",
      });
      await input.fill("Phone search");
      await loaded;
      const pagination = testPage.getByTestId("tasks-pagination");
      await expect(pagination).toContainText("1 to 25 of 26");
      const next = waitForHttp(testPage, "GET", listPath, {
        predicate: (r) => new URL(r.url()).searchParams.get("page") === "2",
      });
      await pagination.getByRole("button", { name: "Go to next page" }).tap();
      await next;
      await expect(pagination).toContainText("26 to 26 of 26");
      const searched = waitForHttp(testPage, "GET", listPath, {
        predicate: (r) => {
          const params = new URL(r.url()).searchParams;
          return params.get("query") === "Phone search 01" && params.get("page") === "1";
        },
      });
      await input.fill("Phone search 00");
      await input.fill("Phone search 01");
      const params = new URL((await searched).url()).searchParams;
      expect(params.get("workflow_id")).toBe(seedData.workflowId);
      expect(params.get("repository_id")).toBe(seedData.repositoryId);
      expect(params.get("sort")).toBe("title_asc");
      expect(params.get("include_archived")).toBe("true");
      await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText(["Phone search 01"]);
      await expect(pagination.getByRole("button", { name: "1", exact: true })).toHaveAttribute(
        "aria-current",
        "page",
      );
      const cleared = waitForHttp(testPage, "GET", listPath, {
        predicate: (r) => !new URL(r.url()).searchParams.has("query"),
      });
      await testPage.getByRole("button", { name: "Clear search", exact: true }).tap();
      await cleared;
      await expect(input).toHaveValue("");
      await expect(pagination).toContainText("1 to 25 of");
    } finally {
      await apiClient.saveUserSettings({
        repository_ids: settings.repository_ids as string[],
        tasks_list_sort: settings.tasks_list_sort as string,
      });
    }
  });
});
