import { type Locator, type Page } from "@playwright/test";

export class KanbanPage {
  readonly board: Locator;
  readonly createTaskButton: Locator;

  constructor(private page: Page) {
    this.board = page.getByTestId("kanban-board");
    this.createTaskButton = page.getByTestId("create-task-button");
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
      hasText: title,
    });
  }

  columnByStepId(stepId: string): Locator {
    return this.page.getByTestId(`kanban-column-${stepId}`);
  }

  taskCardInColumn(title: string, stepId: string): Locator {
    return this.columnByStepId(stepId).locator('[data-testid^="task-card-"]', {
      hasText: title,
    });
  }
}
