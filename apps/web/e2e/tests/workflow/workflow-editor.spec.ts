import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Focused workflow editor", () => {
  test("starts new workflow creation in the dedicated editor route", async ({
    testPage,
    seedData,
  }) => {
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.addWorkflowButton.click();
    await expect(settings.createDialog).toBeVisible();
    await settings.workflowNameInput.fill("Routed Workflow");
    await settings.createDialog.getByText("Custom", { exact: true }).click();
    await settings.confirmCreateButton.click();

    await expect(testPage).toHaveURL(/\/workflows\/new(?:\?|$)/);
    await expect(settings.editor).toBeVisible();
    await expect(settings.editor.locator("#workflow-editor-name")).toHaveValue("Routed Workflow");
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
    const second = await apiClient.createWorkflowStep(workflow.id, "Review step", 1);

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await settings.workflowEditorLink(workflow.id).click();
    await expect(testPage).toHaveURL(new RegExp(`/workflows/${workflow.id}$`));
    await expect(settings.editor).toBeVisible();

    await testPage.getByTestId("workflow-editor-tab-automation").click();
    const enterActions = settings.editorActionList("on_enter");
    await enterActions.locator("select").selectOption("run_script");

    const scriptEditor = settings.editorScript("on_enter");
    await expect(scriptEditor).toBeVisible();
    await scriptEditor.locator("textarea").fill("printf 'focused editor\\n'");
    await testPage.getByTestId(`workflow-editor-step-${second.id}`).click();
    await testPage.getByTestId(`workflow-editor-step-${first.id}`).click();
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

  test("creates a workflow through the client-only editor route", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const settings = new WorkflowSettingsPage(testPage);
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/workflows/new`);
    await expect(settings.editor).toBeVisible();
    await testPage.getByTestId("workflow-editor-name").fill("Client Draft Workflow");

    await testPage.getByTestId("workflow-editor-tab-automation").click();
    await settings.editorActionList("on_enter").locator("select").selectOption("run_script");
    await settings.editorScript("on_enter").locator("textarea").fill("echo new");
    await settings.submitSaveChanges();

    await expect(testPage).not.toHaveURL(/\/workflows\/new$/);
    const workflows = await apiClient.listWorkflows(seedData.workspaceId);
    expect(workflows.workflows.some((item) => item.name === "Client Draft Workflow")).toBe(true);
  });
});
