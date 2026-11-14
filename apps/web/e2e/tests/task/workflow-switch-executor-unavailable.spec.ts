import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { waitForHttp } from "../../helpers/causal-waits";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { createUnavailableExecutorMoveFixture } from "./workflow-switch-executor-unavailable-helpers";

const unavailableMessage =
  "This task's executor profile is no longer available, so this step's agent cannot start.";

test("refuses a workflow move when the task executor profile was deleted", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const fixture = await createUnavailableExecutorMoveFixture(
    apiClient,
    seedData,
    `Deleted executor profile ${Date.now()}`,
  );

  try {
    await testPage.goto(`/t/${fixture.task.id}`);
    await new SessionPage(testPage).waitForLoad();

    const moveResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/api/v1/tasks/${fixture.task.id}/move$`),
    );
    const targetStep = testPage.getByTestId(`workflow-step-${fixture.targetStep.name}`);
    await expect(targetStep).toBeVisible();
    await targetStep.hover();
    const movePopover = testPage.getByTestId("workflow-step-popover");
    await expect(movePopover).toBeVisible();
    await waitForFiniteAnimations(movePopover);
    await movePopover.getByTestId("workflow-step-move-here").click();

    const response = await moveResponse;
    expect(response.status()).toBe(409);
    const body = (await response.json()) as { code?: string; error?: string };
    expect(body.code).toBe("task_move_executor_unavailable");
    expect(body.error).not.toContain(fixture.deletedExecutorProfileId);
    await expect(testPage.getByTestId("task-move-error-banner")).toContainText(unavailableMessage);

    const task = await apiClient.getTask(fixture.task.id);
    expect(task.workflow_step_id).toBe(fixture.sourceStep.id);
    expect(task.state).not.toBe("SCHEDULING");
    expect((await apiClient.listTaskSessions(fixture.task.id)).sessions).toHaveLength(1);

    const kanban = new KanbanPage(testPage);
    await kanban.goto(fixture.workflowId);
    await expect(kanban.taskCardInColumn(fixture.task.title, fixture.sourceStep.id)).toBeVisible();
    await expect(kanban.taskCardInColumn(fixture.task.title, fixture.targetStep.id)).toHaveCount(0);
  } finally {
    await fixture.cleanup();
  }
});
