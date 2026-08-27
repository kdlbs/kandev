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

    // Wait for the ScriptEditor (Monaco) to mount inside the step config panel
    const monacoEditor = card.locator(".monaco-editor");
    await expect(monacoEditor).toBeVisible({ timeout: 10_000 });

    // Click into the editor to focus it
    await monacoEditor.click();
    // Typing before Monaco owns focus silently goes nowhere, so wait for the
    // focus rather than budget for it. The selector is load-bearing and was
    // established by probing the live DOM: this Monaco build uses the
    // EditContext API, so the focus target is `div.native-edit-context`. There
    // is no `textarea.inputarea`, and the only textarea present is a readonly
    // `ime-text-area` that never receives focus.
    await expect(monacoEditor.locator(".native-edit-context")).toBeFocused({ timeout: 5_000 });

    // Type {{ to trigger autocomplete
    await testPage.keyboard.type("{{");

    // The Monaco suggest widget should appear with {{task_prompt}}
    const suggestWidget = testPage.locator(".monaco-editor .suggest-widget");
    await expect(suggestWidget).toBeVisible({ timeout: 5_000 });

    // Should contain task_prompt suggestion
    const suggestion = suggestWidget.locator(".monaco-list-row").first();
    await expect(suggestion).toBeVisible();
    await expect(suggestion).toContainText("task_prompt");
  });

  test("does not duplicate closing braces when completing inside a token", async ({
    testPage,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    const stepNodes = card.locator(".group.relative");
    await stepNodes.first().click();

    const monacoEditor = card.locator(".monaco-editor");
    await expect(monacoEditor).toBeVisible({ timeout: 10_000 });
    await monacoEditor.click();
    await expect(monacoEditor.locator(".native-edit-context")).toBeFocused({ timeout: 5_000 });

    await testPage.keyboard.insertText("{{}}");
    await testPage.keyboard.press("ArrowLeft");
    await testPage.keyboard.press("ArrowLeft");
    await testPage.keyboard.press("Control+Space");

    const suggestWidget = testPage.locator(".monaco-editor .suggest-widget");
    await expect(suggestWidget).toBeVisible({ timeout: 5_000 });
    const suggestion = suggestWidget.locator(".monaco-list-row").filter({ hasText: "task_prompt" });
    await expect(suggestion.first()).toBeVisible();
    await suggestion.first().click();

    await expect(monacoEditor).toContainText("{{task_prompt}}");
    await expect(monacoEditor).not.toContainText("{{task_prompt}}}}");
  });

  test("shows and inserts a saved prompt mention when typing @ in step prompt editor", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const promptName = `Daily Summary ${Date.now()}`;
    await apiClient.createPrompt(promptName, "Some reusable prompt content for e2e mentions.");

    try {
      const page = new WorkflowSettingsPage(testPage);
      await page.goto(seedData.workspaceId);

      const card = await page.findWorkflowCard("E2E Workflow");
      await expect(card).toBeVisible();

      // Click first step to open config panel
      const stepNodes = card.locator(".group.relative");
      await stepNodes.first().click();

      // Wait for the ScriptEditor (Monaco) to mount inside the step config panel
      const monacoEditor = card.locator(".monaco-editor");
      await expect(monacoEditor).toBeVisible({ timeout: 10_000 });

      // Click into the editor to focus it
      await monacoEditor.click();
      // Focus target is `div.native-edit-context` (EditContext API), not a
      // textarea. See the note in the first test.
      await expect(monacoEditor.locator(".native-edit-context")).toBeFocused({ timeout: 5_000 });

      // Type a multi-word name prefix to trigger and filter the prompt-mention autocomplete
      await testPage.keyboard.type("@Daily ");

      // The Monaco suggest widget should appear with the seeded prompt
      const suggestWidget = testPage.locator(".monaco-editor .suggest-widget");
      await expect(suggestWidget).toBeVisible({ timeout: 5_000 });

      const suggestion = suggestWidget.locator(".monaco-list-row").filter({
        hasText: promptName,
      });
      await expect(suggestion.first()).toBeVisible();

      // Accept the suggestion and verify the editor content now contains the mention.
      await suggestion.first().click();

      await expect(monacoEditor).toContainText(`@${promptName}`);
    } finally {
      const { prompts } = await apiClient.listPrompts();
      const created = prompts.find((p) => p.name === promptName);
      if (created) {
        await apiClient.deletePrompt(created.id).catch(() => undefined);
      }
    }
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

    // Find the step agent profile select
    const agentSelect = card.getByTestId("step-agent-profile-select");
    await expect(agentSelect).toBeVisible();

    // Get current value
    const initialText = await agentSelect.textContent();
    expect(initialText).toContain("No profile override");

    // Click to open the dropdown
    await agentSelect.click();

    // Select the first non-"none" option (skip "No profile override").
    // `count()` is a one-shot read, not an auto-retrying assertion, so gate on
    // the listbox being populated before counting instead of sleeping first.
    //
    // Visibility of the *first* option is not that gate: "No profile override"
    // is static markup and is already there while the settings bootstrap is
    // still loading profiles, so a count() taken on it reads 1 and skips the
    // test. Poll the count itself, which is the thing the branch below reads.
    const options = testPage.getByRole("option");
    await expect(options.first()).toBeVisible({ timeout: 5_000 });
    await expect
      .poll(() => options.count(), { timeout: 5_000 })
      .toBeGreaterThan(1)
      // A workspace with genuinely no profiles is a legitimate skip, so this
      // stays tolerant; the poll only removes the race with a slow bootstrap.
      .catch(() => undefined);
    const optionCount = await options.count();
    // Need at least 2 options (none + at least one profile)
    if (optionCount < 2) {
      test.skip(true, "No agent profiles available to test with");
      return;
    }

    const profileOption = options.nth(1);
    const profileName = await profileOption.textContent();
    await profileOption.click();

    // The select should now show the selected profile, not revert to "No
    // profile override". Assert both halves with auto-retrying matchers rather
    // than sleeping and then taking a single non-retrying textContent sample.
    await expect(agentSelect).toContainText(profileName?.trim() ?? "", { timeout: 10_000 });
    await expect(agentSelect).not.toContainText("No profile override");
    await page.saveChanges();

    // Reload the page and verify it persisted
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("E2E Workflow");
    await expect(reloadedCard).toBeVisible();

    // Click the same step again
    const reloadedSteps = reloadedCard.locator(".group.relative");
    await reloadedSteps.first().click();

    const reloadedSelect = reloadedCard.getByTestId("step-agent-profile-select");
    await expect(reloadedSelect).toBeVisible();
    await expect(reloadedSelect).toContainText(profileName?.trim() ?? "", { timeout: 10_000 });

    // Clean up: reset the step agent profile
    const stepId = seedData.steps[0]?.id;
    if (stepId) {
      await apiClient.updateWorkflowStep(stepId, { agent_profile_id: "" });
    }
  });
});
