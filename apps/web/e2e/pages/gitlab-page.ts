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

  // A single linked MR on desktop opens its detail panel directly on click
  // (no dropdown); a task with 2+ linked MRs, and touch/coarse-pointer
  // regardless of MR count, still show the click-driven per-MR dropdown with
  // a "Review !iid" menu item. Only click through the menu item when the
  // dropdown actually appears, so this works for both.
  async openLinkedMR(iid: number) {
    await this.page.getByTestId("mr-topbar-button").click();
    const reviewItem = this.page.getByRole("menuitem", { name: new RegExp(`Review .*\\!${iid}$`) });
    if (await reviewItem.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await reviewItem.click();
    }
    await expect(this.page.getByTestId("mr-detail-panel").last()).toBeVisible({ timeout: 15_000 });
  }

  // Same dual-path reasoning as openLinkedMR: touch/multi-MR unlink via the
  // dropdown's menu item; single-MR desktop unlink lives in the hover
  // popover's header icon button instead.
  async unlinkMR(iid: number) {
    const trigger = this.page.getByTestId("mr-topbar-button");
    await trigger.click();
    const unlinkMenuItem = this.page.getByRole("menuitem", { name: `Unlink !${iid}` });
    if (await unlinkMenuItem.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await unlinkMenuItem.click();
    } else {
      await trigger.hover();
      await this.page.getByRole("button", { name: `Unlink !${iid}` }).click();
    }
    await expect(this.page.getByTestId("mr-topbar-button")).toHaveCount(0);
  }
}
