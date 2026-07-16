import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow settings", () => {
  test("hides system-only templates from the add workflow dialog", async ({
    testPage,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    await page.addWorkflowButton.click();
    await expect(page.createDialog).toBeVisible();
    await expect(page.createDialog.getByText("Office Default", { exact: true })).toHaveCount(0);
    await expect(page.createDialog.getByText("Routine", { exact: true })).toHaveCount(0);
  });

  test("displays existing workflows on the settings page", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // The seeded "E2E Workflow" should be visible
    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    // Should display workflow steps from the "simple" template
    for (const step of seedData.steps) {
      await expect(card.getByText(step.name)).toBeVisible();
    }
  });

  test("creates a workflow from template only after Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Create a new workflow from the first available template
    await page.createWorkflow("Template Test Workflow");

    // Verify the new card appears
    const card = await page.findWorkflowCard("Template Test Workflow");
    await expect(card).toBeVisible();
    await expect(card).toHaveAttribute("data-settings-dirty", "true");
    await expect(card.locator("input").first()).toHaveAttribute("data-settings-dirty", "true");
    await expect(page.stepNodeByName(card, "Todo")).toHaveAttribute("data-settings-dirty", "true");

    const beforeSave = await apiClient.listWorkflows(seedData.workspaceId);
    expect(
      beforeSave.workflows.some((workflow) => workflow.name === "Template Test Workflow"),
    ).toBe(false);
    await expect(page.floatingSave).toBeVisible();
    await page.saveChanges();

    const savedCard = await page.findWorkflowCard("Template Test Workflow");
    await expect(savedCard).toHaveAttribute("data-settings-dirty", "false");
    await expect(page.stepNodeByName(savedCard, "Todo")).toHaveAttribute(
      "data-settings-dirty",
      "false",
    );

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Template Test Workflow");
    await expect(reloadedCard).toBeVisible();
  });

  test("creates a custom workflow without template", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    await page.createWorkflow("Custom Test Workflow", "Custom");

    const card = await page.findWorkflowCard("Custom Test Workflow");
    await expect(card).toBeVisible();

    // Custom workflows get default steps (Todo, In Progress, Review, Done)
    await expect(card.getByText("Todo")).toBeVisible();
    await expect(card.getByText("In Progress")).toBeVisible();
    await expect(card.getByText("Review")).toBeVisible();
    await expect(card.getByText("Done")).toBeVisible();

    await page.saveChanges();
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Custom Test Workflow");
    await expect(reloadedCard).toBeVisible();
  });

  test("adds a step locally and persists it with Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Create workflow from template
    await page.createWorkflow("Step Add Test");

    const card = await page.findWorkflowCard("Step Add Test");
    await expect(card).toBeVisible();
    await page.saveChanges();

    const persistedCard = await page.findWorkflowCard("Step Add Test");
    const workflows = await apiClient.listWorkflows(seedData.workspaceId);
    const workflow = workflows.workflows.find((item) => item.name === "Step Add Test");
    expect(workflow).toBeDefined();
    const stepsBefore = await apiClient.listWorkflowSteps(workflow!.id);

    await page.addStepButton(persistedCard).click();

    await expect(persistedCard.getByText("New Step")).toBeVisible();
    expect((await apiClient.listWorkflowSteps(workflow!.id)).steps).toHaveLength(
      stepsBefore.steps.length,
    );

    await page.saveChanges();

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Step Add Test");
    await expect(reloadedCard).toBeVisible();
    await expect(reloadedCard.getByText("New Step")).toBeVisible();
  });

  test("configures an all child tasks complete transition", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Child Completion Settings",
    );
    const waitStep = await apiClient.createWorkflowStep(workflow.id, "Waiting for Children", 0);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "All Children Done", 1);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("Child Completion Settings");
    await expect(card).toBeVisible();
    await page.stepNodeByName(card, "Waiting for Children").click();

    await card.getByTestId(`${waitStep.id}-children-completed-help`).hover();
    await expect(
      testPage.getByText("When every active direct child task is COMPLETED, FAILED, or CANCELLED"),
    ).toBeVisible();

    await card.getByTestId(`${waitStep.id}-children-completed-transition-select`).click();
    await testPage.getByRole("option", { name: "Move to specific step" }).click();
    await expect(card.getByTestId(`${waitStep.id}-children-completed-step-select`)).toContainText(
      "All Children Done",
    );

    const beforeSave = await apiClient.listWorkflowSteps(workflow.id);
    expect(
      beforeSave.steps.find((step) => step.id === waitStep.id)?.events?.on_children_completed,
    ).toBeUndefined();

    await page.saveChanges();
    const afterSave = await apiClient.listWorkflowSteps(workflow.id);
    expect(
      afterSave.steps.find((step) => step.id === waitStep.id)?.events?.on_children_completed,
    ).toEqual([{ type: "move_to_step", config: { step_id: doneStep.id } }]);
  });

  test("configures WIP limit and feeder step", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "WIP Settings");
    const backlogStep = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0);
    const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 1);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("WIP Settings");
    await expect(card).toBeVisible();
    await page.stepNodeByName(card, "Review").click();

    await card.getByTestId(`${reviewStep.id}-wip-limit-input`).fill("2");
    await card.getByTestId(`${reviewStep.id}-pull-from-step-select`).click();
    await testPage.getByRole("option", { name: "Backlog" }).click();

    expect(
      (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
        (step) => step.id === reviewStep.id,
      ),
    ).not.toMatchObject({ wip_limit: 2, pull_from_step_id: backlogStep.id });

    await page.saveChanges();
    expect(
      (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
        (step) => step.id === reviewStep.id,
      ),
    ).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: backlogStep.id,
    });
  });

  test("modifies a step name only after Save", async ({ testPage, apiClient, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Use the first step name from seed data (same template)
    const firstStepName = seedData.steps[0]?.name;
    if (!firstStepName) {
      test.skip(true, "No template steps available");
      return;
    }

    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    // Click on the first step to open config panel
    const stepNode = page.stepNodeByName(card, firstStepName);
    await stepNode.click();

    // Find the step name input in the config panel and rename it
    const nameInput = card.getByPlaceholder("Step name");
    await nameInput.fill("Renamed Step");

    await expect(nameInput).toHaveAttribute("data-settings-dirty", "true");
    await expect(card).toHaveAttribute("data-settings-dirty", "true");
    await expect(card.getByTestId(`workflow-step-panel-${seedData.steps[0].id}`)).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
    await expect(card.getByTestId(`workflow-step-node-${seedData.steps[0].id}`)).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );

    expect((await apiClient.listWorkflowSteps(seedData.workflowId)).steps[0]?.name).toBe(
      firstStepName,
    );
    await page.saveChanges();

    await expect(nameInput).toHaveAttribute("data-settings-dirty", "false");
    await expect(card).toHaveAttribute("data-settings-dirty", "false");

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("E2E Workflow");
    await expect(reloadedCard).toBeVisible();
    await expect(reloadedCard.getByText("Renamed Step")).toBeVisible();
  });

  test("shows delete confirmation dialog when removing a persisted step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Delete Step Test");
    await apiClient.createWorkflowStep(workflow.id, "Keep", 0);
    await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("Delete Step Test");
    await expect(card).toBeVisible();

    // Try to delete the "Review" step via hovering and clicking trash
    await page.clickDeleteStepButton(card, "Review");

    // Confirmation dialog should appear
    await expect(page.stepDeleteDialog).toBeVisible();
    await expect(page.stepDeleteDialog.getByText("Review")).toBeVisible();

    // Cancel — step should still exist
    await page.stepDeleteDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(page.stepDeleteDialog).not.toBeVisible();
    await expect(card.getByText("Review")).toBeVisible();

    // Delete again and confirm
    await page.clickDeleteStepButton(card, "Review");
    await expect(page.stepDeleteDialog).toBeVisible();
    await page.stepDeleteDialog.getByRole("button", { name: "Delete Step", exact: true }).click();
    await expect(page.stepDeleteDialog).not.toBeVisible();

    // Step should be removed
    await expect(page.stepNodeByName(card, "Review")).not.toBeVisible();
  });

  test("deletes a workflow", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    await page.createWorkflow("To Delete Workflow", "Custom");
    await page.saveChanges();

    const card = await page.findWorkflowCard("To Delete Workflow");
    await expect(card).toBeVisible();

    // Click delete workflow
    await page.deleteWorkflowButton(card).click();

    // The delete dialog should appear — confirm deletion
    const deleteDialog = testPage.getByRole("dialog").filter({ hasText: "Delete" });
    await expect(deleteDialog).toBeVisible();
    // Click the delete button (it will say "Delete" or "Delete Workflow")
    await deleteDialog
      .getByRole("button", { name: /delete/i })
      .last()
      .click();

    // Workflow card should be removed
    const deletedCard = await page.findWorkflowCard("To Delete Workflow");
    await expect(deletedCard).not.toBeVisible();
  });

  test("keeps workflow details local until the route-level Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("E2E Workflow");
    const nameInput = card.locator("input").first();
    await nameInput.fill("Manually Saved Workflow Name");

    await expect(nameInput).toHaveAttribute("data-settings-dirty", "true");
    await expect(card).toHaveAttribute("data-settings-dirty", "true");

    expect(
      (await apiClient.listWorkflows(seedData.workspaceId)).workflows.find(
        (workflow) => workflow.id === seedData.workflowId,
      )?.name,
    ).toBe("E2E Workflow");
    await expect(page.floatingSave).toBeVisible();
    await page.saveChanges();

    await expect(nameInput).toHaveAttribute("data-settings-dirty", "false");
    await expect(card).toHaveAttribute("data-settings-dirty", "false");

    await page.goto(seedData.workspaceId);
    await expect(await page.findWorkflowCard("Manually Saved Workflow Name")).toBeVisible();
  });
});

