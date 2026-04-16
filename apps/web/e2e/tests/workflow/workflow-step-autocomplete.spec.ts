import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow step prompt autocomplete", () => {
  test("shows autocomplete suggestions when typing {{ in step prompt editor", async ({
    testPage,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    // Click first step to open config panel
    const stepNodes = card.locator(".group.relative");
    await stepNodes.first().click();
    await testPage.waitForTimeout(500);

    // Wait for the ScriptEditor (Monaco) to mount inside the step config panel
    const monacoEditor = card.locator(".monaco-editor");
    await expect(monacoEditor).toBeVisible({ timeout: 10_000 });

    // Click into the editor to focus it
    await monacoEditor.click();
    await testPage.waitForTimeout(300);

    // Type {{ to trigger autocomplete
    await testPage.keyboard.type("{{");
    await testPage.waitForTimeout(500);

    // The Monaco suggest widget should appear with {{task_prompt}}
    const suggestWidget = testPage.locator(".monaco-editor .suggest-widget");
    await expect(suggestWidget).toBeVisible({ timeout: 5_000 });

    // Should contain task_prompt suggestion
    const suggestion = suggestWidget.locator(".monaco-list-row").first();
    await expect(suggestion).toBeVisible();
    await expect(suggestion).toContainText("task_prompt");
  });

  test("persists step agent profile selection after change", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    // Click first step to open config panel
    const stepNodes = card.locator(".group.relative");
    await stepNodes.first().click();
    await testPage.waitForTimeout(500);

    // Find the step agent profile select
    const agentSelect = card.getByTestId("step-agent-profile-select");
    await expect(agentSelect).toBeVisible();

    // Get current value
    const initialText = await agentSelect.textContent();
    expect(initialText).toContain("No profile override");

    // Click to open the dropdown
    await agentSelect.click();
    await testPage.waitForTimeout(300);

    // Select the first non-"none" option (skip "No profile override")
    const options = testPage.getByRole("option");
    const optionCount = await options.count();
    // Need at least 2 options (none + at least one profile)
    if (optionCount < 2) {
      test.skip(true, "No agent profiles available to test with");
      return;
    }

    const profileOption = options.nth(1);
    const profileName = await profileOption.textContent();
    await profileOption.click();
    await testPage.waitForTimeout(1000);

    // The select should now show the selected profile, not revert to "No profile override"
    const updatedText = await agentSelect.textContent();
    expect(updatedText).toContain(profileName?.trim() ?? "");
    expect(updatedText).not.toContain("No profile override");

    // Reload the page and verify it persisted
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("E2E Workflow");
    await expect(reloadedCard).toBeVisible();

    // Click the same step again
    const reloadedSteps = reloadedCard.locator(".group.relative");
    await reloadedSteps.first().click();
    await testPage.waitForTimeout(500);

    const reloadedSelect = reloadedCard.getByTestId("step-agent-profile-select");
    await expect(reloadedSelect).toBeVisible();
    const persistedText = await reloadedSelect.textContent();
    expect(persistedText).toContain(profileName?.trim() ?? "");

    // Clean up: reset the step agent profile
    const stepId = seedData.steps[0]?.id;
    if (stepId) {
      await apiClient.updateWorkflowStep(stepId, { agent_profile_id: "" });
    }
  });
});
