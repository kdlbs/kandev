import { expect, type Locator, type Page } from "@playwright/test";

export class LayoutSettingsPage {
  readonly root: Locator;
  readonly editor: Locator;
  readonly actions: Locator;

  constructor(private readonly page: Page) {
    this.root = page.getByTestId("layout-settings");
    this.editor = page.getByTestId("layout-editor");
    this.actions = page.getByTestId("layout-editor-context-actions");
  }

  async open(): Promise<void> {
    await this.page.goto("/settings/general/layouts");
    await expect(this.root).toBeVisible();
  }

  async openFromMobileMenu(): Promise<void> {
    await this.page.goto("/settings/general/terminal");
    await this.page.getByTestId("settings-mobile-menu-button").click();
    const menu = this.page.getByTestId("settings-mobile-menu");
    await expect(menu).toBeVisible();
    await menu.getByRole("link", { name: "Layouts", exact: true }).click();
    await expect(this.page).toHaveURL(/\/settings\/general\/layouts$/);
    await expect(this.root).toBeVisible();
  }

  async duplicateDefault(name: string): Promise<void> {
    await this.page.getByTestId("layout-profile-built-in-default").click();
    await this.page.getByTestId("layout-profile-duplicate").click();
    const nameInput = this.page.getByRole("textbox", { name: "Layout profile name" });
    await expect(nameInput).toBeVisible();
    await nameInput.fill(name);
    await expect(this.actions).toBeVisible();
  }

  async selectPanel(name: string): Promise<void> {
    await this.editor.locator(".dv-tab", { hasText: name }).click();
    await expect(this.actions).toHaveAccessibleName(`Actions for ${name}`);
  }

  async removePanel(name: string): Promise<void> {
    await this.selectPanel(name);
    const button = this.actions.getByRole("button", { name: "Remove panel" });
    await expect(button).toBeEnabled();
    await button.click();
    await expect(
      this.page
        .getByTestId("layout-profile-built-in-default")
        .getByText("Customized", { exact: true }),
    ).toBeVisible();
  }

  async renameSelected(name: string): Promise<void> {
    const input = this.page.getByRole("textbox", { name: "Layout profile name" });
    await expect(input).toBeVisible();
    await input.fill(name);
  }

  async moveSelectedTabRight(): Promise<void> {
    const button = this.actions.getByRole("button", { name: "Move tab right" });
    await expect(button).toBeEnabled();
    await button.click();
  }

  async save(): Promise<void> {
    const response = this.page.waitForResponse(
      (candidate) =>
        candidate.url().includes("/api/v1/user/settings") &&
        candidate.request().method() === "PATCH",
    );
    await this.page.getByRole("button", { name: "Save changes" }).click();
    expect((await response).ok()).toBe(true);
  }
}
