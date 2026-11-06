import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { selectTaskListFilter, waitForTaskList } from "./task-list-archive-helpers";

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-003.2 AC-TASKS-LIST-ARCHIVE-FILTERS-003.3 AC-TASKS-LIST-ARCHIVE-FILTERS-003.5
test("filters the phone archive in an inset drawer and clears without losing archive mode", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const baseline = (await apiClient.getUserSettings()).settings;
  try {
    const other = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Phone other workflow",
      "simple",
    );
    const { steps } = await apiClient.listWorkflowSteps(other.id);
    for (const [title, workflow, step] of [
      ["Phone archive primary", seedData.workflowId, seedData.startStepId],
      ["Phone archive other", other.id, steps[0].id],
    ]) {
      const task = await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: workflow,
        workflow_step_id: step,
      });
      await apiClient.archiveTask(task.id);
    }
    await testPage.goto(
      `/tasks?workspace=${seedData.workspaceId}&archive=archived&workflow=all&repository=all&group=none`,
    );
    await expect(testPage.getByTestId("tasks-list-row-title")).toHaveCount(2);
    const trigger = testPage.getByTestId("tasks-list-filters-trigger");
    expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await trigger.tap();
    const drawer = testPage.getByTestId("tasks-list-filters-drawer");
    await expect(drawer).toBeVisible();
    await waitForFiniteAnimations(drawer);
    const scrollBody = drawer.locator(".overflow-y-auto");
    await expect(scrollBody).toHaveCount(1);
    await expect(scrollBody).toHaveCSS("overscroll-behavior-y", "contain");
    const workflowName = (await apiClient.listWorkflows(seedData.workspaceId)).workflows.find(
      (w) => w.id === seedData.workflowId,
    )!.name;
    const selected = waitForTaskList(testPage, "archived");
    await selectTaskListFilter(testPage, "workflow", workflowName, true);
    await selected;
    await expect(testPage.getByTestId("tasks-list-row-title")).toHaveText([
      "Phone archive primary",
    ]);
    await expect(drawer).toBeVisible();
    for (const id of [
      "tasks-list-workflow-filter",
      "tasks-list-repository-filter",
      "tasks-list-filters-done",
    ]) {
      expect((await testPage.getByTestId(id).boundingBox())!.height).toBeGreaterThanOrEqual(44);
    }
    const box = (await drawer.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(testPage.viewportSize()!.width + 1);
    expect(box.y + box.height).toBeLessThanOrEqual(testPage.viewportSize()!.height + 1);
    await testPage.screenshot({ path: "test-results/task-list-phone-filters.png" });
    await testPage.getByTestId("tasks-list-filters-done").tap();
    await expect(drawer).toBeHidden();
    await expect(trigger).toBeFocused();
    await expect(trigger).toContainText("(1)");
    const all = waitForTaskList(testPage, "archived");
    await testPage.getByRole("button", { name: "Remove workflow filter", exact: true }).tap();
    await all;
    await expect(testPage.getByTestId("tasks-list-row-title")).toHaveCount(2);
    const empty = waitForTaskList(testPage, "archived", "missing");
    await testPage.getByTestId("tasks-list-search").fill("missing");
    await empty;
    await expect(testPage.getByText("No tasks match these filters", { exact: true })).toBeVisible();
    const cleared = waitForTaskList(testPage, "archived");
    await testPage.getByRole("button", { name: "Clear filters", exact: true }).tap();
    await cleared;
    await expect(testPage.getByTestId("tasks-list-row-title")).toHaveCount(2);
    await expect(testPage.getByTestId("tasks-list-archive-archived")).toHaveAttribute(
      "data-state",
      "on",
    );
    await assertNoDocumentHorizontalOverflow(testPage);
    await testPage.setViewportSize({ width: 900, height: 1000 });
    expect(
      (await testPage.getByTestId("tasks-list-workflow-filter").boundingBox())!.height,
    ).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await apiClient.saveUserSettings({
      workflow_filter_id: baseline.workflow_filter_id ?? "",
      repository_ids: baseline.repository_ids ?? [],
    });
  }
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-003.4 AC-TASKS-LIST-ARCHIVE-FILTERS-004.4
test("keeps a failed archive read distinct from an empty archive and retries by touch", async ({
  testPage,
  seedData,
}) => {
  let failed = true;
  await testPage.route(/\/api\/v1\/workspaces\/[^/]+\/tasks(?:\?|$)/, async (route) => {
    if (failed)
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Temporarily unavailable" }),
      });
    else await route.continue();
  });
  await testPage.goto(
    `/tasks?workspace=${seedData.workspaceId}&archive=archived&workflow=all&repository=all`,
  );
  await expect(testPage.getByTestId("tasks-list-error")).toBeVisible();
  await expect(testPage.getByText("No archived tasks", { exact: true })).not.toBeVisible();
  failed = false;
  const retried = waitForTaskList(testPage, "archived");
  await testPage.getByRole("button", { name: "Retry", exact: true }).tap();
  await retried;
  await expect(testPage.getByText("No archived tasks", { exact: true })).toBeVisible();
  await expect(testPage.getByTestId("tasks-list-archive-archived")).toHaveAttribute(
    "data-state",
    "on",
  );
});
