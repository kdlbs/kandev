import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage, openStepSection } from "../../pages/workflow-settings-page";

test.describe("Inline workflow editor", () => {
  test("summarizes the step and expands independent sections with inline actions", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Step summaries");
    const step = await apiClient.createWorkflowStep(workflow.id, "Work", 0, {
      is_start_step: true,
      events: { on_enter: [{ type: "run_script", config: { command: "echo ready" } }] },
    });
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard("Step summaries");
    await settings.stepNodeByName(card, "Work").click();
    const panel = card.getByTestId(`workflow-step-panel-${step.id}`);
    await expect(panel.locator("nav")).toHaveCount(0);
    for (const section of ["agent", "instructions", "automation", "board", "advanced"]) {
      await expect(panel.getByTestId(`workflow-section-toggle-${section}`)).toBeVisible();
    }
    await panel.scrollIntoViewIfNeeded();
    await prCapture.screenshot("desktop-step-summary-rows", {
      caption: "Five compact expandable summaries in the existing step editor.",
    });
    const agent = panel.getByTestId("workflow-section-toggle-agent");
    const automation = panel.getByTestId("workflow-section-toggle-automation");
    await expect(automation).toContainText("1 action");
    await agent.click();
    await automation.click();
    await expect(agent).toHaveAttribute("aria-expanded", "true");
    await expect(automation).toHaveAttribute("aria-expanded", "true");
    await panel.getByRole("checkbox", { name: "Auto-start agent", exact: true }).check();
    await expect(agent).toContainText("Auto-start agent");
    await expect(automation).toContainText("2 actions");
    await settings
      .editorActionList("on_enter")
      .getByRole("button", { name: /select action 1/i })
      .click();
    await settings.editorScript("on_enter").locator("textarea").fill("echo changed");
    await expect(settings.editorActionList("on_turn_complete")).toBeVisible();
    await expect(settings.editorActionList("on_enter")).toContainText("echo changed");
    const moveButton = panel.getByRole("button", { name: "Move action down", exact: true });
    const moveBox = await moveButton.boundingBox();
    expect(moveBox!.height).toBe(28);
    expect(moveBox!.width).toBe(28);
    await prCapture.screenshot("desktop-inline-script-editor", {
      caption: "Script editing keeps the surrounding event actions visible.",
    });
    await automation.click();
    await expect(agent).toHaveAttribute("aria-expanded", "true");
    await settings.saveChanges();
    await testPage.reload();
    const reloadedCard = await settings.findWorkflowCard("Step summaries");
    await settings.selectStep(reloadedCard, "Work");
    await reloadedCard.getByTestId("workflow-section-toggle-automation").click();
    await expect(settings.editorActionList("on_enter")).toContainText("echo changed");
  });
  test("starts new workflow creation in the existing workflow card", async ({
    testPage,
    seedData,
  }) => {
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.createWorkflow("Inline Workflow", "Custom");

    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspaces/${seedData.workspaceId}/workflows(?:\\?|$)`),
    );
    const card = settings.editor;
    await expect(card).toBeVisible();
    await expect(card.locator("input").first()).toHaveValue("Inline Workflow");
    await settings.selectStep(card, "Todo");
    await expect(card.getByTestId("workflow-editor-inspector")).toBeVisible();
    await expect(card.getByTestId("workflow-section-toggle-agent")).toBeVisible();
  });

  test("authors a lifecycle script and retains the draft across pipeline selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Focused Editor Desktop");
    const first = await apiClient.createWorkflowStep(workflow.id, "Draft step", 0, {
      is_start_step: true,
    });
    await apiClient.createWorkflowStep(workflow.id, "Review step", 1);

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard("Focused Editor Desktop");
    await settings.selectStep(card, "Draft step");
    let panel = card.getByTestId(`workflow-step-panel-${first.id}`);

    await openStepSection(panel, "automation", false);
    const enterActions = settings.editorActionList("on_enter");
    await enterActions.locator("select").selectOption("run_script");

    const scriptEditor = settings.editorScript("on_enter");
    await expect(scriptEditor).toBeVisible();
    await scriptEditor.locator("textarea").fill("printf 'focused editor\\n'");
    await settings.selectStep(card, "Review step");
    await settings.selectStep(card, "Draft step");
    panel = card.getByTestId(`workflow-step-panel-${first.id}`);
    await openStepSection(panel, "automation", false);
    await enterActions.getByRole("button", { name: /select action 1/i }).click();
    await expect(scriptEditor.locator("textarea")).toHaveValue("printf 'focused editor\\n'");

    await settings.saveChanges();
    const saved = await apiClient.listWorkflowSteps(workflow.id);
    const savedFirst = saved.steps.find((step) => step.id === first.id);
    expect(savedFirst?.events?.on_enter).toEqual([
      expect.objectContaining({
        type: "run_script",
        config: expect.objectContaining({ command: "printf 'focused editor\\n'" }),
      }),
    ]);
  });

  test("creates a workflow in the client-only card and persists it with Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.createWorkflow("Client Draft Workflow", "Custom");
    const card = settings.editor;
    await settings.selectStep(card, "Todo");
    const panel = card.getByTestId(/workflow-step-panel-/).first();

    await openStepSection(panel, "automation", false);
    await settings.addEditorAction("on_enter", "run_script");
    await settings.editorScript("on_enter").locator("textarea").fill("echo new");
    await settings.backFromEditorAction();
    await settings.submitSaveChanges();

    const workflows = await apiClient.listWorkflows(seedData.workspaceId);
    expect(workflows.workflows.some((item) => item.name === "Client Draft Workflow")).toBe(true);
  });
});
