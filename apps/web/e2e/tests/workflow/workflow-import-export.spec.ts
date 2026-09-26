import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

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
    expect(yamlContent).toContain("version: 2");
    expect(yamlContent).toContain("type: kandev_workflow");
    expect(yamlContent).toContain("E2E Workflow");
    expect(yamlContent).toContain("complete_task_on_enter: false");

    // Copy button should work
    await dialog.getByRole("button", { name: "Copy" }).click();
    await expect(dialog.getByRole("button", { name: "Copied" })).toBeVisible();

    // Close
    await dialog.getByRole("button", { name: "Close" }).first().click();
    await expect(dialog).not.toBeVisible();
  });

  test("export all excludes office-style workflows", async ({ testPage, apiClient, seedData }) => {
    // Workflow settings is kanban-only by design (ADR-0004). Seed an
    // office-style workflow alongside the kanban "E2E Workflow" and confirm
    // Export All omits it (regression for issue #1109, where Export All
    // dumped every workflow — office triggers would silently drop on import).
    await apiClient.seedWorkflow(seedData.workspaceId, "Office Only Workflow", "office");

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // The office workflow must not even appear in the kanban settings list.
    await expect(testPage.getByText("Office Only Workflow")).toHaveCount(0);

    await testPage.getByRole("button", { name: "Export All" }).click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const yamlContent = await dialog.locator("textarea").inputValue();
    expect(yamlContent).toContain("E2E Workflow");
    expect(yamlContent).not.toContain("Office Only Workflow");

    await dialog.getByRole("button", { name: "Close" }).first().click();
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

    // The import refreshes the workflow list in place. Wait for its new name
    // instead of starting a competing navigation while that refresh settles.
    const card = await page.findWorkflowCard("Pasted Workflow", { waitForName: true });
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

  test("import resolves a missing step profile and preserves a later session target", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const replacement = await apiClient.getAgentProfile(seedData.agentProfileId);
    const workflowName = `Profile import ${Date.now()}`;
    const yamlContent = `version: 2
type: kandev_workflow
workflows:
  - name: ${workflowName}
    steps:
      - name: Start
        position: 0
        color: bg-neutral-400
        events: {}
        is_start_step: true
        show_in_command_panel: true
        allow_manual_move: true
        complete_task_on_enter: false
        auto_advance_requires_signal: false
        cancel_triggers_turn_complete: false
      - name: Implement
        position: 1
        color: bg-blue-500
        events: {}
        is_start_step: false
        show_in_command_panel: true
        allow_manual_move: true
        agent_profile:
          agent_name: Missing import agent
          model: missing-import-model
          mode: missing-import-mode
        complete_task_on_enter: false
        auto_advance_requires_signal: false
        cancel_triggers_turn_complete: false
      - name: Review
        position: 2
        color: bg-yellow-500
        events: {}
        is_start_step: false
        show_in_command_panel: true
        allow_manual_move: true
        session_target:
          kind: step
          step_position: 1
        complete_task_on_enter: false
        auto_advance_requires_signal: false
        cancel_triggers_turn_complete: false`;

    try {
      const page = new WorkflowSettingsPage(testPage);
      await page.goto(seedData.workspaceId);
      await testPage.getByRole("button", { name: "Import", exact: true }).click();
      const dialog = testPage.getByRole("dialog");
      const inputGeometry = await dialog.evaluate((surface) => {
        const upload = surface.querySelector('input[type="file"]')!;
        return {
          width: surface.getBoundingClientRect().width,
          fileButtonHeight: parseFloat(getComputedStyle(upload, "::file-selector-button").height),
        };
      });
      expect(inputGeometry.width).toBeGreaterThanOrEqual(720);
      expect(inputGeometry.fileButtonHeight).toBeCloseTo(28, 0);
      await dialog.locator("textarea").fill(yamlContent);
      await dialog.getByRole("button", { name: "Import", exact: true }).click();

      const selection = testPage.getByTestId("workflow-import-profile-selection");
      await expect(selection).toBeVisible();
      await expect(selection.getByText("Implement", { exact: true })).toBeVisible();
      await expect(testPage.getByPlaceholder("Search profiles")).not.toBeVisible();

      await selection.getByTestId("workflow-import-profile-select-0:1").click();
      await testPage.getByTestId(`workflow-import-profile-option-${replacement.id}`).click();
      const selectedTrigger = selection.getByTestId("workflow-import-profile-select-0:1");
      await expect(selectedTrigger).toContainText(replacement.name);
      const geometry = await selectedTrigger.evaluate((button) => {
        const label = button.querySelector("span");
        const bounds = button.getBoundingClientRect();
        const text = label?.getBoundingClientRect();
        return {
          height: bounds.height,
          top: bounds.top,
          bottom: bounds.bottom,
          textTop: text?.top ?? 0,
          textBottom: text?.bottom ?? 0,
        };
      });
      expect(geometry.height).toBeCloseTo(28, 0);
      expect(geometry.textTop).toBeGreaterThanOrEqual(geometry.top);
      expect(geometry.textBottom).toBeLessThanOrEqual(geometry.bottom);
      await selection.getByTestId("workflow-import-profile-submit").click();
      await expect(selection).not.toBeVisible();

      let importedId: string | undefined;
      await expect
        .poll(
          async () => {
            const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
            importedId = workflows.find((workflow) => workflow.name === workflowName)?.id;
            return importedId;
          },
          { timeout: 10_000 },
        )
        .toBeDefined();

      const { steps } = await apiClient.listWorkflowSteps(importedId!);
      const implement = steps.find((step) => step.name === "Implement");
      const review = steps.find((step) => step.name === "Review");
      expect(implement?.agent_profile_id).toBe(replacement.id);
      expect(review?.session_target).toEqual({ kind: "step", step_id: implement?.id });

      const importedCard = await page.findWorkflowCard(workflowName, { waitForName: true });
      await expect(importedCard).toBeVisible();
    } finally {
      const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
      const imported = workflows.find((workflow) => workflow.name === workflowName);
      if (imported) await apiClient.deleteWorkflow(imported.id).catch(() => {});
    }
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
