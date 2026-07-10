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
}
