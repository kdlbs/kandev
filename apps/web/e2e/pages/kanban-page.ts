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

  constructor(private page: Page) {
    this.board = page.getByTestId("kanban-board");
    this.createTaskButton = page.getByTestId("create-task-button");
    this.multiSelectToolbar = page.getByTestId("multi-select-toolbar");
    this.bulkDeleteButton = page.getByTestId("bulk-delete-button");
    this.bulkArchiveButton = page.getByTestId("bulk-archive-button");
    this.bulkMoveButton = page.getByTestId("bulk-move-button");
    this.bulkClearButton = page.getByTestId("bulk-clear-selection");
    this.bulkDeleteConfirm = page.getByTestId("bulk-delete-confirm");
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

  async selectTask(taskId: string) {
    const card = this.taskCard(taskId);
    await card.waitFor({ state: "visible" });
    await card.hover();
    await this.taskSelectCheckbox(taskId).click();
  }
}
