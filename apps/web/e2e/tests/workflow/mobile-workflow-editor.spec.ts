import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Inline workflow editor on mobile", () => {
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

    await panel.getByTestId("workflow-editor-tab-automation").tap();
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
    await testPage.getByRole("button", { name: "Move action up" }).click();
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
      panel.getByTestId("workflow-editor-tab-automation"),
      enterActions.getByRole("button", { name: "Add action" }),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
    }
  });
});
