import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow settings", () => {
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

  test("creates a workflow from template and persists after save", async ({
    testPage,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Create a new workflow from the first available template
    await page.createWorkflow("Template Test Workflow");

    // Verify the new card appears
    const card = await page.findWorkflowCard("Template Test Workflow");
    await expect(card).toBeVisible();

    // Save the workflow
    await page.saveButton(card).click();

    // Wait for save to complete
    await testPage.waitForTimeout(1000);

    // Reload and verify persistence
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

    // Save
    await page.saveButton(card).click();
    await testPage.waitForTimeout(1000);

    // Reload and verify
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Custom Test Workflow");
    await expect(reloadedCard).toBeVisible();
  });

  test("adds a step to template workflow and persists after save", async ({
    testPage,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Create workflow from template
    await page.createWorkflow("Step Add Test");

    const card = await page.findWorkflowCard("Step Add Test");
    await expect(card).toBeVisible();

    // Click the add step button
    await page.addStepButton(card).click();

    // A "New Step" should appear
    await expect(card.getByText("New Step")).toBeVisible();

    // Save the workflow
    await page.saveButton(card).click();
    await testPage.waitForTimeout(1000);

    // Reload and verify the extra step persists
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Step Add Test");
    await expect(reloadedCard).toBeVisible();
    await expect(reloadedCard.getByText("New Step")).toBeVisible();
  });

  test("modifies a template step name and persists after save", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Use the first step name from seed data (same template)
    const firstStepName = seedData.steps[0]?.name;
    if (!firstStepName) {
      test.skip(true, "No template steps available");
      return;
    }

    // Create workflow from template
    await page.createWorkflow("Step Edit Test");

    const card = await page.findWorkflowCard("Step Edit Test");
    await expect(card).toBeVisible();

    // Click on the first step to open config panel
    const stepNode = page.stepNodeByName(card, firstStepName);
    await stepNode.click();

    // Find the step name input in the config panel and rename it
    const nameInput = card.getByPlaceholder("Step name");
    await nameInput.clear();
    await nameInput.fill("Renamed Step");

    // Wait for debounced name update to propagate to state (500ms debounce)
    await testPage.waitForTimeout(600);

    // Save the workflow
    await page.saveButton(card).click();
    await testPage.waitForTimeout(1000);

    // Reload and verify the renamed step persists
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Step Edit Test");
    await expect(reloadedCard).toBeVisible();
    await expect(reloadedCard.getByText("Renamed Step")).toBeVisible();
  });

  test("shows delete confirmation dialog when removing a step", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Create a custom workflow so we don't affect the seeded one
    await page.createWorkflow("Delete Step Test", "Custom");

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
    await page.stepDeleteDialog.getByRole("button", { name: "Delete" }).click();
    await expect(page.stepDeleteDialog).not.toBeVisible();

    // Step should be removed
    await expect(page.stepNodeByName(card, "Review")).not.toBeVisible();
  });

  test("deletes a workflow", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Create and save a workflow first
    await page.createWorkflow("To Delete Workflow", "Custom");

    const card = await page.findWorkflowCard("To Delete Workflow");
    await expect(card).toBeVisible();

    // Save it first so it's persisted
    await page.saveButton(card).click();
    await testPage.waitForTimeout(1000);

    // Reload to get the real workflow card
    await page.goto(seedData.workspaceId);
    const savedCard = await page.findWorkflowCard("To Delete Workflow");
    await expect(savedCard).toBeVisible();

    // Click delete workflow
    await page.deleteWorkflowButton(savedCard).click();

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
});
