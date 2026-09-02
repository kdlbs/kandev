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

  test("fits a nine-step workflow's row on a 1280px board surface without scrolling", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Nine Step Workflow");
    const steps = [];
    for (let i = 0; i < 9; i++) {
      steps.push(await apiClient.createWorkflowStep(workflow.id, `Step ${i + 1}`, i));
    }
    // AC-UI-PIPELINE-ROW-001.3's own fixture: one repository, one pull request
    // indicator, the blocked indicator, no plugin contribution.
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

    const measure = () =>
      testPage.evaluate((taskId) => {
        const row = document.querySelector(`[data-testid="pipeline-task-${taskId}"]`);
        const list = row?.closest(".overflow-x-auto:not(.min-w-0)");
        // The region AC-UI-PIPELINE-ROW-001.3 measures at this (non-terminus)
        // width is the step run's own scroll container: the outer combined
        // region only becomes scrollable at AC-UI-PIPELINE-ROW-003.11's
        // terminus, so measuring it here would pass regardless of whether the
        // collapse rule (AC-UI-PIPELINE-ROW-001.1) actually holds.
        const scrollRegion = row?.querySelector('[data-testid="pipeline-step-run-scroll"]');
        if (!row || !list || !scrollRegion) return null;
        const listRect = list.getBoundingClientRect();
        return {
          rowWidth: row.getBoundingClientRect().width,
          listClientWidth: list.clientWidth,
          listRight: listRect.left + list.clientWidth,
          scrollRegionScrollWidth: scrollRegion.scrollWidth,
          scrollRegionClientWidth: scrollRegion.clientWidth,
        };
      }, task.id);

    // The board surface is the task list's own clientWidth, not the viewport
    // (Terminology: "the width available to the task list after app chrome...
    // not viewport width"). The sidebar claims a viewport-proportional share
    // of chrome, so a 1280px viewport does not produce a 1280px board
    // surface. Calibrate from two measurements so the resulting board surface
    // reaches the AC's 1280px minimum regardless of that ratio.
    const BOARD_SURFACE_TARGET = 1280;
    const waitForListWidthChange = (fromWidth: number) =>
      expect.poll(async () => (await measure())?.listClientWidth).not.toBe(fromWidth);

    let measurements = await measure();
    expect(measurements).not.toBeNull();
    if (measurements!.listClientWidth < BOARD_SURFACE_TARGET) {
      const baseViewport = testPage.viewportSize()!.width;
      const baseListClientWidth = measurements!.listClientWidth;
      const probeViewport = baseViewport + (BOARD_SURFACE_TARGET - baseListClientWidth);
      await testPage.setViewportSize({ width: probeViewport, height: 800 });
      // Dockview recalculates sidebar/board sizing asynchronously after a
      // resize; poll until the list has actually re-rendered at the new width
      // instead of reading a stale pre-resize measurement.
      await waitForListWidthChange(baseListClientWidth);
      const probeMeasurements = await measure();
      expect(probeMeasurements).not.toBeNull();

      const slope =
        (probeMeasurements!.listClientWidth - baseListClientWidth) / (probeViewport - baseViewport);
      expect(slope).toBeGreaterThan(0);

      const targetViewport = Math.ceil(
        probeViewport + (BOARD_SURFACE_TARGET - probeMeasurements!.listClientWidth) / slope,
      );
      if (targetViewport === probeViewport) {
        measurements = probeMeasurements;
      } else {
        await testPage.setViewportSize({ width: targetViewport, height: 800 });
        await waitForListWidthChange(probeMeasurements!.listClientWidth);
        measurements = await measure();
        expect(measurements).not.toBeNull();
      }
    }
    expect(measurements!.listClientWidth).toBeGreaterThanOrEqual(BOARD_SURFACE_TARGET);

    expect(measurements!.rowWidth).toBeLessThanOrEqual(measurements!.listClientWidth);
    expect(measurements!.scrollRegionScrollWidth).toBeLessThanOrEqual(
      measurements!.scrollRegionClientWidth,
    );

    const menuTrigger = kanban.pipelineTaskMenuTrigger(task.id);
    await expect(menuTrigger).toBeVisible();
    const menuBox = await menuTrigger.boundingBox();
    expect(menuBox).not.toBeNull();
    expect(menuBox!.x + menuBox!.width).toBeLessThanOrEqual(measurements!.listRight);
  });

  test("collapses the status strip into the step run's single scroll region and keeps the actions cluster reachable when the row cannot fit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Terminus Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const region = kanban.pipelineOverflowRegion(task.id);
    await expect(region).toBeVisible();

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

  test("shows step title and destination tooltips on hover, the fine-pointer route to every step", async ({
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

    const collapsedMarker = row.getByTestId("graph2-step-node-collapsed-future").first();
    await expect(collapsedMarker).toBeVisible();
    await collapsedMarker.hover();
    await expect(testPage.getByRole("tooltip", { name: nextStep.name })).toBeVisible();

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
