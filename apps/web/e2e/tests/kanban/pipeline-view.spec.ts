import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Pipeline view", () => {
  test.beforeEach(async ({ testPage }) => {
    // The view toggle and multi-select toggle are only rendered on desktop layouts.
    await testPage.setViewportSize({ width: 1280, height: 800 });
  });

  test("shows the linked repository name beneath the task title", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Repo Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const repoName = kanban.pipelineTaskRepoName(task.id);
    await expect(repoName).toBeVisible();
    // seedData seeds the repository with the default name from createRepository().
    await expect(repoName).toHaveText("E2E Repo");
  });

  test("does not show a repo name when the task has no linked repository", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline No Repo Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await expect(kanban.pipelineTask(task.id)).toBeVisible();
    await expect(kanban.pipelineTaskRepoName(task.id)).toHaveCount(0);
  });

  test("starts every row's step run at the same x, whatever each row's own content is", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Two rows that differ in exactly the things that used to push the step
    // run sideways: title length, a linked repository, and a status badge.
    const predecessor = await apiClient.createTask(seedData.workspaceId, "Alignment Predecessor", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const [plain, loaded] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Short", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(
        seedData.workspaceId,
        "A longer pipeline task title that will not fit its column",
        {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          blocked_by: [predecessor.id],
        },
      ),
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();
    await expect(kanban.pipelineTask(plain.id)).toBeVisible();
    await expect(
      kanban.pipelineTask(loaded.id).getByTestId("kanban-card-blocked-badge"),
    ).toBeVisible();

    const [plainInfo, loadedInfo, plainRun, loadedRun] = await Promise.all([
      kanban.pipelineTaskInfo(plain.id).boundingBox(),
      kanban.pipelineTaskInfo(loaded.id).boundingBox(),
      kanban.pipelineStepRunScroll(plain.id).boundingBox(),
      kanban.pipelineStepRunScroll(loaded.id).boundingBox(),
    ]);

    // The information column is one fixed width for every row, so the runs
    // line up as a single track down the board.
    expect(Math.abs(plainInfo!.width - loadedInfo!.width)).toBeLessThanOrEqual(1);
    expect(Math.abs(plainRun!.x - loadedRun!.x)).toBeLessThanOrEqual(1);
  });

  test("clicking the row opens the preview panel when 'Open preview on click' is on", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Preview Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await kanban.pipelineTaskTitle(task.id).click();

    // Preview panel opens in place (URL carries taskId=), not a full-page
    // navigation to /t/:id. This is a synchronous client-side route update,
    // not a backend round trip, so the default assertion timeout is enough.
    await expect(testPage).toHaveURL(/taskId=/);
    await expect(testPage.getByTestId("task-preview-panel")).toBeVisible();

    await apiClient.saveUserSettings({ enable_preview_on_click: false });
  });

  test("clicking the row navigates to the full task page when 'Open preview on click' is off", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Full Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await kanban.pipelineTaskTitle(task.id).click();

    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
  });

  test("multi-select checkbox appears and toolbar reflects the selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Pipeline MS 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Pipeline MS 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    // Checkbox is hidden until multi-select mode is enabled.
    await expect(kanban.taskSelectCheckbox(t1.id)).toHaveCount(0);
    await expect(kanban.multiSelectToolbar).not.toBeVisible();

    await kanban.selectPipelineTask(t1.id);
    await expect(kanban.taskSelectCheckbox(t1.id)).toBeVisible();
    await expect(kanban.multiSelectToolbar).toBeVisible();
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");

    // Once multi-select mode is on, every pipeline task exposes a checkbox.
    await kanban.taskSelectCheckbox(t2.id).click();
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");
  });

  test("bulk delete from pipeline view removes the selected tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Pipeline Delete 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Pipeline Delete 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await kanban.selectPipelineTask(t1.id);
    await kanban.taskSelectCheckbox(t2.id).click();
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");

    await kanban.bulkDeleteButton.click();
    await expect(kanban.bulkDeleteConfirm).toBeVisible();
    await kanban.bulkDeleteConfirm.click();

    await expect(kanban.pipelineTask(t1.id)).toHaveCount(0, { timeout: 10000 });
    await expect(kanban.pipelineTask(t2.id)).toHaveCount(0);
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
  });

  test("keeps a nine-step workflow's row within the board surface, scrolling the step run to the current step instead of growing", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Nine Step Workflow");
    const steps = [];
    for (let i = 0; i < 9; i++) {
      steps.push(await apiClient.createWorkflowStep(workflow.id, `Step ${i + 1}`, i));
    }
    const predecessor = await apiClient.createTask(seedData.workspaceId, "Nine Step Predecessor", {
      workflow_id: workflow.id,
      workflow_step_id: steps[0].id,
    });
    const task = await apiClient.createTask(seedData.workspaceId, "Nine Step Fit Task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[4].id,
      repository_ids: [seedData.repositoryId],
      blocked_by: [predecessor.id],
    });
    await apiClient.updateTaskState(predecessor.id, "FAILED");

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 900,
      pr_url: "https://github.com/testorg/testrepo/pull/900",
      pr_title: "Nine step fit",
      head_branch: "feat/nine-step-fit",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    const kanban = new KanbanPage(testPage);
    await testPage.setViewportSize({ width: 1280, height: 800 });
    await testPage.goto(`/?workflowId=${workflow.id}`);
    await expect(kanban.board).toBeVisible();
    await kanban.switchToPipelineView();
    await expect(kanban.pipelineTask(task.id)).toBeVisible();
    await expect(kanban.pipelineTask(task.id).getByTestId(`pr-task-icon-${task.id}`)).toBeVisible({
      timeout: 15_000,
    });
    await expect(
      kanban.pipelineTask(task.id).getByTestId("kanban-card-blocked-badge"),
    ).toBeVisible();

    // Every step keeps its own labelled pill (no collapsing), so a nine-step
    // run routinely needs more width than the row has: the step-run lane
    // scrolls internally instead. The row itself must still never grow past
    // the task list, and the current step's pill -- not an earlier one --
    // must be the part that stays in view without further scrolling.
    const row = kanban.pipelineTask(task.id);
    await expect
      .poll(async () =>
        testPage.evaluate((taskId) => {
          const rowEl = document.querySelector(`[data-testid="pipeline-task-${taskId}"]`);
          const list = rowEl?.closest(".overflow-x-auto:not(.min-w-0)");
          if (!rowEl || !list) return null;
          return rowEl.getBoundingClientRect().width <= list.clientWidth + 1;
        }, task.id),
      )
      .toBe(true);

    const currentPill = row.getByText(steps[4].name, { exact: true });
    await expect(currentPill).toBeVisible();
    const scrollRegion = kanban.pipelineStepRunScroll(task.id);
    const [pillBox, scrollBox] = await Promise.all([
      currentPill.boundingBox(),
      scrollRegion.boundingBox(),
    ]);
    expect(pillBox).not.toBeNull();
    expect(scrollBox).not.toBeNull();
    expect(pillBox!.x).toBeGreaterThanOrEqual(scrollBox!.x - 1);
    expect(pillBox!.x + pillBox!.width).toBeLessThanOrEqual(scrollBox!.x + scrollBox!.width + 1);

    const menuTrigger = kanban.pipelineTaskMenuTrigger(task.id);
    await expect(menuTrigger).toBeVisible();
  });

  test("collapses the status strip into the step run's single scroll region and keeps the actions cluster reachable when the row cannot fit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // The task carries status, so the strip has natural width to not fit. A
    // task with nothing to show renders an empty strip, and an empty strip
    // never reaches the terminus — there is nothing for the step run's own
    // scroll to fail to accommodate.
    const predecessor = await apiClient.createTask(seedData.workspaceId, "Terminus Blocker", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Terminus Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      blocked_by: [predecessor.id],
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const region = kanban.pipelineOverflowRegion(task.id);
    await expect(region).toBeVisible();
    await expect(
      kanban.pipelineTask(task.id).getByTestId("kanban-card-blocked-badge"),
    ).toBeVisible();

    // Force the combined region far narrower than the status strip's natural
    // content so the strip alone cannot fit, driving the row into its
    // single-scroll-region terminus.
    await testPage.addStyleTag({
      content: `[data-testid="pipeline-row-overflow-region"] { max-width: 24px !important; }`,
    });

    await expect(region).toHaveClass(/overflow-x-auto/);
    // The step run never scrolls independently while the terminus owns the
    // scroll: only one scrollable region exists at a time.
    await expect(kanban.pipelineStepRunScroll(task.id)).not.toHaveClass(/overflow-x-auto/);

    const menuTrigger = kanban.pipelineTaskMenuTrigger(task.id);
    await expect(menuTrigger).toBeVisible();
    await menuTrigger.click();
    await expect(testPage.getByRole("menuitem", { name: "Edit" })).toBeVisible();
  });

  test("shows every step's title inline and destination tooltips on hover of the current step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const currentIndex = seedData.steps.findIndex((step) => step.id === seedData.startStepId);
    const nextStep = seedData.steps[currentIndex + 1];
    if (!nextStep) {
      test.skip(true, "seed workflow needs a step after the start step");
      return;
    }
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Hover Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();
    const row = kanban.pipelineTask(task.id);
    await expect(row).toBeVisible();

    // Every step keeps a visible labelled pill, so the next step's title
    // needs no hover disclosure -- only the current step's move controls do.
    await expect(row.getByTestId("graph2-step-node-future").first()).toContainText(nextStep.name);

    await row.getByText(seedData.steps[currentIndex].name, { exact: true }).hover();
    await expect(row.getByLabel(`Move to ${nextStep.name}`)).toBeVisible();
  });

  test("opens the shared task menu on right-click, matching the Kanban card", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Context Menu Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await kanban.openPipelineTaskContextMenu(task.id);
    const expectedLabels = ["Edit", "Move to", "Archive", "Delete"];
    for (const label of expectedLabels) {
      await expect(testPage.getByRole("menuitem", { name: label })).toBeVisible();
    }
  });

  test("keeps the task-menu trigger reachable on a coarse-pointer tablet", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Pipeline Tablet Overflow Workflow",
    );
    const targetStepCount = 9;
    const steps: { id: string; title: string }[] = [];
    for (let position = 0; position < targetStepCount; position++) {
      const title = `Tablet Step ${position}`;
      const step = await apiClient.createWorkflowStep(workflow.id, title, position);
      steps.push({ id: step.id, title });
    }
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Tablet Task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[targetStepCount - 1].id,
    });
    const kanban = new KanbanPage(tabletTestPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const trigger = kanban.pipelineTaskMenuTrigger(task.id);
    await expect(trigger).toBeVisible();
    const box = await trigger.boundingBox();
    const viewportSize = tabletTestPage.viewportSize();
    expect(box).not.toBeNull();
    expect(viewportSize).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewportSize!.width);

    const hitTarget = await tabletTestPage.evaluate(
      ({ x, y }) =>
        document
          .elementFromPoint(x, y)
          ?.closest<HTMLElement>('[data-testid^="pipeline-row-menu-trigger-"]')?.dataset.testid ??
        null,
      { x: box!.x + box!.width / 2, y: box!.y + box!.height / 2 },
    );
    expect(hitTarget).toBe(`pipeline-row-menu-trigger-${task.id}`);

    await trigger.tap();
    await expect(tabletTestPage.getByRole("menuitem", { name: "Delete" })).toBeVisible();
  });

  test("the current pill's right move chevron stays clickable when it is the last visible pill", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Auto-hide-empty-steps is the common configuration that makes a task's
    // own pill the LAST VISIBLE pill while it still has a next move target:
    // the trailing step has no tasks, so it is hidden from `steps` but
    // remains in `moveTargetSteps`, which is exactly the hidden-destination-
    // disclosure affordance (nextStepHidden). Dedicated workflow, not
    // seedData.workflowId, so the auto-hide setting doesn't leak into other
    // specs sharing this worker's seed data.
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Pipeline Last Visible Pill Workflow",
    );
    const currentStep = await apiClient.createWorkflowStep(workflow.id, "Current", 0);
    await apiClient.createWorkflowStep(workflow.id, "Hidden Next", 1);
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      workflow_ids_with_auto_hide_empty_steps: [workflow.id],
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Last Pill Task", {
      workflow_id: workflow.id,
      workflow_step_id: currentStep.id,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const row = kanban.pipelineTask(task.id);
    const currentPill = row.getByRole("button", { name: "Current" });
    await expect(currentPill).toBeVisible();
    // Hidden Next never renders: it has no tasks and auto-hide is on.
    await expect(row.getByRole("button", { name: "Hidden Next" })).toHaveCount(0);

    await currentPill.hover();
    const moveChevron = row.getByRole("button", { name: "Move to Hidden Next" });
    await expect(moveChevron).toBeVisible();

    // A plain click exercises Playwright's actionability check, which fails
    // if another element intercepts the pointer event at the chevron's
    // location - the exact F6 regression.
    const moved = waitForHttp(testPage, "POST", /\/tasks\/[^/]+\/move$/);
    await moveChevron.click();
    await moved;

    // The move actually landed: the task's current pill is now the
    // previously-hidden step, which is no longer empty and so no longer
    // auto-hidden. The backend round trip is already awaited above, so this
    // needs no hand-picked budget beyond the default.
    await expect(row.getByRole("button", { name: "Hidden Next" })).toBeVisible();
  });
});
