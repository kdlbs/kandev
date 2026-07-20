import { expect, type Locator, type Page } from "@playwright/test";

export class GitLabPage {
  readonly mobileFiltersButton: Locator;
  readonly mobileSidebar: Locator;

  constructor(private readonly page: Page) {
    this.mobileFiltersButton = page.getByTestId("gitlab-mobile-menu-button");
    this.mobileSidebar = page.getByTestId("gitlab-mobile-sidebar");
  }

  async goto() {
    await this.page.goto("/gitlab");
    await this.page.getByTestId("gitlab-list-toolbar-title").waitFor();
  }

  mrRow(iid: number): Locator {
    return this.page.locator(`[data-testid="mr-row"][data-mr-iid="${iid}"]`);
  }

  issueRow(iid: number): Locator {
    return this.page.locator(`[data-testid="issue-row"][data-issue-iid="${iid}"]`);
  }

  async startMRTask(iid: number, action = "Review") {
    const row = this.page.locator(`[data-testid="mr-row"][data-mr-iid="${iid}"]`);
    await row.getByRole("button", { name: "Create task" }).click();
    await this.page.getByRole("menuitem", { name: new RegExp(`^${action}\\b`, "i") }).click();
    const dialog = this.page.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("task-title-input")).toHaveValue(new RegExp(`^${action}:`));
    const start = dialog.getByTestId("submit-start-agent");
    await expect(start).toBeEnabled({ timeout: 30_000 });
    await start.click();
    await expect(dialog).toBeHidden();
    await expect(this.page).toHaveURL(/\/t\//);
  }

  async openLinkedMR(iid: number) {
    await this.page.getByTestId("mr-topbar-button").click();
    await this.page.getByRole("menuitem", { name: new RegExp(`Review .*\\!${iid}$`) }).click();
    await expect(this.page.getByTestId("mr-detail-panel").last()).toBeVisible();
  }

  async unlinkMR(iid: number) {
    await this.page.getByTestId("mr-topbar-button").click();
    await this.page.getByRole("menuitem", { name: `Unlink !${iid}` }).click();
    await expect(this.page.getByTestId("mr-topbar-button")).toHaveCount(0);
  }
}
