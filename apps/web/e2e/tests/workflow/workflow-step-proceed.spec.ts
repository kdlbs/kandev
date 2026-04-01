import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Manual proceed to next workflow step", () => {
  /**
   * Regression test: clicking "proceed to next step" from a plan-mode step
   * must disable plan mode and show the next step's auto-start prompt in chat.
   *
   * Workflow: Spec (no auto events) -> Work (auto_start_agent) -> Done
   *
   * 1. Create task via dialog in plan mode -> lands in Spec with plan layout
   * 2. Click "proceed to next step" button (Work)
   * 3. Assert: plan mode is disabled (plan panel hidden, default input placeholder)
   * 4. Assert: stepper shows Work as current step
   * 5. Assert: Work step's auto-start prompt response is visible in chat
   */
  test("proceed from plan-mode step disables plan mode and shows next step prompt", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Proceed Plan Step Workflow",
    );

    await apiClient.createWorkflowStep(workflow.id, "Spec", 0);
    const workStep = await apiClient.createWorkflowStep(workflow.id, "Work", 1);
    await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    // Work: auto-start agent with a delayed response so we can observe the message
    await apiClient.updateWorkflowStep(workStep.id, {
      prompt: 'e2e:delay(2000)\ne2e:message("work step response")\n{{task_prompt}}',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
      },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    // --- Create task via dialog in plan mode ---
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill title only -> "Start Plan Mode" becomes the default submit action
    await testPage.getByTestId("task-title-input").fill("Plan Proceed Task");

    const submitBtn = dialog.getByRole("button", { name: /Start Plan Mode/ });
    await expect(submitBtn).toBeEnabled({ timeout: 10_000 });
    await submitBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Navigates to session page with plan layout
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Plan panel should be visible (plan layout activated by dialog flow)
    await expect(session.planPanel).toBeVisible({ timeout: 15_000 });

    // Plan mode input placeholder confirms plan mode is on
    await expect(session.planModeInput()).toBeVisible({ timeout: 10_000 });

    // Stepper shows Spec as current step
    await expect(session.stepperStep("Spec")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // The "proceed to next step" button should be visible (showing "Work")
    const proceedBtn = session.proceedNextStepButton();
    await expect(proceedBtn).toBeVisible({ timeout: 10_000 });

    // --- Click proceed to move to Work step ---
    await proceedBtn.click();

    // Plan mode should be disabled: plan panel hidden and input shows default placeholder
    await expect(session.planPanel).not.toBeVisible({ timeout: 15_000 });
    await expect(session.planModeInput()).not.toBeVisible({ timeout: 10_000 });

    // Stepper shows Work as current step
    await expect(session.stepperStep("Work")).toHaveAttribute("aria-current", "step", {
      timeout: 15_000,
    });

    // Work step auto-start prompt should be visible in chat (user message bubble)
    await expect(
      session.chat.getByText("work step response", { exact: false }).first(),
    ).toBeVisible({ timeout: 30_000 });

    // Session returns to idle in default (non-plan) mode
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
  });
});
