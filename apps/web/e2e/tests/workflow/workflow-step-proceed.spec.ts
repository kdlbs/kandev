import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Manual proceed to next workflow step", () => {
  /**
   * Regression test: moving a task out of a plan-mode step must disable plan mode
   * and show the next step's auto-start prompt in chat.
   *
   * Workflow: Spec (auto_start_agent) -> Work (auto_start_agent) -> Done
   *
   * 1. Create task via API in Spec step — agent auto-starts and completes
   * 2. Navigate to session page, manually enable plan mode
   * 3. Click "proceed to next step" button (Work)
   * 4. Assert: plan mode is disabled (plan panel hidden, default input placeholder)
   * 5. Assert: stepper shows Work as current step
   * 6. Assert: Work step's auto-start prompt response is visible in chat
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

    const specStep = await apiClient.createWorkflowStep(workflow.id, "Spec", 0);
    const workStep = await apiClient.createWorkflowStep(workflow.id, "Work", 1);
    await apiClient.createWorkflowStep(workflow.id, "Done", 2);

    // Spec: auto-start agent (completes quickly so we can proceed)
    await apiClient.updateWorkflowStep(specStep.id, {
      prompt: 'e2e:message("spec done")\n{{task_prompt}}',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
      },
    });

    // Work: auto-start agent with a response we can assert on
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

    // Create task via API in Spec step — triggers auto_start_agent
    await apiClient.createTask(seedData.workspaceId, "Plan Proceed Task", {
      workflow_id: workflow.id,
      workflow_step_id: specStep.id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    // Navigate to task session page
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("Plan Proceed Task", specStep.id);
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for agent to complete its turn (idle input visible)
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Stepper shows Spec as current step
    await expect(session.stepperStep("Spec")).toHaveAttribute("aria-current", "step", {
      timeout: 10_000,
    });

    // The "proceed to next step" button should be visible (showing "Work")
    const proceedBtn = session.proceedNextStepButton();
    await expect(proceedBtn).toBeVisible({ timeout: 10_000 });

    // --- Enable plan mode manually (simulates being in a plan-mode workflow step) ---
    await testPage.waitForTimeout(1_000);
    await session.togglePlanMode();
    await expect(session.planPanel).toBeVisible({ timeout: 15_000 });
    await expect(session.planModeInput()).toBeVisible({ timeout: 10_000 });

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

  /**
   * Regression test: the proceed button must re-enable after each step transition.
   * Previously, isMoving stayed true after clicking proceed because the button
   * reappeared (for the next step) before isMoving was reset.
   *
   * Workflow: Spec -> Work -> Review -> QA -> Done (all with auto_start_agent)
   */
  test("proceed button re-enables across multiple step transitions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Multi Step Proceed Workflow",
    );

    const stepNames = ["Spec", "Work", "Review", "QA", "Done"];
    const steps: { id: string; name: string }[] = [];
    for (let i = 0; i < stepNames.length; i++) {
      const step = await apiClient.createWorkflowStep(workflow.id, stepNames[i], i);
      steps.push({ id: step.id, name: stepNames[i] });
    }

    // Configure all steps except Done with auto_start_agent and a fast prompt
    for (const step of steps.slice(0, -1)) {
      await apiClient.updateWorkflowStep(step.id, {
        prompt: `e2e:message("${step.name} done")\n{{task_prompt}}`,
        events: {
          on_enter: [{ type: "auto_start_agent" }],
        },
      });
    }

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    // Create task in Spec step
    await apiClient.createTask(seedData.workspaceId, "Multi Step Task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[0].id,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardInColumn("Multi Step Task", steps[0].id);
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Walk through each step transition: Spec -> Work -> Review -> QA -> Done
    for (let i = 0; i < steps.length - 1; i++) {
      const currentName = steps[i].name;
      const nextName = steps[i + 1].name;

      // Wait for agent to complete in current step
      await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

      // Stepper shows current step
      await expect(session.stepperStep(currentName)).toHaveAttribute("aria-current", "step", {
        timeout: 10_000,
      });

      // Proceed button is visible AND enabled (not disabled by stale isMoving)
      const proceedBtn = session.proceedNextStepButton();
      await expect(proceedBtn).toBeVisible({ timeout: 10_000 });
      await expect(proceedBtn).toBeEnabled({ timeout: 5_000 });

      // Click proceed
      await proceedBtn.click();

      // Stepper updates to next step
      await expect(session.stepperStep(nextName)).toHaveAttribute("aria-current", "step", {
        timeout: 15_000,
      });
    }

    // On Done step: proceed button should NOT be visible (final step)
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.proceedNextStepButton()).not.toBeVisible({ timeout: 5_000 });
  });
});
