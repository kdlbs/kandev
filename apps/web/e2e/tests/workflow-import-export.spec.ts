import { test, expect } from "../fixtures/test-base";
import { WorkflowSettingsPage } from "../pages/workflow-settings-page";

test.describe("Workflow import/export", () => {
  test("export all button opens dialog with valid YAML", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Click "Export All" button
    await testPage.getByRole("button", { name: "Export All" }).click();

    // Export dialog should appear with YAML content
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Export Workflows")).toBeVisible();

    // Textarea should contain valid YAML with the workflow
    const textarea = dialog.locator("textarea");
    const yamlContent = await textarea.inputValue();
    expect(yamlContent).toContain("version: 1");
    expect(yamlContent).toContain("type: kandev_workflow");
    expect(yamlContent).toContain("E2E Workflow");

    // Copy button should work
    await dialog.getByRole("button", { name: "Copy" }).click();
    await expect(dialog.getByRole("button", { name: "Copied" })).toBeVisible();

    // Close
    await dialog.getByRole("button", { name: "Close" }).first().click();
    await expect(dialog).not.toBeVisible();
  });

  test("per-workflow export button shows that workflow's YAML", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Find the workflow card and click its Export button
    const card = await page.findWorkflowCard("E2E Workflow");
    await card.getByRole("button", { name: "Export" }).click();

    // Dialog should show YAML with step names from the Kanban template
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const yamlContent = await dialog.locator("textarea").inputValue();
    expect(yamlContent).toContain("E2E Workflow");
    expect(yamlContent).toContain("Backlog");
    expect(yamlContent).toContain("In Progress");
    expect(yamlContent).toContain("Review");
    expect(yamlContent).toContain("Done");

    await dialog.getByRole("button", { name: "Close" }).first().click();
  });

  test("import via paste creates workflow and shows on page", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Click Import button
    await testPage.getByRole("button", { name: "Import" }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Import Workflows")).toBeVisible();

    // Paste YAML into the textarea
    const yamlContent = `version: 1
type: kandev_workflow
workflows:
  - name: Pasted Workflow
    steps:
      - name: Open
        position: 0
        color: bg-blue-500
        is_start_step: true
        allow_manual_move: true
      - name: Closed
        position: 1
        color: bg-green-500
        allow_manual_move: true`;

    await dialog.locator("textarea").fill(yamlContent);

    // Click Import button in dialog
    await dialog.getByRole("button", { name: "Import" }).click();

    // Dialog should close and toast should appear
    await expect(dialog).not.toBeVisible();

    // Reload to see the imported workflow
    await page.goto(seedData.workspaceId);
    const card = await page.findWorkflowCard("Pasted Workflow");
    await expect(card).toBeVisible();
    await expect(card.getByText("Open")).toBeVisible();
    await expect(card.getByText("Closed")).toBeVisible();
  });

  test("import shows skip message for duplicate workflow name", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    await testPage.getByRole("button", { name: "Import" }).click();
    const dialog = testPage.getByRole("dialog");

    // Try to import a workflow with the same name as the existing one
    const yamlContent = `version: 1
type: kandev_workflow
workflows:
  - name: E2E Workflow
    steps:
      - name: Step1
        position: 0
        color: bg-neutral-400
        is_start_step: true
        allow_manual_move: true`;

    await dialog.locator("textarea").fill(yamlContent);
    await dialog.getByRole("button", { name: "Import" }).click();

    // Should show a toast mentioning "Skipped" since the workflow already exists
    await expect(testPage.getByText("Skipped", { exact: false })).toBeVisible({ timeout: 5000 });
  });

  test("round-trip: export workflow, delete, re-import preserves structure", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Seed a workflow with custom prompt via API
    const wf = await apiClient.createWorkflow(seedData.workspaceId, "Roundtrip WF", "standard");
    const { steps } = await apiClient.listWorkflowSteps(wf.id);
    const planStep = steps.find((s) => s.name === "Plan");
    if (planStep) {
      await apiClient.updateWorkflowStep(planStep.id, {
        prompt: "Custom roundtrip prompt",
      });
    }

    // Navigate to settings and export the workflow via UI
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("Roundtrip WF");
    await card.getByRole("button", { name: "Export" }).click();

    const dialog = testPage.getByRole("dialog");
    const exportedYaml = await dialog.locator("textarea").inputValue();
    expect(exportedYaml).toContain("Roundtrip WF");
    expect(exportedYaml).toContain("Custom roundtrip prompt");
    await dialog.getByRole("button", { name: "Close" }).first().click();

    // Delete the workflow via API
    await apiClient.deleteWorkflow(wf.id);

    // Re-import via UI
    await page.goto(seedData.workspaceId);
    await testPage.getByRole("button", { name: "Import" }).click();
    const importDialog = testPage.getByRole("dialog");
    await importDialog.locator("textarea").fill(exportedYaml);
    await importDialog.getByRole("button", { name: "Import" }).click();
    await expect(importDialog).not.toBeVisible();

    // Verify the workflow is back with its structure
    await page.goto(seedData.workspaceId);
    const reimportedCard = await page.findWorkflowCard("Roundtrip WF");
    await expect(reimportedCard).toBeVisible();
    await expect(reimportedCard.getByText("Plan")).toBeVisible();
    await expect(reimportedCard.getByText("Implementation")).toBeVisible();
    await expect(reimportedCard.getByText("Done")).toBeVisible();

    // Verify custom prompt survived via API
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const reimported = workflows.find((w) => w.name === "Roundtrip WF");
    const { steps: reimportedSteps } = await apiClient.listWorkflowSteps(reimported!.id);
    const reimportedPlan = reimportedSteps.find((s) => s.name === "Plan");
    expect(reimportedPlan?.prompt).toContain("Custom roundtrip prompt");
  });

  test("import rejects invalid YAML with error toast", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    await testPage.getByRole("button", { name: "Import" }).click();
    const dialog = testPage.getByRole("dialog");

    await dialog.locator("textarea").fill("this: is: not: [valid yaml");
    await dialog.getByRole("button", { name: "Import" }).click();

    // Should show error toast
    await expect(testPage.getByText("Failed to import", { exact: false })).toBeVisible({
      timeout: 5000,
    });
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
