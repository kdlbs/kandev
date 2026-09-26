import { type Locator, type Page } from "@playwright/test";

/** Page object for the unified AppSidebar; office nav (Agents/Projects) lives in collapsible sections that default closed on /office, so tests expand them via expandSection. */
export class AppSidebarPage {
  readonly root: Locator;

  constructor(private readonly page: Page) {
    this.root = page.getByTestId("app-sidebar");
  }

  /** Expand a collapsible section by label if collapsed. Idempotent. */
  async expandSection(label: string): Promise<void> {
    const header = this.root.getByRole("button", { name: label, exact: true }).first();
    await header.waitFor({ state: "visible", timeout: 10_000 });
    if ((await header.getAttribute("aria-expanded")) !== "true") {
      await header.click();
    }
  }

  /** Task row, scoped to this sidebar. `data-task-row-id` lives on the row itself. */
  row(taskId: string): Locator {
    return this.root.locator(`[data-task-row-id="${taskId}"]`);
  }

  /** MR badge for a task row, scoped to `app-sidebar` per the duplicate-mount rule. */
  mrBadge(taskId: string): Locator {
    return this.row(taskId).getByTestId(`mr-task-icon-${taskId}`);
  }

  /** PR badge for a task row, scoped to `app-sidebar` per the duplicate-mount rule. */
  prBadge(taskId: string): Locator {
    return this.row(taskId).getByTestId(`pr-task-icon-${taskId}`);
  }
}
