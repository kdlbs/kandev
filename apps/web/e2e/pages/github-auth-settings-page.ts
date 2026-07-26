import type { Locator, Page } from "@playwright/test";

export class GitHubAuthSettingsPage {
  constructor(private readonly page: Page) {}

  async goto(workspaceId: string, query = "") {
    await this.page.goto(
      `/settings/workspace/${encodeURIComponent(workspaceId)}/integrations/github${query}`,
    );
  }

  automation() {
    return this.page.getByTestId("github-workspace-automation");
  }

  personalIdentity() {
    return this.page.getByTestId("github-personal-identity");
  }

  async openConnection() {
    await this.automation()
      .getByRole("button", { name: /^(Connect GitHub|Change connection)$/ })
      .click();
    return this.connectionSurface();
  }

  connectionSurface(): Locator {
    return this.page.locator(
      '[data-testid="github-connection-desktop"], [data-testid="github-connection-mobile"]',
    );
  }

  async chooseMethod(name: "Personal access token" | "GitHub CLI account" | "GitHub App") {
    const surface = this.connectionSurface();
    await surface.getByRole("radio", { name: new RegExp(`^${name}`) }).click();
    return surface;
  }

  async chooseApp(registrationId: string) {
    await this.connectionSurface().locator(`#github-app-${registrationId}`).click();
  }
}
