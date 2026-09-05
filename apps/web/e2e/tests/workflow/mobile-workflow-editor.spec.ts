import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Focused workflow editor on mobile", () => {
  test("navigates the journey and authors ordered lifecycle scripts", async ({
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
    await expect(settings.workflowEditorLink(workflow.id)).toBeVisible({ timeout: 30_000 });
    await settings.workflowEditorLink(workflow.id).click();
    await expect(settings.page.getByTestId("workflow-editor-mobile")).toBeVisible();
    await settings.editorStep(first.id, true).tap();
    await expect(testPage.getByTestId("workflow-editor-mobile-step-screen")).toBeVisible();

    await testPage.getByTestId("workflow-editor-tab-automation").tap();
    const enterActions = settings.editorActionList("on_enter");
    await enterActions.getByRole("button", { name: "Add action" }).tap();
    const picker = testPage.getByTestId("workflow-mobile-action-picker");
    await expect(picker).toBeVisible();
    await picker.getByRole("button", { name: "Run script" }).tap();
    await expect(testPage.getByTestId("workflow-editor-mobile-action-screen")).toBeVisible();
    await settings.editorScript("on_enter").locator("textarea").fill("echo mobile one");
    await testPage.getByRole("button", { name: "Back to step" }).tap();
    await expect(testPage).toHaveURL(/\?step=[^&]+&tab=automation$/);

    await enterActions.getByRole("button", { name: "Add action" }).tap();
    await testPage
      .getByTestId("workflow-mobile-action-picker")
      .getByRole("button", { name: "Run script" })
      .tap();
    await expect(testPage.getByTestId("workflow-editor-mobile-action-screen")).toBeVisible();
    await settings.editorScript("on_enter").locator("textarea").fill("echo mobile two");
    await testPage.getByRole("button", { name: "Move action up" }).click();
    await expect(settings.editorScript("on_enter").locator("textarea")).toHaveValue(
      "echo mobile two",
    );
    await testPage.getByRole("button", { name: "Back to step" }).tap();
    await expect(testPage).toHaveURL(/\?step=[^&]+&tab=automation$/);

    await settings.saveChanges(true);
    await expect(testPage.getByTestId("workflow-editor-mobile-step-screen")).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    for (const control of [
      testPage.getByTestId("workflow-editor-tab-automation"),
      enterActions.getByRole("button", { name: "Add action" }),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
    }
  });
});
