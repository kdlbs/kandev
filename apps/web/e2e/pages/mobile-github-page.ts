import { type Locator, type Page } from "@playwright/test";

export class MobileGitHubPage {
  readonly mobileMenuButton: Locator;
  readonly mobileSidebar: Locator;
  readonly inlineSidebar: Locator;
  readonly toolbarTitle: Locator;

  constructor(private page: Page) {
    this.mobileMenuButton = page.getByTestId("github-mobile-menu-button");
    this.mobileSidebar = page.getByTestId("github-mobile-sidebar");
    this.inlineSidebar = page.getByTestId("github-presets-sidebar-inline");
    this.toolbarTitle = page.locator("h2.text-sm.font-semibold").first();
  }

  async goto() {
    await this.page.goto("/github");
    // PageTopbar renders the "GitHub" breadcrumb — wait for it so the auth
    // status fetch has resolved and the actions slot is mounted.
    await this.page.getByText("GitHub", { exact: true }).first().waitFor({ state: "visible" });
  }

  presetByLabel(label: string): Locator {
    return this.mobileSidebar.getByRole("button", { name: label });
  }
}
