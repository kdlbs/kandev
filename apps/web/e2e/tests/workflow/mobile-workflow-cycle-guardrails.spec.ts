import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow cycle guardrails on mobile", () => {
  test("keeps a blocking cycle readable above the dialog action", async ({
    testPage,
    seedData,
  }) => {
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.createWorkflow("Mobile blocked draft", "Custom", true);
    const card = await settings.findWorkflowCard("Mobile blocked draft");

    await settings.setAutoStart(card, "Todo", true, true);
    await settings.setTurnCompleteTransition(card, "Todo", "Move to next step", true);
    await settings.setAutoStart(card, "In Progress", true, true);
    await settings.setTurnCompleteTransition(card, "In Progress", "Move to previous step", true);
    await settings.submitSaveChanges(true);

    const dialog = settings.cycleGuardDialog;
    await expect(dialog.getByRole("heading", { name: "Workflow cycle blocked" })).toBeVisible();
    await expect(dialog.getByText("Automatic workflow cycle")).toHaveCount(2);

    const transitionLabels = dialog.getByText(/^(On turn complete|Move to (next|previous) step)$/);
    await expect(transitionLabels).toHaveCount(8);
    const labelSizes = await transitionLabels.evaluateAll((labels) =>
      labels.map((label) => {
        const box = label.getBoundingClientRect();
        return { width: box.width, height: box.height };
      }),
    );
    for (const label of labelSizes) expect(label.width).toBeGreaterThan(label.height);

    const finalExplanation = dialog
      .getByText('"In Progress" has no step prompt, so re-entering it sends the task description.')
      .last();
    const returnButton = dialog.getByRole("button", { name: "Return to workflow" });
    await finalExplanation.scrollIntoViewIfNeeded();
    const [explanationBox, buttonBox] = await Promise.all([
      finalExplanation.boundingBox(),
      returnButton.boundingBox(),
    ]);
    expect(explanationBox).not.toBeNull();
    expect(buttonBox).not.toBeNull();
    expect(explanationBox!.y + explanationBox!.height).toBeLessThanOrEqual(buttonBox!.y);

    await returnButton.tap();
    await expect(dialog).not.toBeVisible();
  });

  test("reviews and confirms a repeated agent run by touch without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflowName = "Mobile warning draft";
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.createWorkflow(workflowName, "Custom", true);
    const card = await settings.findWorkflowCard(workflowName);

    await settings.setAutoStart(card, "Todo", true, true);
    await settings.setTurnCompleteTransition(card, "Todo", "Move to next step", true);
    await settings.setTurnCompleteTransition(card, "In Progress", "Move to previous step", true);
    await settings.submitSaveChanges(true);

    const dialog = settings.cycleGuardDialog;
    await expect(dialog.getByRole("heading", { name: "Confirm workflow cycle" })).toBeVisible();
    await expect(dialog.getByText("Potential repeated agent run")).toBeVisible();
    await expect(
      dialog.getByRole("list", { name: "Replay path for Todo" }).getByRole("listitem"),
    ).toHaveCount(2);
    await expect(
      dialog.getByText('"Todo" has no step prompt, so re-entering it sends the task description.'),
    ).toBeVisible();

    const actionSizes = await dialog.getByRole("button").evaluateAll((buttons) =>
      buttons.map((button) => ({
        name: button.textContent?.trim(),
        height: button.getBoundingClientRect().height,
      })),
    );
    expect(
      actionSizes.filter((action) => ["Cancel", "Create anyway"].includes(action.name ?? "")),
    ).toEqual([
      expect.objectContaining({ name: "Cancel", height: expect.any(Number) }),
      expect.objectContaining({ name: "Create anyway", height: expect.any(Number) }),
    ]);
    for (const action of actionSizes.filter((item) =>
      ["Cancel", "Create anyway"].includes(item.name ?? ""),
    )) {
      expect(action.height).toBeGreaterThanOrEqual(44);
    }

    const overflow = await testPage.evaluate(() => {
      const guard = document.querySelector<HTMLElement>(
        '[data-testid="workflow-cycle-guard-dialog"]',
      );
      return {
        document: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        dialog: guard ? guard.scrollWidth - guard.clientWidth : Number.POSITIVE_INFINITY,
      };
    });
    expect(overflow.document).toBeLessThanOrEqual(1);
    expect(overflow.dialog).toBeLessThanOrEqual(1);

    await dialog.getByRole("button", { name: "Create anyway" }).tap();
    await expect(dialog).not.toBeVisible();
    await expect
      .poll(async () => (await apiClient.listWorkflows(seedData.workspaceId)).workflows)
      .toContainEqual(expect.objectContaining({ name: workflowName }));
  });
});
