import { type Locator, type Page } from "@playwright/test";

export class MobileGitHubPage {
  readonly mobileMenuButton: Locator;
  readonly mobileSidebar: Locator;
  readonly inlineSidebar: Locator;
  readonly toolbarTitle: Locator;

  constructor(private page: Page) {
    this.mobileMenuButton = page.getByTestId("github-mobile-menu-button");
    this.mobileSidebar = page.getByTestId("github-mobile-sidebar");
    // Desktop scope bar (replaces the old inline presets rail). Hidden on
    // mobile, where the presets live in the hamburger sheet instead.
    this.inlineSidebar = page.getByTestId("github-presets-scope-bar");
    this.toolbarTitle = page.getByTestId("github-list-toolbar-title");
  }

  async goto() {
    await this.page.goto("/github");
    // The hamburger only mounts once auth has resolved on a mobile viewport —
    // waiting on it is deterministic and avoids the ambiguity of matching
    // "GitHub" text (which appears in the breadcrumb AND breadcrumb link aria).
    await this.mobileMenuButton.waitFor({ state: "visible" });
  }

  presetByLabel(label: string): Locator {
    return this.mobileSidebar.getByRole("button", { name: label });
  }
}
