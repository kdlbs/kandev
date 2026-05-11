import { type Locator, type Page } from "@playwright/test";

export class KanbanPage {
  readonly board: Locator;
  readonly createTaskButton: Locator;
  readonly multiSelectToolbar: Locator;
  readonly bulkDeleteButton: Locator;
  readonly bulkArchiveButton: Locator;
  readonly bulkMoveButton: Locator;
  readonly bulkClearButton: Locator;
  readonly bulkDeleteConfirm: Locator;
  readonly bulkArchiveConfirm: Locator;

  readonly multiSelectToggle: Locator;

  readonly viewTogglePipeline: Locator;
  readonly viewToggleKanban: Locator;

  constructor(private page: Page) {
    this.board = page.getByTestId("kanban-board");
    this.createTaskButton = page.getByTestId("create-task-button");
    this.multiSelectToolbar = page.getByTestId("multi-select-toolbar");
    this.multiSelectToggle = page.getByTestId("multi-select-toggle");
    this.bulkDeleteButton = page.getByTestId("bulk-delete-button");
    this.bulkArchiveButton = page.getByTestId("bulk-archive-button");
    this.bulkMoveButton = page.getByTestId("bulk-move-button");
    this.bulkClearButton = page.getByTestId("bulk-clear-selection");
    this.bulkDeleteConfirm = page.getByTestId("bulk-delete-confirm");
    this.bulkArchiveConfirm = page.getByTestId("bulk-archive-confirm");
    this.viewTogglePipeline = page.getByTestId("view-toggle-pipeline");
    this.viewToggleKanban = page.getByTestId("view-toggle-kanban");
  }

  async goto() {
    await this.page.goto("/");
    await this.board.waitFor({ state: "visible" });
  }

  taskCard(taskId: string): Locator {
    return this.page.getByTestId(`task-card-${taskId}`);
  }

  taskCardByTitle(title: string): Locator {
    return this.board.locator(`[data-testid^="task-card-"]`, {
      has: this.page.locator('[data-testid="task-card-title"]', { hasText: title }),
    });
  }

  taskSelectCheckbox(taskId: string): Locator {
    return this.page.getByTestId(`task-select-checkbox-${taskId}`);
  }

  bulkMoveStepOption(stepId: string): Locator {
    return this.page.getByTestId(`bulk-move-step-${stepId}`);
  }

  columnByStepId(stepId: string): Locator {
    return this.page.getByTestId(`kanban-column-${stepId}`);
  }

  taskCardInColumn(title: string, stepId: string): Locator {
    return this.columnByStepId(stepId).locator('[data-testid^="task-card-"]', {
      has: this.page.locator('[data-testid="task-card-title"]', { hasText: title }),
    });
  }

  contextMoveTo(): Locator {
    return this.page.getByTestId("task-context-move-to");
  }

  contextSendToWorkflow(): Locator {
    return this.page.getByTestId("task-context-send-to-workflow");
  }

  contextWorkflow(workflowId: string): Locator {
    return this.page.getByTestId(`task-context-workflow-${workflowId}`);
  }

  contextStep(stepId: string): Locator {
    return this.page.getByTestId(`task-context-step-${stepId}`);
  }

  contextAutoStartStep(stepId: string): Locator {
    return this.page.getByTestId(`task-context-step-autostart-${stepId}`);
  }

  async openTaskContextMenu(taskId: string) {
    const card = this.taskCard(taskId);
    await card.waitFor({ state: "visible" });
    await card.click({ button: "right" });
  }

  async openTaskActionsMenu(taskId: string) {
    const card = this.taskCard(taskId);
    await card.waitFor({ state: "visible" });
    await card.getByLabel("More options").click();
  }

  async moveTaskWithinWorkflow(taskId: string, stepId: string) {
    await this.openTaskContextMenu(taskId);
    await this.contextMoveTo().hover();
    await this.contextStep(stepId).click();
  }

  async sendTaskToWorkflow(taskId: string, workflowId: string, stepId: string) {
    await this.openTaskContextMenu(taskId);
    await this.contextSendToWorkflow().hover();
    await this.contextWorkflow(workflowId).hover();
    await this.contextStep(stepId).click();
  }

  async sendTaskToWorkflowFromActions(taskId: string, workflowId: string, stepId: string) {
    await this.openTaskActionsMenu(taskId);
    await this.contextSendToWorkflow().hover();
    await this.contextWorkflow(workflowId).hover();
    await this.contextStep(stepId).click();
  }

  async enableMultiSelect() {
    await this.multiSelectToggle.first().waitFor({ state: "visible" });
    const isEnabled = await this.page
      .locator('[data-multi-select-active="true"]')
      .first()
      .isVisible();
    if (!isEnabled) {
      await this.multiSelectToggle.first().click();
    }
  }

  async selectTask(taskId: string) {
    const card = this.taskCard(taskId);
    await card.waitFor({ state: "visible" });
    await this.enableMultiSelect();
    await this.taskSelectCheckbox(taskId).click();
  }

  pipelineTask(taskId: string): Locator {
    return this.page.getByTestId(`pipeline-task-${taskId}`);
  }

  pipelineTaskRepoName(taskId: string): Locator {
    return this.page.getByTestId(`pipeline-task-repo-${taskId}`);
  }

  async switchToPipelineView() {
    await this.viewTogglePipeline.first().click();
    // Wait for a pipeline-specific element — swimlane-container is shared with the kanban view.
    await this.page.locator('[data-testid^="pipeline-task-"]').first().waitFor();
  }

  async selectPipelineTask(taskId: string) {
    const row = this.pipelineTask(taskId);
    await row.waitFor({ state: "visible" });
    await this.enableMultiSelect();
    await this.taskSelectCheckbox(taskId).click();
  }
}
