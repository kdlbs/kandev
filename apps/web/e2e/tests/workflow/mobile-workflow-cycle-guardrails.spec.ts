import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow cycle guardrails on mobile", () => {
  test("reviews and confirms a repeated agent run by touch without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflowName = "Mobile warning draft";
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.createWorkflowByTouch(workflowName, "Custom");
    const card = await settings.findWorkflowCard(workflowName);

    await settings.setAutoStart(card, "Todo", true, true);
    await settings.setTurnCompleteTransition(card, "Todo", "Move to next step", true);
    await settings.setTurnCompleteTransition(card, "In Progress", "Move to previous step", true);
    await settings.saveButton(card).tap();

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
