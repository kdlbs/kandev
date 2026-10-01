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
    await this.openSelector(trigger);
    const option = this.option(workflowId);
    await expect(option).toBeVisible();
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
    await this.openSelector(trigger);
    const option = this.option(stepId);
    await expect(option).toBeVisible();
    if (this.mobile) await option.tap();
    else await option.click();
  }

  async chooseProfile(sourceProfileId: string, replacementProfileName: string) {
    const trigger = this.form.getByTestId(`change-workflow-profile-selector-${sourceProfileId}`);
    await this.openSelector(trigger);
    const option = this.page.getByRole("option").filter({ hasText: replacementProfileName }).last();
    await expect(option).toBeVisible();
    if (this.mobile) await option.tap();
    else await option.click();
  }

  async expectStepOptionColor(stepId: string, cssColor: string) {
    await this.openSelector(this.form.getByTestId("change-workflow-step"));
    await this.expectColorDot(this.option(stepId), cssColor);
  }

  async expectSelectedStepColor(cssColor: string) {
    await this.expectColorDot(this.form.getByTestId("change-workflow-step"), cssColor);
  }

  private async expectColorDot(container: Locator, cssColor: string) {
    const expectedColor = await this.page.evaluate((color) => {
      const reference = document.createElement("span");
      reference.style.backgroundColor = color;
      document.body.append(reference);
      const resolved = getComputedStyle(reference).backgroundColor;
      reference.remove();
      return resolved;
    }, cssColor);
    expect(expectedColor).not.toBe("rgba(0, 0, 0, 0)");
    const dot = container.locator(".rounded-full");
    await expect(dot).toBeVisible();
    await expect(dot).toHaveCSS("background-color", expectedColor);
  }

  private async openSelector(trigger: Locator) {
    await expect(trigger).toBeVisible();
    if ((await trigger.getAttribute("aria-expanded")) !== "true") {
      if (this.mobile) await trigger.tap();
      else await trigger.click();
    }
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
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
