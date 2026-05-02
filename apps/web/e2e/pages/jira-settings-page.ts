import { type Locator, type Page } from "@playwright/test";

export class JiraSettingsPage {
  readonly siteInput: Locator;
  readonly projectInput: Locator;
  readonly emailInput: Locator;
  readonly secretInput: Locator;
  readonly testButton: Locator;
  readonly saveButton: Locator;
  readonly deleteButton: Locator;
  readonly statusBanner: Locator;

  constructor(private page: Page) {
    this.siteInput = page.getByTestId("jira-site-input");
    this.projectInput = page.getByTestId("jira-project-input");
    this.emailInput = page.getByTestId("jira-email-input");
    this.secretInput = page.getByTestId("jira-secret-input");
    this.testButton = page.getByTestId("jira-test-button");
    this.saveButton = page.getByTestId("jira-save-button");
    this.deleteButton = page.getByTestId("jira-delete-button");
    this.statusBanner = page.getByTestId("integration-auth-status-banner");
  }

  async goto() {
    await this.page.goto(`/settings/integrations/jira`);
    await this.siteInput.waitFor({ state: "visible" });
  }

  async fillForm(args: { siteUrl: string; email?: string; secret: string; projectKey?: string }) {
    await this.siteInput.fill(args.siteUrl);
    if (args.email !== undefined) await this.emailInput.fill(args.email);
    await this.secretInput.fill(args.secret);
    if (args.projectKey) await this.projectInput.fill(args.projectKey);
  }
}
