import { type Locator, type Page } from "@playwright/test";

export class GitLabSettingsPage {
  readonly hostInput: Locator;
  readonly tokenInput: Locator;
  readonly reviewWatches: Locator;
  readonly issueWatches: Locator;

  constructor(private readonly page: Page) {
    this.hostInput = page.getByTestId("gitlab-host-input");
    this.tokenInput = page.getByTestId("gitlab-token-input");
    this.reviewWatches = page.getByTestId("gitlab-review-watches-card");
    this.issueWatches = page.getByTestId("gitlab-issue-watches-card");
  }

  async goto(workspaceId?: string) {
    const path = workspaceId
      ? `/settings/workspace/${workspaceId}/integrations/gitlab`
      : "/settings/integrations/gitlab";
    await this.page.goto(path);
    await this.hostInput.waitFor();
  }

  reviewSection(): Locator {
    return this.page
      .getByRole("heading", { name: "Merge request review watches" })
      .locator("xpath=ancestor::section");
  }

  issueSection(): Locator {
    return this.page
      .getByRole("heading", { name: "Issue watches" })
      .locator("xpath=ancestor::section");
  }
}
