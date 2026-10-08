import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage, openStepSection } from "../../pages/workflow-settings-page";
import { replacePromptEditor } from "../../helpers/settings-prompt-editor";

test.describe("Inline workflow editor on mobile", () => {
  test("expands summary rows by touch and preserves edits when sections close", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Mobile step summaries");
    const step = await apiClient.createWorkflowStep(workflow.id, "Work", 0, {
      is_start_step: true,
    });
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard("Mobile step summaries");
    const panel = await settings.selectStep(card, "Work", true);
    await openStepSection(panel, "board", true);
    await panel.getByTestId(`${step.id}-wip-limit-input`).fill("3");
    const board = panel.getByTestId("workflow-section-toggle-board");
    await expect(board).toContainText("WIP limit: 3");
    await board.tap();
    await openStepSection(panel, "instructions", true);
    const templateBox = await panel
      .getByRole("button", { name: "Plan", exact: true })
      .boundingBox();
    expect(templateBox!.height).toBeGreaterThanOrEqual(44);
    await replacePromptEditor(
      testPage,
      panel.getByTestId(`workflow-step-prompt-${step.id}`),
      "Keep these instructions.",
      { touch: true },
    );
    await panel.getByTestId("workflow-section-toggle-instructions").tap();
    await expect(panel.getByTestId("workflow-section-toggle-instructions")).toContainText(
      "Keep these instructions.",
    );
    await expect(panel.getByTestId("workflow-section-toggle-agent")).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    await panel.getByTestId("workflow-section-toggle-agent").tap();
    for (const section of ["agent", "instructions", "automation", "board", "advanced"]) {
      const toggle = panel.getByTestId(`workflow-section-toggle-${section}`);
      const box = await toggle.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
    }
    await panel.scrollIntoViewIfNeeded();
    await prCapture.screenshot("mobile-step-summary-rows", {
      caption: "Expandable step summaries with touch-safe controls.",
    });
    await settings.saveChanges(true);
    await testPage.reload();
    const reloaded = await settings.findWorkflowCard("Mobile step summaries");
    const reloadedPanel = await settings.selectStep(reloaded, "Work", true);
    await expect(reloadedPanel.getByTestId("workflow-section-toggle-board")).toContainText(
      "WIP limit: 3",
    );
    await expect(reloadedPanel.getByTestId("workflow-section-toggle-instructions")).toContainText(
      "Keep these instructions.",
    );
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
  });
  test("authors ordered lifecycle scripts inside the workflow card", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Focused Editor Mobile");
    const first = await apiClient.createWorkflowStep(workflow.id, "Mobile draft", 0, {
      is_start_step: true,
    });

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard("Focused Editor Mobile");
    await settings.selectStep(card, "Mobile draft", true);
    const panel = card.getByTestId(`workflow-step-panel-${first.id}`);
    await expect(panel).toBeVisible();

    await openStepSection(panel, "automation", true);
    const enterActions = settings.editorActionList("on_enter");
    await enterActions.getByRole("button", { name: "Add action" }).tap();
    const picker = testPage.getByTestId("workflow-mobile-action-picker");
    await expect(picker).toBeVisible();
    await picker.getByRole("button", { name: "Run script" }).tap();
    await expect(testPage.getByTestId("workflow-focused-action-editor")).toBeVisible();
    await settings.editorScript("on_enter").locator("textarea").fill("echo mobile one");
    await settings.backFromEditorAction(true);

    await enterActions.getByRole("button", { name: "Add action" }).tap();
    await testPage
      .getByTestId("workflow-mobile-action-picker")
      .getByRole("button", { name: "Run script" })
      .tap();
    await expect(testPage.getByTestId("workflow-focused-action-editor")).toBeVisible();
    await settings.editorScript("on_enter").locator("textarea").fill("echo mobile two");
    const moveButton = panel.getByRole("button", { name: "Move action up", exact: true });
    const moveBox = await moveButton.boundingBox();
    expect(moveBox!.height).toBeGreaterThanOrEqual(44);
    expect(moveBox!.width).toBeGreaterThanOrEqual(44);
    await moveButton.tap();
    await expect(settings.editorScript("on_enter").locator("textarea")).toHaveValue(
      "echo mobile two",
    );
    await settings.backFromEditorAction(true);

    await settings.saveChanges(true);
    await expect(panel).toBeVisible();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspaces/${seedData.workspaceId}/workflows(?:\\?|$)`),
    );
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    for (const control of [
      panel.getByTestId("workflow-section-toggle-automation"),
      enterActions.getByRole("button", { name: "Add action" }),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
    }
    const persisted = (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
      (step) => step.id === first.id,
    );
    expect(persisted?.events?.on_enter?.map((action) => action.config?.command)).toEqual([
      "echo mobile two",
      "echo mobile one",
    ]);
    await testPage.reload();
    const reloadedCard = await settings.findWorkflowCard("Focused Editor Mobile");
    const reloadedPanel = await settings.selectStep(reloadedCard, "Mobile draft", true);
    await openStepSection(reloadedPanel, "automation", true);
    await expect(settings.editorActionList("on_enter")).toContainText("echo mobile two");
    await expect(settings.editorActionList("on_enter")).toContainText("echo mobile one");
  });
});
