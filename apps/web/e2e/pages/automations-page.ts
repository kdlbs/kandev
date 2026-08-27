import { type Locator, type Page } from "@playwright/test";

export class AutomationsPage {
  readonly listPage: Locator;
  readonly newAutomationButton: Locator;
  readonly exportButton: Locator;
  readonly table: Locator;
  readonly emptyState: Locator;
  readonly editor: Locator;
  readonly nameInput: Locator;
  readonly saveButton: Locator;
  readonly deleteButton: Locator;
  readonly frequencySelector: Locator;
  readonly customScheduleInput: Locator;
  readonly timeInput: Locator;
  readonly timezoneButton: Locator;
  readonly nextRun: Locator;
  readonly addConditionButton: Locator;
  readonly workflowSelector: Locator;

  constructor(
    private page: Page,
    private workspaceId: string,
  ) {
    this.listPage = page.getByTestId("automations-list-page");
    this.newAutomationButton = page.getByTestId("new-automation-button");
    this.exportButton = page.getByTestId("export-automations-button");
    this.table = page.getByTestId("automations-table");
    this.emptyState = page.getByTestId("automations-empty");
    this.editor = page.getByTestId("automation-editor");
    this.nameInput = page.getByTestId("automation-name-input");
    this.saveButton = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: /save changes/i });
    this.deleteButton = page.getByTestId("automation-delete-button");
    this.frequencySelector = page.getByTestId("schedule-frequency");
    this.customScheduleInput = page.getByTestId("schedule-custom-input");
    this.timeInput = page.getByTestId("schedule-time");
    this.timezoneButton = page.getByTestId("schedule-timezone");
    this.nextRun = page.getByTestId("schedule-next-run");
    this.addConditionButton = page.getByTestId("add-condition-button");
    this.workflowSelector = page.getByTestId("workflow-selector-trigger");
  }

  async goto() {
    await this.page.goto(`/settings/workspaces/${this.workspaceId}/automations`);
    await this.listPage.waitFor({ state: "visible", timeout: 15_000 });
  }

  async gotoNew() {
    await this.page.goto(`/settings/workspaces/${this.workspaceId}/automations/new`);
    await this.editor.waitFor({ state: "visible", timeout: 15_000 });
  }

  automationRow(id: string): Locator {
    return this.page.getByTestId(`automation-row-${id}`);
  }

  /**
   * Open an automation from the listings by name.
   *
   * Clicks the name cell rather than the row, because a click on the row is
   * aimed at its centre — which lands on whichever column happens to be in the
   * middle, and the Enabled and actions cells deliberately stop propagation.
   * Targeting the name keeps this independent of the column layout.
   */
  async openByName(name: string) {
    await this.table.locator("tr", { hasText: name }).getByText(name, { exact: true }).click();
  }

  enabledSwitch(id: string): Locator {
    return this.page.getByTestId(`automation-enabled-${id}`);
  }

  /**
   * Pick a schedule frequency by its label, e.g. "every day" or
   * "a custom schedule". The detail controls that follow depend on the choice.
   */
  async selectFrequency(label: string) {
    await this.frequencySelector.click();
    await this.page.getByRole("option", { name: label, exact: true }).click();
  }

  /** Switch to the cron escape hatch and set an expression. */
  async setCustomSchedule(expression: string) {
    await this.selectFrequency("a custom schedule");
    await this.customScheduleInput.fill(expression);
    await this.customScheduleInput.blur();
  }

  /** Select a workflow by clicking the selector and picking an item by name. */
  async selectWorkflow(name: string) {
    await this.workflowSelector.click();
    await this.page.getByRole("button", { name: new RegExp(name) }).click();
  }
}
