import { type Locator, type Page, expect } from "@playwright/test";

export class WorkflowSettingsPage {
  readonly page: Page;
  readonly addWorkflowButton: Locator;
  readonly createDialog: Locator;
  readonly workflowNameInput: Locator;
  readonly confirmCreateButton: Locator;
  readonly cycleGuardDialog: Locator;

  constructor(page: Page) {
    this.page = page;
    this.addWorkflowButton = page.getByTestId("add-workflow-button");
    this.createDialog = page.getByTestId("create-workflow-dialog");
    this.workflowNameInput = page.getByTestId("workflow-name-input");
    this.confirmCreateButton = page.getByTestId("confirm-create-workflow");
    this.cycleGuardDialog = page.getByTestId("workflow-cycle-guard-dialog");
  }

  async goto(workspaceId: string) {
    await this.page.goto(`/settings/workspace/${workspaceId}/workflows`);
    // Wait for a client-rendered element to confirm hydration is complete
    // (networkidle is unreliable with persistent WebSocket connections)
    await expect(this.addWorkflowButton).toBeVisible();
  }

  /** Returns the card container for a workflow by matching text in the card's name input. */
  workflowCard(workflowId: string): Locator {
    return this.page.getByTestId(`workflow-card-${workflowId}`);
  }

  /** Find a workflow card by the name shown in its input field using its current value. */
  async findWorkflowCard(name: string): Promise<Locator> {
    const cards = this.page.locator('[data-testid^="workflow-card-"]');
    await expect(cards.first()).toBeVisible();

    const testIds = await cards.evaluateAll((elements) =>
      elements
        .map((element) => element.getAttribute("data-testid"))
        .filter((testId): testId is string => Boolean(testId)),
    );

    for (const testId of testIds) {
      const card = this.page.getByTestId(testId);
      const input = card.locator("input").first();
      const value = await input.inputValue({ timeout: 500 }).catch(() => null);
      if (value === name) {
        return card;
      }
    }

    return this.page.getByTestId(`workflow-card-not-found-${name}`);
  }

  /** The pipeline step nodes within a specific workflow card. */
  stepNodes(card: Locator): Locator {
    return card.locator('[data-slot="alert-dialog-trigger"], .group.relative').filter({
      has: this.page.locator(".rounded-full"),
    });
  }

  /** Find a step node by its name text within a card. */
  stepNodeByName(card: Locator, stepName: string): Locator {
    return card.locator(".group.relative").filter({ hasText: stepName });
  }

  /** A replay-cycle diagnostic rendered inside a workflow card or guard dialog. */
  cycleDiagnostic(container: Locator, autoStartStepId: string): Locator {
    return container.getByTestId(`workflow-cycle-diagnostic-${autoStartStepId}`);
  }

  /** Select a step and return its configuration panel. */
  async selectStep(card: Locator, stepName: string, touch = false): Promise<Locator> {
    const currentName = card.getByPlaceholder("Step name");
    const alreadySelected =
      (await currentName.isVisible().catch(() => false)) &&
      (await currentName.inputValue().catch(() => "")) === stepName;
    if (!alreadySelected) {
      await this.activate(this.stepNodeByName(card, stepName), touch);
    }
    await expect(currentName).toHaveValue(stepName);
    return currentName.locator(
      "xpath=ancestor::div[contains(concat(' ', normalize-space(@class), ' '), ' rounded-lg ')][1]",
    );
  }

  /** Toggle auto-start for a step through the visible configuration panel. */
  async setAutoStart(card: Locator, stepName: string, enabled: boolean, touch = false) {
    const panel = await this.selectStep(card, stepName, touch);
    const checkbox = panel.getByRole("checkbox", { name: "Auto-start agent" });
    if ((await checkbox.isChecked()) !== enabled) {
      await this.activate(checkbox, touch);
    }
    if (enabled) await expect(checkbox).toBeChecked();
    else await expect(checkbox).not.toBeChecked();
  }

  /** Set the On Turn Complete transition in a step's configuration panel. */
  async setTurnCompleteTransition(
    card: Locator,
    stepName: string,
    optionName: string,
    touch = false,
  ) {
    const panel = await this.selectStep(card, stepName, touch);
    const transitionSection = panel
      .getByText("On Turn Complete", { exact: true })
      .locator("xpath=../..");
    await this.activate(transitionSection.getByRole("combobox").first(), touch);
    await this.activate(this.page.getByRole("option", { name: optionName }), touch);
  }

  /** The add-step (+) button within a workflow card. */
  addStepButton(card: Locator): Locator {
    return card.getByTestId("add-step-button");
  }

  /** The save button within a workflow card (matches button text "Save"). */
  saveButton(card: Locator): Locator {
    return card.getByRole("button", { name: "Save" });
  }

  /** The delete workflow button within a card. */
  deleteWorkflowButton(card: Locator): Locator {
    return card.getByTestId("delete-workflow-button");
  }

  /** The step delete confirmation dialog. */
  get stepDeleteDialog(): Locator {
    return this.page.getByTestId("step-delete-confirm-dialog");
  }

  /** Returns the ordered names of all workflow cards on the page. */
  async getWorkflowOrder(): Promise<string[]> {
    const cards = this.page.locator('[data-testid^="workflow-card-"]');
    const count = await cards.count();
    const names: string[] = [];
    for (let i = 0; i < count; i++) {
      const input = cards.nth(i).locator("input").first();
      names.push(await input.inputValue());
    }
    return names;
  }

  /** The drag handle for a specific workflow card. */
  dragHandle(workflowId: string): Locator {
    return this.page.getByTestId(`workflow-drag-handle-${workflowId}`);
  }

  /** Open the "Add Workflow" dialog and create a workflow. */
  async createWorkflow(name: string, templateName?: string, touch = false) {
    await this.activate(this.addWorkflowButton, touch);
    await expect(this.createDialog).toBeVisible();

    if (name) {
      if (touch) await this.workflowNameInput.tap();
      await this.workflowNameInput.fill(name);
    }

    if (templateName === "Custom") {
      await this.activate(this.createDialog.locator('label[for="custom"]'), touch);
    } else if (templateName) {
      await this.activate(
        this.createDialog.getByText(templateName, { exact: false }).first(),
        touch,
      );
    }

    await this.activate(this.confirmCreateButton, touch);
    await expect(this.createDialog).not.toBeVisible();
  }

  /** The workflow-level agent profile select trigger within a workflow card. */
  workflowAgentProfileSelect(card: Locator): Locator {
    return card.getByTestId("workflow-agent-profile-select");
  }

  /** The step agent profile override select trigger in the step config panel within a workflow card. */
  stepAgentProfileSelect(card: Locator): Locator {
    return card.getByTestId("step-agent-profile-select");
  }

  /** Hover over a step node to reveal the trash button, then click it. */
  async clickDeleteStepButton(card: Locator, stepName: string) {
    const node = this.stepNodeByName(card, stepName);
    await node.hover();
    await node
      .locator("button")
      .filter({ has: this.page.locator(".tabler-icon-trash") })
      .click();
  }

  private async activate(locator: Locator, touch: boolean) {
    if (touch) await locator.tap();
    else await locator.click();
  }
}
