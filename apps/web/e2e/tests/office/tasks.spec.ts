import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test.describe("Tasks (Issues)", () => {
  test("tasks page loads", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/tasks");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Tasks/i, {
      timeout: 10_000,
    });
  });

  test("list tasks returns array", async ({ officeApi, officeSeed }) => {
    const result = await officeApi.listTasks(officeSeed.workspaceId);
    const issues = (result as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    expect(Array.isArray(issues)).toBe(true);
  });

  test("onboarding task appears in tasks when created with title", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Use the existing office workspace and create a task directly rather than
    // re-running onboarding (which would fail: "workspace already has a CEO agent").
    await apiClient.createTask(officeSeed.workspaceId, "My Onboarding Issue", {
      workflow_id: officeSeed.workflowId,
    });

    const issues = await officeApi.listTasks(officeSeed.workspaceId);
    const list = (issues as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    const found = list.find((i) => (i as Record<string, unknown>).title === "My Onboarding Issue");
    expect(found).toBeDefined();
  });

  test("task created via API appears in tasks list", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    await apiClient.createTask(officeSeed.workspaceId, "API Created Issue", {
      workflow_id: officeSeed.workflowId,
    });

    const result = await officeApi.listTasks(officeSeed.workspaceId);
    const list = (result as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    const found = list.find((i) => (i as Record<string, unknown>).title === "API Created Issue");
    expect(found).toBeDefined();
  });

  test("task created via API appears in tasks page UI", async ({
    testPage,
    apiClient,
    officeApi: _,
    officeSeed,
  }) => {
    await apiClient.createTask(officeSeed.workspaceId, "UI Visible Issue", {
      workflow_id: officeSeed.workflowId,
    });

    await testPage.goto("/office/tasks");
    await expect(testPage.getByText("UI Visible Issue")).toBeVisible({ timeout: 10_000 });
  });

  test("subtasks are expanded and nested by default", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const parentTitle = "Default Expanded Parent";
    const childTitle = "Default Expanded Child";
    const parent = await apiClient.createTask(officeSeed.workspaceId, parentTitle, {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, childTitle, {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    await testPage.goto("/office/tasks");

    await expect(testPage.getByText(parentTitle)).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(childTitle)).toBeVisible();

    const parentTitleBox = await testPage.getByText(parentTitle).boundingBox();
    const childTitleBox = await testPage.getByText(childTitle).boundingBox();
    expect(parentTitleBox).not.toBeNull();
    expect(childTitleBox).not.toBeNull();
    expect(childTitleBox!.x).toBeGreaterThan(parentTitleBox!.x + 16);

    const parentRow = testPage.getByRole("button", { name: new RegExp(parentTitle) });
    await parentRow.getByRole("button", { name: "Collapse" }).click();
    await expect(testPage.getByText(childTitle)).toBeHidden();
  });

  test("get task by id returns correct data", async ({ apiClient, officeApi, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Fetch By ID Issue", {
      workflow_id: officeSeed.workflowId,
    });

    const issueResp = await officeApi.getTask(task.id);
    const i = (issueResp as { task: Record<string, unknown> }).task;
    expect(i.id).toBe(task.id);
    expect(i.title).toBe("Fetch By ID Issue");
  });

  test("subtask has parent_id in task response", async ({ apiClient, officeApi, officeSeed }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Parent Issue", {
      workflow_id: officeSeed.workflowId,
    });
    const child = await apiClient.createTask(officeSeed.workspaceId, "Child Issue", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    const childIssueResp = await officeApi.getTask(child.id);
    const c = (childIssueResp as { task: Record<string, unknown> }).task;
    expect(c.id).toBe(child.id);
    expect(c.title).toBe("Child Issue");
    // parent linkage preserved
    expect(c.parentId ?? c.parent_id).toBe(parent.id);
  });

  test("task search returns matching results", async ({ apiClient, officeApi, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Searchable Unique Task XYZ987", {
      workflow_id: officeSeed.workflowId,
    });

    const results = await officeApi.searchTasks(officeSeed.workspaceId, "XYZ987");
    const tasks = (results as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    expect(tasks.length).toBeGreaterThan(0);
    const found = tasks.find(
      (t) =>
        (t as Record<string, unknown>).title &&
        ((t as Record<string, unknown>).title as string).includes("XYZ987"),
    );
    expect(found).toBeDefined();
  });

  test("task search with no results returns empty array", async ({ officeApi, officeSeed }) => {
    const results = await officeApi.searchTasks(
      officeSeed.workspaceId,
      "NORESULT_NONEXISTENT_TOKEN_99999",
    );
    const tasks = (results as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    expect(Array.isArray(tasks)).toBe(true);
    expect(tasks.length).toBe(0);
  });

  test("dragging a card to another column changes the task status", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Fillers pile up in todo (the CREATED wire state normalizes there - see
    // office-task-normalize.ts's STATUS_MAP - and it's where a task created
    // with no workflow_step_id lands) so that column is visibly taller than
    // the empty in_progress column. Flex row stretch then makes in_progress
    // span the same height on screen, but its droppable was previously sized
    // to its own (empty) content - about one min-h box. Dropping in the
    // resulting blank space, not at the column's own reported center, is
    // what actually exercises that gap.
    for (let i = 0; i < 6; i++) {
      await apiClient.createTask(officeSeed.workspaceId, `Filler ${i}`, {
        workflow_id: officeSeed.workflowId,
      });
    }
    const task = await apiClient.createTask(officeSeed.workspaceId, "Board Drag Task", {
      workflow_id: officeSeed.workflowId,
    });

    await testPage.goto("/office/tasks");
    await testPage.getByTestId("task-view-board").click();

    const card = testPage.getByTestId(`board-card-${task.id}`);
    await expect(card).toBeVisible({ timeout: 15_000 });

    const tallColumn = testPage.getByTestId("board-column-todo");
    const target = testPage.getByTestId("board-column-in_progress");
    await expect(target).toBeVisible();
    // The drop must actually move the card, so it cannot already live there.
    await expect(target.getByTestId(`board-card-${task.id}`)).toHaveCount(0);

    const from = await card.boundingBox();
    const tallBox = await tallColumn.boundingBox();
    const targetBox = await target.boundingBox();
    expect(from).not.toBeNull();
    expect(tallBox).not.toBeNull();
    expect(targetBox).not.toBeNull();

    // The drop point's Y comes from the tall todo column's real height, not
    // from the target column's own reported box - so a target droppable
    // that under-reports its own height (the exact shape of the bug this
    // reproduces) cannot make the test pass by construction.
    const dropX = targetBox!.x + targetBox!.width / 2;
    const dropY = tallBox!.y + tallBox!.height - 10;

    // dnd-kit's PointerSensor needs a real pointer path, not a single jump:
    // the 8px activation distance only trips on intermediate moves.
    await testPage.mouse.move(from!.x + from!.width / 2, from!.y + from!.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(dropX, dropY, { steps: 12 });
    await testPage.mouse.up();

    // The card lands in the target column...
    await expect(target.getByTestId(`board-card-${task.id}`)).toBeVisible({ timeout: 15_000 });

    // ...and the move is persisted, not just an optimistic store patch.
    await expect
      .poll(
        async () => {
          const persisted = (await officeApi.getTask(task.id)) as Record<string, unknown>;
          const inner = (persisted.task as Record<string, unknown>) ?? persisted;
          return ((inner.status as string) ?? (inner.state as string) ?? "").toLowerCase();
        },
        { timeout: 15_000 },
      )
      .toContain("progress");
  });

  test("dragging a searched card updates the visible search result", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    for (let i = 0; i < 6; i++) {
      await apiClient.createTask(officeSeed.workspaceId, `Searched Board Filler ${i}`, {
        workflow_id: officeSeed.workflowId,
      });
    }
    const task = await apiClient.createTask(officeSeed.workspaceId, "Searched Board Drag Task", {
      workflow_id: officeSeed.workflowId,
    });

    await testPage.goto("/office/tasks");
    await testPage.getByTestId("task-view-board").click();

    const searchInput = testPage.getByPlaceholder(/search tasks/i);
    const searchResponse = testPage.waitForResponse(
      (response) =>
        response.url().includes("/tasks/search?") &&
        response.request().method() === "GET" &&
        response.ok(),
    );
    await searchInput.fill("Searched Board");
    await searchResponse;

    const card = testPage.getByTestId(`board-card-${task.id}`);
    await expect(card).toBeVisible({ timeout: 15_000 });

    const tallColumn = testPage.getByTestId("board-column-todo");
    const target = testPage.getByTestId("board-column-in_progress");
    await expect(target.getByTestId(`board-card-${task.id}`)).toHaveCount(0);

    const from = await card.boundingBox();
    const tallBox = await tallColumn.boundingBox();
    const targetBox = await target.boundingBox();
    expect(from).not.toBeNull();
    expect(tallBox).not.toBeNull();
    expect(targetBox).not.toBeNull();

    await testPage.mouse.move(from!.x + from!.width / 2, from!.y + from!.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(
      targetBox!.x + targetBox!.width / 2,
      tallBox!.y + tallBox!.height - 10,
      { steps: 12 },
    );
    await testPage.mouse.up();

    await expect
      .poll(
        async () => {
          const persisted = (await officeApi.getTask(task.id)) as Record<string, unknown>;
          const inner = (persisted.task as Record<string, unknown>) ?? persisted;
          return ((inner.status as string) ?? (inner.state as string) ?? "").toLowerCase();
        },
        { timeout: 15_000 },
      )
      .toContain("progress");
    await expect(target.getByTestId(`board-card-${task.id}`)).toBeVisible({ timeout: 15_000 });
  });
});
