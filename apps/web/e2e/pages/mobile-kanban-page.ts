import { type Locator, type Page } from "@playwright/test";

export class MobileKanbanPage {
  readonly board: Locator;
  readonly mobileFab: Locator;
  readonly mobileSearchBar: Locator;
  readonly mobileSearchToggle: Locator;
  readonly mobileMenuButton: Locator;
  readonly mobileTaskSheet: Locator;
  readonly swimlaneContainer: Locator;
  readonly workflowTrigger: Locator;
  readonly stepTrigger: Locator;

  constructor(private page: Page) {
    this.board = page.getByTestId("kanban-board");
    this.mobileFab = page.getByTestId("mobile-fab");
    this.mobileSearchBar = page.getByTestId("mobile-search-bar");
    this.mobileSearchToggle = page.getByTestId("mobile-search-toggle");
    this.mobileMenuButton = page.getByRole("button", { name: "Open menu" });
    this.mobileTaskSheet = page.getByTestId("mobile-task-sheet");
    this.swimlaneContainer = page.getByTestId("swimlane-container");
    this.workflowTrigger = page.getByTestId("mobile-workflow-trigger");
    this.stepTrigger = page.getByTestId("mobile-step-trigger");
  }

  async goto() {
    await this.page.goto("/");
    await this.board.waitFor({ state: "visible" });
    // Wait for mobile-specific layout to render
    await this.page.getByTestId("mobile-kanban-layout").waitFor({ state: "visible" });
  }

  mobileKanbanLayout(): Locator {
    return this.page.getByTestId("mobile-kanban-layout");
  }

  columnTab(name: string): Locator {
    return this.page.getByRole("button", { name });
  }

  workflowItem(workflowId: string): Locator {
    return this.page.getByTestId(`mobile-workflow-item-${workflowId}`);
  }

  taskCard(taskId: string): Locator {
    return this.page.getByTestId(`task-card-${taskId}`);
  }

  taskCardByTitle(title: string): Locator {
    return this.board.locator(`[data-testid^="task-card-"]`, {
      has: this.page.locator('[data-testid="task-card-title"]', { hasText: title }),
    });
  }

  searchInput(): Locator {
    return this.mobileSearchBar.getByPlaceholder("Search tasks...");
  }

  async openSearch() {
    await this.mobileSearchToggle.click();
    await this.mobileSearchBar.waitFor({ state: "visible" });
  }

  sheetGoToSession(): Locator {
    return this.mobileTaskSheet.getByRole("button", { name: /Open Session/ });
  }

  sheetEditButton(): Locator {
    return this.mobileTaskSheet.getByRole("button", { name: /Edit/ });
  }

  sheetDeleteButton(): Locator {
    return this.mobileTaskSheet.getByRole("button", { name: /Delete/ });
  }
}
