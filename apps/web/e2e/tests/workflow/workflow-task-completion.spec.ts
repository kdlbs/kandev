import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow task completion", () => {
  test("shows completion only on the final step and preserves it through edit flows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      `Completion editor ${Date.now()}`,
    );
    const start = await apiClient.createWorkflowStep(workflow.id, "Draft", 0, {
      is_start_step: true,
      complete_task_on_enter: false,
    });
    const middle = await apiClient.createWorkflowStep(workflow.id, "Review", 1, {
      complete_task_on_enter: false,
    });
    const done = await apiClient.createWorkflowStep(workflow.id, "Done", 2, {
      complete_task_on_enter: true,
    });

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard(workflow.name, { waitForName: true });
    await expect(card).toBeVisible();

    await settings.selectStep(card, "Review");
    await expect(settings.completeTaskOnEnterCheckbox(card, middle.id)).toHaveCount(0);
    await expect(settings.completeTaskOnEnterHelp(card, middle.id)).toHaveCount(0);

    await settings.selectStep(card, "Done");
    const checkbox = settings.completeTaskOnEnterCheckbox(card, done.id);
    const help = settings.completeTaskOnEnterHelp(card, done.id);
    await expect(checkbox).toBeChecked();
    await expect(help).toBeVisible();

    await help.hover();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Marks the task complete when it enters this final step",
    );
    await help.focus();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Marks the task complete when it enters this final step",
    );
    await help.click();
    await expect(checkbox).toBeChecked();

    await checkbox.click();
    await expect(checkbox).not.toBeChecked();
    await settings.floatingSave.getByRole("button", { name: "Reset", exact: true }).click();
    await expect(checkbox).toBeChecked();

    await checkbox.click();
    await settings.saveChanges();
    await testPage.reload();
    await settings.goto(seedData.workspaceId);
    const reloadedCard = await settings.findWorkflowCard(workflow.name, { waitForName: true });
    await settings.selectStep(reloadedCard, "Done");
    await expect(settings.completeTaskOnEnterCheckbox(reloadedCard, done.id)).not.toBeChecked();

    // Move the saved step away from the end and back. Its persisted value stays
    // attached to the step, while the control follows final position.
    await settings.reorderStep(reloadedCard, "Done", "Review");
    await settings.selectStep(reloadedCard, "Done");
    await expect(settings.completeTaskOnEnterCheckbox(reloadedCard, done.id)).toHaveCount(0);
    await settings.reorderStep(reloadedCard, "Done", "Review");
    await settings.selectStep(reloadedCard, "Done");
    await expect(settings.completeTaskOnEnterCheckbox(reloadedCard, done.id)).toBeVisible();
    await expect(settings.completeTaskOnEnterCheckbox(reloadedCard, done.id)).not.toBeChecked();
    if (await settings.floatingSave.isVisible().catch(() => false)) {
      await settings.saveChanges();
    }

    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    expect(steps.find((step) => step.id === start.id)?.position).toBe(0);
    expect(steps.find((step) => step.id === done.id)?.position).toBe(2);
    expect(steps.find((step) => step.id === done.id)?.complete_task_on_enter).toBe(false);
    await assertNoDocumentHorizontalOverflow(testPage, "workflow task completion editor");
  });

  test("does not complete an unchecked final step and accepts a follow-up turn", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      `Unchecked final ${Date.now()}`,
    );
    const start = await apiClient.createWorkflowStep(workflow.id, "Start", 0, {
      is_start_step: true,
      events: { on_turn_complete: [{ type: "move_to_next" }] },
    });
    const final = await apiClient.createWorkflowStep(workflow.id, "Final", 1, {
      complete_task_on_enter: false,
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Unchecked final task ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: workflow.id,
        workflow_step_id: start.id,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for start step turn");
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, {
        timeout: 60_000,
        message: "Workflow did not enter the unchecked final step",
      })
      .toBe(final.id);
    expect((await apiClient.getTask(task.id)).state).not.toBe("COMPLETED");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.completedSessionBanner()).toHaveCount(0);
    await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 60_000 });
    await session.waitForChatIdle({ timeout: 30_000 });

    const after = await apiClient.getTask(task.id);
    expect(after.state).not.toBe("COMPLETED");
    expect(after.workflow_step_id).toBe(final.id);
    await assertNoDocumentHorizontalOverflow(testPage, "unchecked final follow-up");
  });

  test("completes a task when it enters an enabled final step", async ({ apiClient, seedData }) => {
    test.setTimeout(150_000);
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      `Checked final ${Date.now()}`,
    );
    const start = await apiClient.createWorkflowStep(workflow.id, "Start", 0, {
      is_start_step: true,
      events: { on_turn_complete: [{ type: "move_to_next" }] },
    });
    const final = await apiClient.createWorkflowStep(workflow.id, "Final", 1, {
      complete_task_on_enter: true,
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Checked final task ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: workflow.id,
        workflow_step_id: start.id,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for checked final step");
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).state, {
        timeout: 60_000,
        message: "Task did not complete on the enabled final step",
      })
      .toBe("COMPLETED");
    expect((await apiClient.getTask(task.id)).workflow_step_id).toBe(final.id);
  });
});
