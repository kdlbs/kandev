import { type Locator, type Page } from "@playwright/test";

export class AutomationsPage {
  readonly listPage: Locator;
  readonly newAutomationButton: Locator;
  readonly table: Locator;
  readonly emptyState: Locator;
  readonly editor: Locator;
  readonly nameInput: Locator;
  readonly saveButton: Locator;
  readonly deleteButton: Locator;
  readonly customScheduleInput: Locator;
  readonly addConditionButton: Locator;
  readonly workflowSelector: Locator;
  readonly workflowStepSelector: Locator;

  constructor(
    private page: Page,
    private workspaceId: string,
  ) {
    this.listPage = page.getByTestId("automations-list-page");
    this.newAutomationButton = page.getByTestId("new-automation-button");
    this.table = page.getByTestId("automations-table");
    this.emptyState = page.getByTestId("automations-empty");
    this.editor = page.getByTestId("automation-editor");
    this.nameInput = page.getByTestId("automation-name-input");
    this.saveButton = page.getByTestId("automation-save-button");
    this.deleteButton = page.getByTestId("automation-delete-button");
    this.customScheduleInput = page.getByTestId("schedule-custom-input");
    this.addConditionButton = page.getByTestId("add-condition-button");
    this.workflowSelector = page.getByTestId("workflow-selector");
    this.workflowStepSelector = page.getByTestId("workflow-step-selector");
  }

  async goto() {
    await this.page.goto(`/settings/workspace/${this.workspaceId}/automations`);
    await this.listPage.waitFor({ state: "visible", timeout: 15_000 });
  }

  async gotoNew() {
    await this.page.goto(`/settings/workspace/${this.workspaceId}/automations/new`);
    await this.editor.waitFor({ state: "visible", timeout: 15_000 });
  }

  automationRow(id: string): Locator {
    return this.page.getByTestId(`automation-row-${id}`);
  }

  enabledSwitch(id: string): Locator {
    return this.page.getByTestId(`automation-enabled-${id}`);
  }

  schedulePreset(expression: string): Locator {
    return this.page.getByTestId(`schedule-preset-${expression}`);
  }

  /** Select a workflow by clicking the selector and picking an item by name. */
  async selectWorkflow(name: string) {
    await this.workflowSelector.click();
    await this.page.getByRole("option", { name }).click();
  }

  /** Select a workflow step by clicking the selector and picking an item by name. */
  async selectWorkflowStep(name: string) {
    await this.workflowStepSelector.click();
    await this.page.getByRole("option", { name }).click();
  }
}
