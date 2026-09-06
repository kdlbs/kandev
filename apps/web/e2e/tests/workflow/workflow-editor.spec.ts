import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Inline workflow editor", () => {
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
    await expect(card.getByTestId("workflow-editor-tab-agent")).toBeVisible();
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

    await panel.getByTestId("workflow-editor-tab-automation").click();
    const enterActions = settings.editorActionList("on_enter");
    await enterActions.locator("select").selectOption("run_script");

    const scriptEditor = settings.editorScript("on_enter");
    await expect(scriptEditor).toBeVisible();
    await scriptEditor.locator("textarea").fill("printf 'focused editor\\n'");
    await settings.selectStep(card, "Review step");
    await settings.selectStep(card, "Draft step");
    panel = card.getByTestId(`workflow-step-panel-${first.id}`);
    await panel.getByTestId("workflow-editor-tab-automation").click();
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

    await panel.getByTestId("workflow-editor-tab-automation").click();
    await settings.addEditorAction("on_enter", "run_script");
    await settings.editorScript("on_enter").locator("textarea").fill("echo new");
    await settings.backFromEditorAction();
    await settings.submitSaveChanges();

    const workflows = await apiClient.listWorkflows(seedData.workspaceId);
    expect(workflows.workflows.some((item) => item.name === "Client Draft Workflow")).toBe(true);
  });
});
