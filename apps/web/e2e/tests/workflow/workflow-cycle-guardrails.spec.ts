import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

const WARNING_WORKFLOW_NAME = "Guardrail warning workflow";

test.describe("Workflow cycle guardrails", () => {
  test("warns before persisting a repeated agent run and keeps its diagnostic after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, WARNING_WORKFLOW_NAME);
    const work = await apiClient.createWorkflowStep(workflow.id, "Work", 0, {
      is_start_step: true,
    });
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    await apiClient.updateWorkflowStep(work.id, {
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_next" }],
      },
    });

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard(WARNING_WORKFLOW_NAME);
    await settings.setTurnCompleteTransition(card, "Review", "Move to previous step");

    const dialog = settings.cycleGuardDialog;
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "Confirm workflow cycle" })).toBeVisible();
    const diagnostic = settings.cycleDiagnostic(dialog, work.id);
    await expect(diagnostic.getByText("Potential repeated agent run")).toBeVisible();
    const trace = diagnostic
      .getByRole("list", { name: "Replay path for Work" })
      .getByRole("listitem");
    await expect(trace).toHaveCount(2);
    await expect(trace.nth(0)).toContainText(/Work\s*on_turn_complete\s*move_to_next\s*Review/);
    await expect(trace.nth(1)).toContainText(/Review\s*on_turn_complete\s*move_to_previous\s*Work/);
    await expect(trace.nth(1).getByText("User action required")).toBeVisible();
    await expect(
      diagnostic.getByText(
        '"Work" has no step prompt, so re-entering it sends the task description.',
      ),
    ).toBeVisible();

    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).not.toBeVisible();
    let persistedSteps = await apiClient.listWorkflowSteps(workflow.id);
    expect(
      persistedSteps.steps.find((step) => step.id === review.id)?.events?.on_turn_complete,
    ).toBeFalsy();

    await settings.setTurnCompleteTransition(card, "Review", "Move to previous step");
    await dialog.getByRole("button", { name: "Apply anyway" }).click();
    await expect(dialog).not.toBeVisible();
    await expect
      .poll(async () => {
        persistedSteps = await apiClient.listWorkflowSteps(workflow.id);
        return persistedSteps.steps.find((step) => step.id === review.id)?.events?.on_turn_complete;
      })
      .toEqual([{ type: "move_to_previous" }]);

    await expect(settings.cycleDiagnostic(card, work.id)).toBeVisible();
    await expect(card.getByLabel("Work is part of a replay cycle")).toBeVisible();
    await expect(card.getByLabel("Review is part of a replay cycle")).toBeVisible();

    await settings.goto(seedData.workspaceId);
    const reloadedCard = await settings.findWorkflowCard(WARNING_WORKFLOW_NAME);
    await expect(settings.cycleDiagnostic(reloadedCard, work.id)).toBeVisible();
  });

  test("blocks creation of a draft with a fully automatic cycle", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.createWorkflow("Blocked automatic draft", "Custom");
    const card = await settings.findWorkflowCard("Blocked automatic draft");

    await settings.setAutoStart(card, "Todo", true);
    await settings.setTurnCompleteTransition(card, "Todo", "Move to next step");
    await settings.setAutoStart(card, "In Progress", true);
    await settings.setTurnCompleteTransition(card, "In Progress", "Move to previous step");
    await settings.saveButton(card).click();

    const dialog = settings.cycleGuardDialog;
    await expect(dialog.getByRole("heading", { name: "Workflow cycle blocked" })).toBeVisible();
    await expect(dialog.getByText("Automatic workflow cycle")).toHaveCount(2);
    await expect(dialog.getByRole("button", { name: /anyway/i })).toHaveCount(0);
    await expect(dialog.getByRole("button", { name: "Return to workflow" })).toBeVisible();
    await expect
      .poll(async () => (await apiClient.listWorkflows(seedData.workspaceId)).workflows)
      .not.toContainEqual(expect.objectContaining({ name: "Blocked automatic draft" }));
  });

  test("allows a generic cycle that cannot replay an auto-start step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflowName = "Allowed generic cycle";
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.createWorkflow(workflowName, "Custom");
    const card = await settings.findWorkflowCard(workflowName);

    await settings.setTurnCompleteTransition(card, "Todo", "Move to next step");
    await settings.setTurnCompleteTransition(card, "In Progress", "Move to previous step");
    await expect(card.locator('[data-testid^="workflow-cycle-diagnostic-"]')).toHaveCount(0);
    await settings.saveButton(card).click();

    await expect(settings.cycleGuardDialog).not.toBeVisible();
    await expect
      .poll(async () => (await apiClient.listWorkflows(seedData.workspaceId)).workflows)
      .toContainEqual(expect.objectContaining({ name: workflowName }));
  });
});
