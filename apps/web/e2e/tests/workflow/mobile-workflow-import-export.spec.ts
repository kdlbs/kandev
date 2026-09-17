import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow import profile selection on mobile", () => {
  test("uses a drawer picker and preserves a later session target", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const replacement = await apiClient.getAgentProfile(seedData.agentProfileId);
    const workflowName = `Mobile profile import ${Date.now()}`;
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
          agent_name: Missing mobile import agent
          model: missing-mobile-import-model
          mode: missing-mobile-import-mode
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
      await testPage.getByRole("button", { name: "Import", exact: true }).tap();

      const importDialog = testPage.getByRole("dialog");
      await importDialog.locator("textarea").fill(yamlContent);
      await importDialog.getByRole("button", { name: "Import", exact: true }).tap();

      const selection = testPage.getByTestId("workflow-import-profile-selection");
      await expect(selection).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage, "workflow import profile selection");

      const selectProfile = selection.getByTestId("workflow-import-profile-select-0:1");
      const submit = selection.getByTestId("workflow-import-profile-submit");
      const [selectBox, submitBox] = await Promise.all([
        selectProfile.boundingBox(),
        submit.boundingBox(),
      ]);
      expect(selectBox).not.toBeNull();
      expect(submitBox).not.toBeNull();
      expect(selectBox!.height).toBeGreaterThanOrEqual(44);
      expect(submitBox!.height).toBeGreaterThanOrEqual(44);

      await selectProfile.tap();
      await expect(selection.getByRole("button", { name: "Back", exact: true })).toBeVisible();
      await selection.getByTestId(`workflow-import-profile-option-${replacement.id}`).tap();
      await expect(selection.getByRole("button", { name: "Back", exact: true })).not.toBeVisible();
      await submit.tap();
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
    } finally {
      const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
      const imported = workflows.find((workflow) => workflow.name === workflowName);
      if (imported) await apiClient.deleteWorkflow(imported.id).catch(() => {});
    }
  });
});
