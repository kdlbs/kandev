import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { activeSessionId, seedSecondaryClarificationTask } from "../../helpers/clarification";
import { makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile sidebar task actions", () => {
  test("opens the secondary session that owns clarification from the phone drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const target = await seedSecondaryClarificationTask(
      apiClient,
      seedData,
      "Mobile secondary clarification owner",
    );
    const source = await apiClient.createTask(seedData.workspaceId, "Mobile pending owner source", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${source.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();
    const drawer = testPage.getByRole("dialog", { name: "Tasks" });
    const targetRow = drawer.getByTestId("sidebar-task-item").filter({ hasText: target.title });
    await expect(targetRow).toBeVisible();
    await expect(targetRow.getByTestId("task-state-waiting-for-input")).toBeVisible();
    await targetRow.tap();

    await expect(drawer).toBeHidden();
    await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}$`));
    await expect.poll(() => activeSessionId(testPage)).toBe(target.clarificationSessionId);
    await expect(session.clarificationOverlay()).toBeVisible();
    await expect(session.clarificationOverlay()).toContainText(
      "Which database should we use for this project?",
    );
    await expect
      .poll(
        () =>
          testPage.evaluate(
            () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
          ),
        { message: "pending clarification task should not overflow the phone viewport" },
      )
      .toBe(true);
  });

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

  test("keeps the inline archive confirmation readable on a phone", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, "Mobile archive inline target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();

    const drawer = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = drawer
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile archive inline target" });
    await taskRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Archive", exact: true }).tap();

    const confirmation = taskRow.getByTestId("task-archive-inline-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(confirmation).toContainText("Mobile archive inline target");

    for (const button of [
      confirmation.getByRole("button", { name: "Cancel" }),
      confirmation.getByTestId("archive-task-confirm"),
    ]) {
      const box = await button.boundingBox();
      if (!box) throw new Error("inline archive action has no layout box");
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
      await expect(button).toBeInViewport();
    }

    const pageWidth = await testPage.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);
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
    // Chromium can report a 44px transformed box as 43.999969 CSS pixels.
    // Compare physical CSS-pixel intent, not floating-point layout residue.
    expect(Math.round(actionBox.width)).toBeGreaterThanOrEqual(44);
    expect(Math.round(actionBox.height)).toBeGreaterThanOrEqual(44);
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
      "Edit",
      "Rename",
      "Duplicate",
      "Archive",
      "Create Subtask",
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

  test("edits a started task from the phone drawer and closes the drawer", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Phone sidebar edit target", {
      description: "Phone prompt stays locked",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.updateTaskState(task.id, "IN_PROGRESS");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();

    const drawer = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = drawer
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Phone sidebar edit target" });
    await taskRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Edit", exact: true }).tap();

    await expect(drawer).toBeHidden();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog.getByTestId("task-title-input")).toBeEnabled();
    await expect(dialog.getByTestId("task-description-input")).toBeDisabled();
    await expect(dialog.getByTestId("task-description-input")).toHaveValue(
      "Phone prompt stays locked",
    );
    await prCapture.screenshot("phone-sidebar-task-edit-dialog", {
      caption: "Phone sidebar task editor with started-task prompt locked",
    });
    await dialog.getByTestId("task-title-input").fill("Phone sidebar edit updated");
    await dialog.getByRole("button", { name: "Update", exact: true }).tap();

    await expect(dialog).toHaveCount(0);
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).title)
      .toBe("Phone sidebar edit updated");
  });

  test("keeps the tablet task sheet behind the sidebar editor", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 820, height: 900 });
    const task = await apiClient.seedTask(seedData.workspaceId, "Tablet sidebar edit target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.task_id}`);
    await expect(testPage.getByTestId("tablet-task-layout")).toBeVisible();
    await testPage.evaluate(
      "window.__KANDEV_E2E_STORE__?.getState().setMobileSessionTaskSwitcherOpen(true)",
    );

    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = sheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Tablet sidebar edit target" });
    await taskRow.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Edit", exact: true }).click();

    const editor = testPage
      .getByRole("dialog")
      .filter({ has: testPage.getByTestId("task-title-input") });
    await expect(editor.getByTestId("task-title-input")).toBeVisible();
    await expect(sheet).toBeVisible();
    await prCapture.screenshot("tablet-sidebar-task-edit-dialog", {
      caption: "Tablet sidebar task editor with task sheet retained",
    });
    await editor.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(editor).toHaveCount(0);
    await expect(sheet).toBeVisible();
  });

  test("opens create subtask from the mobile task actions menu", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const parentTitle = "Mobile create subtask parent";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      parentTitle,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await testPage.getByTestId("mobile-session-menu").click();

    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = taskSheet.getByTestId("sidebar-task-item").filter({ hasText: parentTitle });
    await taskRow.getByRole("button", { name: "Task actions" }).click();

    const createSubtask = testPage.getByRole("menuitem", { name: "Create Subtask", exact: true });
    await expect(createSubtask).toBeVisible();
    await prCapture.screenshot("mobile-create-subtask-context-menu", {
      caption: "Mobile task actions menu with Create Subtask",
    });
    await createSubtask.click();

    const dialog = testPage.getByTestId("new-subtask-dialog");
    await expect(dialog).toBeVisible();
    await expect(testPage.locator('[data-slot="tooltip-content"][data-state="open"]')).toHaveCount(
      0,
    );
    await expect(testPage.getByTestId("subtask-title-input")).toHaveValue(
      /Mobile create subtask parent \/ Subtask 1/,
    );
    const parentBranchBadge = dialog.getByText("Same branch as current session", { exact: true });
    await expect(parentBranchBadge).toBeVisible();
    await expect(dialog.getByTestId("subtask-workspace-mode-inherit")).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await dialog.getByTestId("subtask-workspace-mode-new").tap();
    await expect(parentBranchBadge).toHaveCount(0);
    await expect(dialog.getByTestId("repo-chip-trigger")).toBeVisible();
    await expect(dialog.getByTestId("branch-chip-trigger")).toBeVisible();
    await prCapture.screenshot("mobile-subtask-isolated-workspace", {
      caption: "Mobile New Subtask dialog with isolated workspace controls",
    });
    await expect(testPage.getByTestId("subtask-context-autopilot-row")).toBeVisible();
    const contextTrigger = testPage
      .getByTestId("subtask-context-autopilot-row")
      .locator('[data-slot="select-trigger"]');
    const contextHeight = await contextTrigger.evaluate((element) =>
      Math.round(element.getBoundingClientRect().height),
    );
    const autopilotHeight = await testPage
      .getByTestId("autopilot-toggle-row")
      .evaluate((element) => Math.round(element.getBoundingClientRect().height));
    expect(autopilotHeight).toBe(contextHeight);
    await expect(dialog.getByTestId("autopilot-toggle-row")).toBeVisible();
    await dialog.getByRole("switch", { name: "Autopilot" }).tap();
    await expect(dialog.getByRole("switch", { name: "Autopilot" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toBeHidden();
  });

  test("uses the selected non-active parent defaults for mobile subtasks", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const parentRepoDir = path.join(backend.tmpDir, "repos", "mobile-parent-repo");
    fs.mkdirSync(parentRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: parentRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: parentRepoDir, env: gitEnv });
    const parentRepo = await apiClient.createRepository(
      seedData.workspaceId,
      parentRepoDir,
      "main",
      { name: "Mobile parent repo" },
    );
    const activeTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile active task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile non-active parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [parentRepo.id],
      },
    );

    await testPage.goto(`/t/${activeTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();

    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const parentRow = taskSheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile non-active parent" });
    await parentRow.getByRole("button", { name: "Task actions" }).click();
    await testPage.getByRole("menuitem", { name: "Create Subtask", exact: true }).click();

    const dialog = testPage.getByTestId("new-subtask-dialog");
    await expect(dialog).toBeVisible();
    await expect(testPage.getByTestId("repo-chip-trigger")).toContainText("Mobile parent repo");
    await expect(testPage.getByTestId("subtask-title-input")).toHaveValue(
      /Mobile non-active parent \/ Subtask 1/,
    );
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
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
