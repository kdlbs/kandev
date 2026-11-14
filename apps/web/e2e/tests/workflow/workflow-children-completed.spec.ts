import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Workflow children completed trigger", () => {
  test("moves the parent only after every active child succeeds", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Children Completed Workflow",
    );
    const waitStep = await apiClient.createWorkflowStep(workflow.id, "Waiting on Children", 0);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Parent Done", 1);

    await apiClient.updateWorkflowStep(waitStep.id, {
      events: {
        on_children_completed: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    const createParent = async (title: string) => {
      const parent = await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: workflow.id,
        workflow_step_id: waitStep.id,
        agent_profile_id: seedData.agentProfileId,
        repository_ids: [seedData.repositoryId],
      });
      const { session_id: parentSessionId } = await apiClient.seedTaskSession(parent.id, {
        state: "WAITING_FOR_INPUT",
        sessionId: `children-completed-parent-${parent.id}`,
        agentProfileId: seedData.agentProfileId,
      });
      const parentSessions = await apiClient.listTaskSessions(parent.id);
      expect(parentSessions.sessions).toContainEqual(
        expect.objectContaining({ id: parentSessionId, task_id: parent.id }),
      );
      return parent;
    };

    const createChildren = async (parentId: string, labels: string[]) =>
      Promise.all(
        labels.map((label) =>
          apiClient.createTask(seedData.workspaceId, label, {
            workflow_id: workflow.id,
            workflow_step_id: waitStep.id,
            parent_id: parentId,
            repository_ids: [seedData.repositoryId],
          }),
        ),
      );

    const failedParent = await createParent("Parent waits after failed children");
    const failedChildren = await createChildren(failedParent.id, [
      "Failed child one",
      "Failed child two",
    ]);
    const mixedParent = await createParent("Parent waits after mixed children");
    const mixedChildren = await createChildren(mixedParent.id, ["Completed child", "Failed child"]);
    const cancelledParent = await createParent("Parent waits after cancelled child");
    const cancelledChildren = await createChildren(cancelledParent.id, [
      "Successful child before cancellation",
      "Cancelled child",
    ]);
    const successfulParent = await createParent("Parent moves after successful children");
    const successfulChildren = await createChildren(successfulParent.id, [
      "Successful child one",
      "Successful child two",
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    for (const title of [
      "Parent waits after failed children",
      "Parent waits after mixed children",
      "Parent waits after cancelled child",
      "Parent moves after successful children",
    ]) {
      await expect(kanban.taskCardInColumn(title, waitStep.id)).toBeVisible({ timeout: 15_000 });
    }

    await apiClient.updateTaskState(failedChildren[0].id, "FAILED");
    await apiClient.updateTaskState(failedChildren[1].id, "FAILED");
    await dwell(
      testPage,
      1_000,
      "negative-assertion",
      "A failed child set has no parent transition notification to await",
    );
    await assertParentWaiting(failedParent.id, "Parent waits after failed children");

    await apiClient.updateTaskState(mixedChildren[0].id, "COMPLETED");
    await apiClient.updateTaskState(mixedChildren[1].id, "FAILED");
    await dwell(
      testPage,
      1_000,
      "negative-assertion",
      "A mixed successful and failed child set has no parent transition notification to await",
    );
    await assertParentWaiting(mixedParent.id, "Parent waits after mixed children");

    await apiClient.updateTaskState(mixedChildren[1].id, "COMPLETED");
    await assertParentMoved(mixedParent.id, "Parent waits after mixed children");

    await apiClient.updateTaskState(cancelledChildren[0].id, "COMPLETED");
    await apiClient.updateTaskState(cancelledChildren[1].id, "CANCELLED");
    await dwell(
      testPage,
      1_000,
      "negative-assertion",
      "A cancelled child set has no parent transition notification to await",
    );
    await assertParentWaiting(cancelledParent.id, "Parent waits after cancelled child");

    await apiClient.updateTaskState(successfulChildren[0].id, "COMPLETED");
    await expect
      .poll(async () => (await apiClient.getTask(successfulParent.id)).workflow_step_id)
      .toBe(waitStep.id);
    await apiClient.updateTaskState(successfulChildren[1].id, "COMPLETED");
    await assertParentMoved(successfulParent.id, "Parent moves after successful children");

    async function assertParentWaiting(parentId: string, title: string) {
      const parent = await apiClient.getTask(parentId);
      expect(parent.workflow_step_id).toBe(waitStep.id);
      await expect(kanban.taskCardInColumn(title, waitStep.id)).toBeVisible();
      await expect(kanban.taskCardInColumn(title, doneStep.id)).toHaveCount(0);
    }

    async function assertParentMoved(parentId: string, title: string) {
      await expect
        .poll(async () => (await apiClient.getTask(parentId)).workflow_step_id)
        .toBe(doneStep.id);
      await expect(kanban.taskCardInColumn(title, doneStep.id)).toBeVisible();
    }
  });
});
