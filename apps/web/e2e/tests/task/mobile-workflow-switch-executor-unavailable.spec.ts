import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { createUnavailableExecutorMoveFixture } from "./workflow-switch-executor-unavailable-helpers";

const unavailableMessage =
  "This task's executor profile is no longer available, so this step's agent cannot start.";

test("shows the executor refusal when moving a task from the phone task drawer", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const fixture = await createUnavailableExecutorMoveFixture(
    apiClient,
    seedData,
    `Mobile deleted executor profile ${Date.now()}`,
  );

  try {
    await testPage.goto(`/t/${fixture.task.id}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").tap();

    const drawer = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = drawer.getByTestId("sidebar-task-item").filter({ hasText: fixture.task.title });
    await expect(taskRow).toBeVisible();
    await taskRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByTestId("task-context-move-to").tap();

    const moveResponse = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/api/v1/tasks/${fixture.task.id}/move$`),
    );
    await testPage.getByTestId(`task-context-step-${fixture.targetStep.id}`).tap();

    const response = await moveResponse;
    expect(response.status()).toBe(409);
    const body = (await response.json()) as { code?: string; error?: string };
    expect(body.code).toBe("task_move_executor_unavailable");
    expect(body.error).not.toContain(fixture.deletedExecutorProfileId);
    await expect(
      testPage.getByTestId("toast-container").getByTestId("toast-message").last(),
    ).toContainText(unavailableMessage);

    const task = await apiClient.getTask(fixture.task.id);
    expect(task.workflow_step_id).toBe(fixture.sourceStep.id);
    expect(task.state).not.toBe("SCHEDULING");
    expect((await apiClient.listTaskSessions(fixture.task.id)).sessions).toHaveLength(1);
    await expect(taskRow).toBeVisible();
  } finally {
    await fixture.cleanup();
  }
});
