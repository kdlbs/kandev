import { expect, type Locator, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "../helpers/animations";

export type SelectorFieldStyle = {
  borderWidth: number;
  borderStyle: string;
  borderAlpha: number;
  backgroundColor: string;
  backgroundAlpha: number;
  boxShadow: string;
  x: number;
  y: number;
  width: number;
  height: number;
};

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
    await expect(option).toBeHidden();
    await expect(trigger).toBeFocused();
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
    await expect(option).toBeHidden();
    await expect(trigger).toBeFocused();
  }

  async chooseProfile(sourceProfileId: string, replacementProfileName: string) {
    const trigger = this.form.getByTestId(`change-workflow-profile-selector-${sourceProfileId}`);
    await this.openSelector(trigger);
    const option = this.page.getByRole("option").filter({ hasText: replacementProfileName }).last();
    await expect(option).toBeVisible();
    if (this.mobile) await option.tap();
    else await option.click();
    await expect(option).toBeHidden();
    await expect(trigger).toBeFocused();
  }

  async expectStepOptionColor(stepId: string, cssColor: string) {
    await this.openSelector(this.form.getByTestId("change-workflow-step"));
    await this.expectColorDot(this.option(stepId), cssColor);
  }

  async expectSelectedStepColor(cssColor: string) {
    await this.expectColorDot(this.form.getByTestId("change-workflow-step"), cssColor);
  }

  selectorField(testId: string) {
    return this.form.getByTestId(testId);
  }

  async tabToSelectorField(testId: string, maxTabs = 6) {
    const trigger = this.selectorField(testId);
    for (let attempt = 0; attempt < maxTabs; attempt += 1) {
      if (await trigger.evaluate((element) => element === document.activeElement)) return;
      await this.page.keyboard.press("Tab");
    }
    await expect(trigger).toBeFocused();
  }

  async readSelectorField(testId: string): Promise<SelectorFieldStyle> {
    const trigger = this.selectorField(testId);
    await expect(trigger).toBeVisible();
    return trigger.evaluate((element) => {
      const style = getComputedStyle(element);
      const rect = element.getBoundingClientRect();
      const alpha = (color: string) => {
        const canvas = document.createElement("canvas");
        canvas.width = 1;
        canvas.height = 1;
        const context = canvas.getContext("2d");
        if (!context) return 0;
        context.fillStyle = color;
        context.fillRect(0, 0, 1, 1);
        return context.getImageData(0, 0, 1, 1).data[3] ?? 0;
      };
      return {
        borderWidth: Number.parseFloat(style.borderTopWidth),
        borderStyle: style.borderTopStyle,
        borderAlpha: alpha(style.borderTopColor),
        backgroundColor: style.backgroundColor,
        backgroundAlpha: alpha(style.backgroundColor),
        boxShadow: style.boxShadow,
        x: rect.x,
        y: rect.y,
        width: rect.width,
        height: rect.height,
      };
    });
  }

  expectVisibleSelectorField(style: SelectorFieldStyle, label: string, opaqueBackground = true) {
    expect(style.borderWidth, `${label} should render a field border`).toBeGreaterThanOrEqual(1);
    expect(style.borderStyle, `${label} border style`).toBe("solid");
    expect(style.borderAlpha, `${label} border should be visible`).toBeGreaterThan(0);
    expect(style.backgroundAlpha, `${label} should paint an input surface`).toBeGreaterThan(0);
    if (opaqueBackground) {
      expect(style.backgroundAlpha, `${label} should paint an opaque input surface`).toBe(255);
    }
  }

  expectStableSelectorBounds(before: SelectorFieldStyle, after: SelectorFieldStyle, label: string) {
    expect(after.width, `${label} width should not shift`).toBeCloseTo(before.width, 0);
    expect(after.height, `${label} height should not shift`).toBeCloseTo(before.height, 0);
    expect(after.x, `${label} x should not shift`).toBeCloseTo(before.x, 0);
    expect(after.y, `${label} y should not shift`).toBeCloseTo(before.y, 0);
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
    if (this.mobile) await waitForFiniteAnimations(this.phoneDrawer);
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
