import { type Locator, type Page } from "@playwright/test";

export class MobileGitHubPage {
  readonly mobileMenuButton: Locator;
  readonly mobileSidebar: Locator;
  readonly inlineSidebar: Locator;
  readonly toolbarTitle: Locator;
  readonly repoFilterTrigger: Locator;
  readonly repoSearchInput: Locator;
  readonly saveQueryRepoTrigger: Locator;
  readonly saveQueryRepoDropdown: Locator;

  constructor(private page: Page) {
    this.mobileMenuButton = page.getByTestId("github-mobile-menu-button");
    this.mobileSidebar = page.getByTestId("github-mobile-sidebar");
    // Desktop scope bar (replaces the old inline presets rail). Hidden on
    // mobile, where the presets live in the hamburger sheet instead.
    this.inlineSidebar = page.getByTestId("github-presets-scope-bar");
    this.toolbarTitle = page.getByTestId("github-list-toolbar-title");
    this.repoFilterTrigger = page.getByTestId("github-repo-filter-trigger");
    this.repoSearchInput = page.getByTestId("github-repo-filter-dropdown").getByRole("combobox");
    this.saveQueryRepoTrigger = page.getByTestId("github-save-query-repo-trigger");
    this.saveQueryRepoDropdown = page.getByTestId("github-save-query-repo-dropdown");
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

  issueRowByTitle(title: string): Locator {
    return this.page.getByTestId("issue-row").filter({ hasText: title });
  }

  savedQueryByLabel(label: string): Locator {
    return this.mobileSidebar.getByText(label, { exact: true });
  }
}
