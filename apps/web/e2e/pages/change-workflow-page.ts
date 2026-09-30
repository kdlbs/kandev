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

  async expectStepColors(steps: Array<{ id: string; color: string }>) {
    const trigger = this.form.getByTestId("change-workflow-step");
    const backgrounds: string[] = [];
    for (const step of steps) {
      if (this.mobile) await trigger.tap();
      else await trigger.click();
      const list = this.page.locator('[role="listbox"]:visible');
      const option = list.locator(`[role="option"][data-value="${step.id}"]`);
      const dot = option.locator("span.rounded-full");
      await expect(dot).toBeVisible();
      if (step.color.startsWith("bg-")) {
        await expect(dot).toHaveClass(new RegExp(step.color));
      } else {
        const expected = await this.page.evaluate((color) => {
          const element = document.createElement("span");
          element.style.backgroundColor = color;
          document.body.append(element);
          const background = getComputedStyle(element).backgroundColor;
          element.remove();
          return background;
        }, step.color);
        await expect(dot).toHaveCSS("background-color", expected);
      }
      await expect(dot).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
      await expect(dot).toHaveAttribute("aria-hidden", "true");
      const background = await dot.evaluate((element) => getComputedStyle(element).backgroundColor);
      backgrounds.push(background);
      if (this.mobile) await option.tap();
      else await option.click();
      await expect(list).toBeHidden();
      await expect(trigger.locator("span.rounded-full")).toHaveCSS("background-color", background);
    }
    expect(new Set(backgrounds).size).toBe(steps.length);
  }

  async captureStepPicker(path: string) {
    const trigger = this.form.getByTestId("change-workflow-step");
    if (this.mobile) await trigger.tap();
    else await trigger.click();
    const list = this.page.locator('[role="listbox"]:visible');
    await expect(list).toBeVisible();
    await list.evaluate(async () => {
      await Promise.all(
        document
          .getAnimations()
          .filter((animation) => Number.isFinite(animation.effect?.getComputedTiming().iterations))
          .map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    await this.page.screenshot({ path });
    const selected = list.locator('[role="option"][aria-selected="true"]');
    if (this.mobile) await selected.tap();
    else await selected.click();
    await expect(list).toBeHidden();
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