test.describe("Seed protection", () => {
  // Backend restart can be flaky
  test.describe.configure({ retries: 1 });

  test("backend restart preserves user-customized workflows visible in UI", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // 1. Create workflows from templates via API
    const kanbanWf = await apiClient.createWorkflow(seedData.workspaceId, "My Kanban", "simple");
    const prReviewWf = await apiClient.createWorkflow(
      seedData.workspaceId,
      "My PR Review",
      "pr-review",
    );

    // 2. Customize via API — set custom prompts
    const { steps: kanbanSteps } = await apiClient.listWorkflowSteps(kanbanWf.id);
    const reviewStep = kanbanSteps.find((s) => s.name === "Review");
    expect(reviewStep).toBeDefined();
    await apiClient.updateWorkflowStep(reviewStep!.id, {
      prompt: "Custom QA review prompt",
    });

    const { steps: prSteps } = await apiClient.listWorkflowSteps(prReviewWf.id);
    const prReviewStep = prSteps.find((s) => s.name === "Review");
    expect(prReviewStep).toBeDefined();
    await apiClient.updateWorkflowStep(prReviewStep!.id, {
      prompt: "My custom PR review instructions",
    });

    // 3. Verify workflows are visible in UI before restart
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);
    await expect(await page.findWorkflowCard("My Kanban")).toBeVisible();
    await expect(await page.findWorkflowCard("My PR Review")).toBeVisible();

    // 4. Restart the backend — triggers seed/init again
    await backend.restart();

    // 5. Reload the page and verify workflows still visible with correct steps
    await page.goto(seedData.workspaceId);
    const kanbanCard = await page.findWorkflowCard("My Kanban");
    await expect(kanbanCard).toBeVisible();
    await expect(kanbanCard.getByText("Backlog")).toBeVisible();
    await expect(kanbanCard.getByText("Review")).toBeVisible();

    const prCard = await page.findWorkflowCard("My PR Review");
    await expect(prCard).toBeVisible();

    // 6. Verify customizations survived via API
    const { steps: postKanban } = await apiClient.listWorkflowSteps(kanbanWf.id);
    const postReview = postKanban.find((s) => s.id === reviewStep!.id);
    expect(postReview).toBeDefined();
    expect(postReview!.prompt).toBe("Custom QA review prompt");

    const { steps: postPR } = await apiClient.listWorkflowSteps(prReviewWf.id);
    const postPRReview = postPR.find((s) => s.id === prReviewStep!.id);
    expect(postPRReview).toBeDefined();
    expect(postPRReview!.prompt).toBe("My custom PR review instructions");

    // 7. Same number of steps (no duplication or loss)
    expect(postKanban).toHaveLength(kanbanSteps.length);
    expect(postPR).toHaveLength(prSteps.length);
  });

  test("hidden system workflows do not appear in the settings list", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Reproduces the original "Improve Kandev" leak: while the user is on
    // the workspace workflow settings page, a hidden system workflow gets
    // created (e.g. via the Improve Kandev dialog). The backend fires a
    // `workflow.created` WS event with hidden=true; the frontend receives
    // it and previously surfaced the entry as a manageable card in the
    // settings list. Verify the new hidden entry never appears as a card.
    const hiddenName = "Improve Kandev";

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // The seeded visible workflow is rendered before the leak attempt.
    await expect
      .poll(async () => (await page.findWorkflowCard("E2E Workflow")).isVisible())
      .toBe(true);
    const visibleCard = await page.findWorkflowCard("E2E Workflow");
    await expect(visibleCard).toBeVisible();
    const baselineCount = await testPage.locator('[data-testid^="workflow-card-"]').count();

    // Trigger the leak path: a hidden workflow is created and the
    // `workflow.created` WS event arrives at the open settings page.
    await apiClient.e2eCreateHiddenWorkflow(seedData.workspaceId, hiddenName);

    // Allow the WS event to propagate and the React effect in
    // useWorkflowSettings a chance to (incorrectly) add a card.
    await testPage.waitForTimeout(500);

    // No new card appeared and the hidden entry is not in the list.
    const allCards = testPage.locator('[data-testid^="workflow-card-"]');
    const newCount = await allCards.count();
    const cardNames: string[] = [];
    for (let i = 0; i < newCount; i++) {
      const value = await allCards
        .nth(i)
        .locator("input")
        .first()
        .inputValue({ timeout: 500 })
        .catch(() => "");
      cardNames.push(value);
    }
    expect(cardNames).not.toContain(hiddenName);
    expect(newCount).toBe(baselineCount);
  });
});
