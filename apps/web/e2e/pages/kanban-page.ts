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
      has: this.page.locator('[data-testid="task-card-title"]', { hasText: title }),
    });
  }

  columnByStepId(stepId: string): Locator {
    return this.page.getByTestId(`kanban-column-${stepId}`);
  }

  taskCardInColumn(title: string, stepId: string): Locator {
    return this.columnByStepId(stepId).locator('[data-testid^="task-card-"]', {
      has: this.page.locator('[data-testid="task-card-title"]', { hasText: title }),
    });
  }

  laneMenuTrigger(stepId: string): Locator {
    return this.page.getByTestId(`lane-menu-trigger-${stepId}`);
  }

  async openLaneMenu(stepId: string): Promise<void> {
    await this.columnByStepId(stepId).hover();
    await this.laneMenuTrigger(stepId).click();
  }

  laneMenuMoveAll(): Locator {
    return this.page.getByTestId("lane-menu-move-all");
  }

  laneMenuMoveToStep(stepId: string): Locator {
    return this.page.getByTestId(`lane-menu-move-to-${stepId}`);
  }

  laneMenuArchiveAll(): Locator {
    return this.page.getByTestId("lane-menu-archive-all");
  }

  laneMenuClear(): Locator {
    return this.page.getByTestId("lane-menu-clear");
  }

  laneConfirmArchive(): Locator {
    return this.page.getByTestId("lane-confirm-archive");
  }

  laneConfirmClear(): Locator {
    return this.page.getByTestId("lane-confirm-clear");
  }
}
