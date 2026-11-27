import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test.describe("Tasks (Issues)", () => {
  test("tasks list renders multiple persisted tasks, identifiers and newly created tasks after reload", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    expect(officeSeed.workspaceId).toBeTruthy();
    expect(officeSeed.agentId).toBeTruthy();
    const titles = ["Task List Alpha", "Task List Beta"];
    const created = [];
    for (const title of titles)
      created.push(
        await apiClient.createTask(officeSeed.workspaceId, title, {
          workflow_id: officeSeed.workflowId,
        }),
      );
    const { tasks } = (await officeApi.listTasks(officeSeed.workspaceId)) as {
      tasks: Array<Record<string, unknown>>;
    };
    expect(Array.isArray(tasks)).toBe(true);
    for (const task of created)
      expect(tasks.find((entry) => entry.id === task.id)).toMatchObject({ title: task.title });

    await testPage.goto("/office/tasks");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Tasks/i, { timeout: 10_000 });
    const main = testPage.locator("main");
    for (const title of titles) await expect(main.getByText(title, { exact: true })).toBeVisible();
    const row = main.getByRole("button", { name: /Task List Alpha/ });
    await expect(row.locator(".font-mono")).toBeAttached();
    await officeApi.updateTaskStatus(created[0].id, "in_progress", "Starting work");
    await expect(main.getByText(titles[0], { exact: true })).toBeVisible();
    await apiClient.createTask(officeSeed.workspaceId, "Task Created While List Open", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.reload();
    for (const title of [...titles, "Task Created While List Open"])
      await expect(main.getByText(title, { exact: true })).toBeVisible({ timeout: 10_000 });
  });

  test("subtasks are expanded and nested by default", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const parentTitle = "Default Expanded Parent";
    const childTitle = "Default Expanded Child";
    const parent = await apiClient.createTask(officeSeed.workspaceId, parentTitle, {
      workflow_id: officeSeed.workflowId,
    });
    const child = await apiClient.createTask(officeSeed.workspaceId, childTitle, {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });
    const parentResponse = (await officeApi.getTask(parent.id)) as {
      task: Record<string, unknown>;
    };
    expect(parentResponse.task).toMatchObject({ id: parent.id, title: parentTitle });
    const childResponse = (await officeApi.getTask(child.id)) as { task: Record<string, unknown> };
    expect(childResponse.task).toMatchObject({ id: child.id, title: childTitle });
    expect(childResponse.task.parentId).toBe(parent.id);

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

  test("task search returns matching results and an empty array for no match", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Searchable Unique Task XYZ987", {
      workflow_id: officeSeed.workflowId,
    });

    const results = await officeApi.searchTasks(officeSeed.workspaceId, "XYZ987");
    const tasks = (results as { tasks: Record<string, unknown>[] }).tasks;
    expect(tasks.length).toBeGreaterThan(0);
    const found = tasks.find(
      (t) =>
        (t as Record<string, unknown>).title &&
        ((t as Record<string, unknown>).title as string).includes("XYZ987"),
    );
    expect(found).toBeDefined();
    const empty = (await officeApi.searchTasks(
      officeSeed.workspaceId,
      "NORESULT_NONEXISTENT_TOKEN_99999",
    )) as { tasks: unknown[] };
    expect(Array.isArray(empty.tasks)).toBe(true);
    expect(empty.tasks).toHaveLength(0);
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
          const persisted = (await officeApi.getTask(task.id)) as { task: { status: string } };
          return persisted.task.status;
        },
        { timeout: 15_000 },
      )
      .toBe("in_progress");
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
          const persisted = (await officeApi.getTask(task.id)) as { task: { status: string } };
          return persisted.task.status;
        },
        { timeout: 15_000 },
      )
      .toBe("in_progress");
    await expect(target.getByTestId(`board-card-${task.id}`)).toBeVisible({ timeout: 15_000 });
  });
});
