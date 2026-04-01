import { type Locator, type Page, expect } from "@playwright/test";

export class WorkflowSettingsPage {
  readonly page: Page;
  readonly addWorkflowButton: Locator;
  readonly createDialog: Locator;
  readonly workflowNameInput: Locator;
  readonly confirmCreateButton: Locator;

  constructor(page: Page) {
    this.page = page;
    this.addWorkflowButton = page.getByTestId("add-workflow-button");
    this.createDialog = page.getByTestId("create-workflow-dialog");
    this.workflowNameInput = page.getByTestId("workflow-name-input");
    this.confirmCreateButton = page.getByTestId("confirm-create-workflow");
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
    const count = await cards.count();
    for (let i = 0; i < count; i++) {
      const card = cards.nth(i);
      const input = card.locator("input").first();
      const value = await input.inputValue();
      if (value === name) {
        // Return a stable locator using the specific data-testid, not positional nth()
        const testId = await card.getAttribute("data-testid");
        if (testId) return this.page.getByTestId(testId);
        return card;
      }
    }
    // Return a locator that won't match so assertions can fail with a good message
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
  async createWorkflow(name: string, templateName?: string) {
    await this.addWorkflowButton.click();
    await expect(this.createDialog).toBeVisible();

    if (name) {
      await this.workflowNameInput.fill(name);
    }

    if (templateName === "Custom") {
      await this.createDialog.locator('label[for="custom"]').click();
    } else if (templateName) {
      await this.createDialog.getByText(templateName, { exact: false }).first().click();
    }

    await this.confirmCreateButton.click();
    await expect(this.createDialog).not.toBeVisible();
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
}
