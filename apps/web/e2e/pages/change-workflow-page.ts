import { expect, type Locator, type Page } from "@playwright/test";

export class ChangeWorkflowPage {
  readonly form: Locator;
  readonly desktopDialog: Locator;
  readonly phoneDrawer: Locator;

  constructor(
    readonly page: Page,
    readonly mobile = false,
  ) {
    this.form = page.getByTestId("change-workflow-form");
    this.desktopDialog = page.getByTestId("change-workflow-dialog");
    this.phoneDrawer = page.getByTestId("change-workflow-drawer");
  }

  async chooseWorkflow(workflowId: string) {
    const trigger = this.form.getByTestId("change-workflow-destination");
    await expect(trigger).toBeVisible();
    if (this.mobile) await trigger.tap();
    else await trigger.click();
    const option = this.option(workflowId);
    if (this.mobile) await option.tap();
    else await option.click();
    await expect
      .poll(async () => {
        const stepPicker = this.form.getByTestId("change-workflow-step");
        const noSteps = this.form.getByTestId("change-workflow-no-steps");
        return (await stepPicker.isVisible()) || (await noSteps.isVisible());
      })
      .toBe(true);
  }

  async chooseStep(stepId: string) {
    const trigger = this.form.getByTestId("change-workflow-step");
    await expect(trigger).toBeEnabled();
    if (this.mobile) await trigger.tap();
    else await trigger.click();
    const option = this.option(stepId);
    if (this.mobile) await option.tap();
    else await option.click();
  }

  async chooseProfile(sourceProfileId: string, replacementProfileName: string) {
    const trigger = this.form.getByTestId(`change-workflow-profile-selector-${sourceProfileId}`);
    await expect(trigger).toBeVisible();
    if (this.mobile) await trigger.tap();
    else await trigger.click();
    const option = this.page.getByRole("option").filter({ hasText: replacementProfileName }).last();
    if (this.mobile) await option.tap();
    else await option.click();
  }

  async submit() {
    const button = this.form.getByTestId("change-workflow-submit");
    await expect(button).toBeEnabled();
    if (this.mobile) await button.tap();
    else await button.click();
  }

  private option(value: string) {
    return this.page.locator(`[role="option"][data-value="${value}"]`);
  }
}
