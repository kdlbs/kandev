import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile sidebar task actions", () => {
  test("switches to the selected task and its chat from the phone task drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sourceTitle = "Mobile drawer source task";
    const destinationTitle = "Mobile drawer destination task";
    const destinationResponse = "destination drawer session response";
    const taskOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    };
    const source = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      sourceTitle,
      seedData.agentProfileId,
      { ...taskOptions, description: 'e2e:message("source drawer session response")' },
    );
    const destination = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      destinationTitle,
      seedData.agentProfileId,
      { ...taskOptions, description: `e2e:message("${destinationResponse}")` },
    );
    if (!source.session_id || !destination.session_id) {
      throw new Error("mobile drawer task setup did not create both sessions");
    }
    expect(source.session_id).not.toBe(destination.session_id);

    // Wait for the destination's actual agent response before navigating, so
    // the assertion below proves that the drawer selected its session rather
    // than merely replacing the URL/title.
    await expect
      .poll(async () => {
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/task-sessions/${destination.session_id}/messages?limit=50`,
        );
        const body = (await response.json()) as { messages?: Array<{ content?: string }> };
        return body.messages?.some((message) => message.content?.includes(destinationResponse));
      })
      .toBe(true);

    await testPage.goto(`/t/${source.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("source drawer session response").last()).toBeVisible({
      timeout: 30_000,
    });

    await testPage.getByTestId("mobile-session-menu").tap();
    const drawer = testPage.getByRole("dialog", { name: "Tasks" });
    const destinationRow = drawer
      .getByTestId("sidebar-task-item")
      .filter({ hasText: destinationTitle });
    await expect(destinationRow).toBeVisible();
    await destinationRow.tap();

    await expect(drawer).toBeHidden();
    await expect(testPage).toHaveURL(new RegExp(`/t/${destination.id}$`));
    const mobileTopBar = testPage
      .getByTestId("mobile-session-menu")
      .locator("xpath=ancestor::header");
    await expect(mobileTopBar.getByText(destinationTitle, { exact: true })).toBeVisible();
    await expect(session.chat.getByText(destinationResponse).last()).toBeVisible({
      timeout: 30_000,
    });
  });

  test("opens the phone task switcher as an inset bottom card", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, "Mobile task drawer surface", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();

    const surface = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(surface).toBeVisible();
    await expect(surface).toHaveAttribute("data-slot", "drawer-content");
    await surface.evaluate((element) =>
      Promise.all(
        element
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished.catch(() => undefined)),
      ),
    );

    const card = surface.locator('[data-slot="drawer-header"]');
    const [cardBox, viewport] = await Promise.all([
      card.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    if (!cardBox) throw new Error("mobile task drawer card has no layout box");
    expect(cardBox.x).toBeGreaterThanOrEqual(7);
    expect(cardBox.x + cardBox.width).toBeLessThanOrEqual(viewport.width - 7);
    expect(cardBox.y).toBeGreaterThan(0);
  });

  test("keeps the tablet task switcher as a left-side sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 820, height: 900 });
    const task = await apiClient.seedTask(seedData.workspaceId, "Tablet task sheet surface", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    await expect(testPage.getByTestId("tablet-task-layout")).toBeVisible();
    await testPage.evaluate(
      "window.__KANDEV_E2E_STORE__?.getState().setMobileSessionTaskSwitcherOpen(true)",
    );

    const surface = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(surface).toBeVisible();
    await expect(surface).toHaveAttribute("data-slot", "sheet-content");
    await expect(surface).toHaveAttribute("data-side", "left");
  });

  test("returns to the tablet task switcher after canceling a task action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 820, height: 900 });
    const title = "Tablet task action target";
    const task = await apiClient.seedTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    await expect(testPage.getByTestId("tablet-task-layout")).toBeVisible();
    await testPage.evaluate(
      "window.__KANDEV_E2E_STORE__?.getState().setMobileSessionTaskSwitcherOpen(true)",
    );

    const surface = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = surface.getByTestId("sidebar-task-item").filter({ hasText: title });
    await taskRow.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Rename", exact: true }).click();

    const renameDialog = testPage.getByRole("dialog", { name: "Rename task" });
    await expect(renameDialog).toBeVisible();
    await expect(surface).toBeVisible();
    await renameDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(renameDialog).toHaveCount(0);
    await expect(surface).toBeVisible();
  });

  test("opens a viewport-bound action sheet without covering diff stats", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskTitle = "Mobile task with diff stats";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      taskTitle,
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({
      timeout: 60_000,
    });

    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = sheet.getByTestId("sidebar-task-item").filter({ hasText: taskTitle });
    const diffStats = taskRow.getByTestId("sidebar-task-diff-stats");
    const actions = taskRow.getByRole("button", { name: "Task actions" });

    await expect(diffStats).toBeVisible({ timeout: 15_000 });
    await expect(actions).toBeVisible();
    const [diffBox, actionBox] = await Promise.all([
      diffStats.boundingBox(),
      actions.boundingBox(),
    ]);
    if (!diffBox || !actionBox) throw new Error("mobile task controls have no layout box");
    expect(actionBox.width).toBeGreaterThanOrEqual(44);
    expect(actionBox.height).toBeGreaterThanOrEqual(44);
    const overlapWidth =
      Math.min(diffBox.x + diffBox.width, actionBox.x + actionBox.width) -
      Math.max(diffBox.x, actionBox.x);
    const overlapHeight =
      Math.min(diffBox.y + diffBox.height, actionBox.y + actionBox.height) -
      Math.max(diffBox.y, actionBox.y);
    expect(overlapWidth <= 0 || overlapHeight <= 0).toBe(true);

    await testPage.setViewportSize({ width: 390, height: 480 });
    await actions.click();

    const archiveItem = testPage.getByRole("menuitem", { name: "Archive", exact: true });
    await expect(archiveItem).toBeVisible();
    const menu = archiveItem.locator("xpath=ancestor::*[@role='menu'][1]");
    await menu.evaluate((element) =>
      Promise.all(
        element
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished.catch(() => undefined)),
      ),
    );
    const [menuBox, itemBox] = await Promise.all([menu.boundingBox(), archiveItem.boundingBox()]);
    const viewport = testPage.viewportSize();
    if (!menuBox || !itemBox || !viewport) throw new Error("mobile action sheet has no layout box");

    expect(menuBox.x).toBeGreaterThanOrEqual(8);
    expect(menuBox.x).toBeLessThanOrEqual(10);
    expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(viewport.width - 8);
    expect(menuBox.width).toBeGreaterThanOrEqual(viewport.width - 20);
    expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(viewport.height);
    expect(viewport.height - (menuBox.y + menuBox.height)).toBeGreaterThanOrEqual(7);
    expect(viewport.height - (menuBox.y + menuBox.height)).toBeLessThanOrEqual(10);
    expect(itemBox.height).toBeGreaterThanOrEqual(44);
    const menuOverflow = await menu.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
    }));
    expect(menuOverflow.scrollHeight).toBeGreaterThan(menuOverflow.clientHeight);
    for (const actionName of [
      "Pin",
      "Rename",
      "Duplicate",
      "Archive",
      "Color",
      "Link",
      "Move to",
      "Delete",
    ]) {
      await expect(menu.getByRole("menuitem", { name: actionName, exact: true })).toHaveCount(1);
    }
    await archiveItem.scrollIntoViewIfNeeded();
    await expect(archiveItem).toBeInViewport();
    await expect(diffStats).toBeVisible();
  });

  test("moves a task to another step from the mobile task drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetStep = seedData.steps.find((step) => step.id !== seedData.startStepId);
    if (!targetStep) throw new Error("mobile move test requires at least two workflow steps");
    const task = await apiClient.seedTask(seedData.workspaceId, "Mobile move target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();

    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = sheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile move target" });
    const actions = taskRow.getByRole("button", { name: "Task actions" });
    await expect(actions).toBeVisible({ timeout: 15_000 });
    await actions.click();

    await testPage.getByTestId("task-context-move-to").click();
    await testPage.getByTestId(`task-context-step-${targetStep.id}`).click();

    await expect
      .poll(async () => (await apiClient.getTask(task.task_id)).workflow_step_id)
      .toBe(targetStep.id);
  });
});
