import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";

/**
 * A user's Done on the Office status board, for a task that is not on its
 * workflow's final step, moves the task to that step instead of redirecting
 * it to in_review (REQ-OFFICE-HUMAN-DONE-001). Pending approvers refuse the
 * move and leave the task where it was (REQ-OFFICE-HUMAN-DONE-002).
 */

async function workflowSteps(apiClient: ApiClient, workflowId: string) {
  const { steps } = await apiClient.listWorkflowSteps(workflowId);
  const sorted = [...steps].sort((a, b) => a.position - b.position);
  return { first: sorted[0], final: sorted[sorted.length - 1] };
}

async function dragCardToColumn(page: Page, taskId: string, status: string) {
  const card = page.getByTestId(`board-card-${taskId}`);
  const target = page.getByTestId(`board-column-${status}`);
  await expect(card).toBeVisible();
  await expect(target).toBeInViewport();
  const from = await card.boundingBox();
  const to = await target.boundingBox();
  expect(from).not.toBeNull();
  expect(to).not.toBeNull();
  // dnd-kit's MouseSensor needs intermediate moves to pass its 8px activation.
  await page.mouse.move(from!.x + from!.width / 2, from!.y + from!.height / 2);
  await page.mouse.down();
  await page.mouse.move(to!.x + to!.width / 2, to!.y + 40, { steps: 12 });
  await page.mouse.up();
}

// The board is a horizontally scrolling row of seven columns; a viewport wide
// enough to show the source and the Done column keeps the drag on screen.
async function openBoard(page: Page) {
  await page.setViewportSize({ width: 2200, height: 900 });
  await page.goto("/office/tasks");
  await page.getByTestId("task-view-board").click();
}

async function persistedTask(apiClient: ApiClient, taskId: string) {
  const task = await apiClient.getTask(taskId);
  return { state: task.state ?? "", stepId: task.workflow_step_id ?? "" };
}

test.describe("Office board Done before the final workflow step", () => {
  test("dropping a card on Done moves it to the final step and completes it", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const { first, final } = await workflowSteps(apiClient, officeSeed.workflowId);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Board Done Move", {
      workflow_id: officeSeed.workflowId,
      workflow_step_id: first.id,
    });
    expect((await persistedTask(apiClient, task.id)).stepId).not.toBe(final.id);

    await openBoard(testPage);
    const changed = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/tasks/${task.id}$`));
    await dragCardToColumn(testPage, task.id, "done");
    const response = await changed;
    expect(response.status()).toBe(200);
    expect(response.request().postDataJSON()).toEqual({ status: "done" });

    const doneColumn = testPage.getByTestId("board-column-done");
    await expect(doneColumn.getByTestId(`board-card-${task.id}`)).toBeVisible();
    await expect
      .poll(() => persistedTask(apiClient, task.id))
      .toEqual({ state: "COMPLETED", stepId: final.id });
    await expect(testPage.getByText(/move the task to the final workflow step/i)).toHaveCount(0);
  });

  test("a pending approver refuses the drop and leaves the task in place", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const approver = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Board Done Approver",
      role: "worker",
    })) as { id: string };
    const { first } = await workflowSteps(apiClient, officeSeed.workflowId);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Board Done Gated", {
      workflow_id: officeSeed.workflowId,
      workflow_step_id: first.id,
    });
    const attach = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/approvers`, {
      agent_profile_id: approver.id,
    });
    expect(attach.ok).toBe(true);
    const before = await persistedTask(apiClient, task.id);

    await openBoard(testPage);
    const changed = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/tasks/${task.id}$`));
    await dragCardToColumn(testPage, task.id, "done");
    const response = await changed;
    expect(response.status()).toBe(409);
    expect(response.request().postDataJSON()).toEqual({ status: "done" });

    await expect(
      testPage.getByText(/awaiting approval from Board Done Approver/i).first(),
    ).toBeVisible();
    const doneColumn = testPage.getByTestId("board-column-done");
    await expect(doneColumn.getByTestId(`board-card-${task.id}`)).toHaveCount(0);
    await expect(testPage.getByTestId(`board-card-${task.id}`)).toBeVisible();
    expect(await persistedTask(apiClient, task.id)).toEqual(before);
  });
  test("a queued Done waits for capacity without promising completion", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const { first, final } = await workflowSteps(apiClient, officeSeed.workflowId);
    try {
      await apiClient.updateWorkflowStep(final.id, { wip_limit: 1, complete_task_on_enter: false });
      const holder = await apiClient.createTask(officeSeed.workspaceId, "Board capacity holder", {
        workflow_id: officeSeed.workflowId,
        workflow_step_id: final.id,
      });
      expect((await apiClient.getTask(holder.id)).wip_admitted).toBe(true);
      const task = await apiClient.createTask(officeSeed.workspaceId, "Board queued Done", {
        workflow_id: officeSeed.workflowId,
        workflow_step_id: first.id,
      });
      const before = await persistedTask(apiClient, task.id);
      expect(before.state).toBe("CREATED");
      expect(before.stepId).toBe(first.id);
      await openBoard(testPage);
      const changed = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/office/tasks/${task.id}$`),
      );
      await dragCardToColumn(testPage, task.id, "done");
      const response = await changed;
      expect(response.status()).toBe(200);
      expect(response.request().postDataJSON()).toEqual({ status: "done" });
      expect(await response.json()).toMatchObject({
        status: "CREATED",
        queued: true,
        workflow_step: { id: final.id, name: final.name },
      });
      await expect(
        testPage
          .getByText(`Queued for "${final.name}". Waiting for room.`, { exact: true })
          .first(),
      ).toBeVisible();
      await expect(
        testPage.getByTestId("board-column-todo").getByTestId(`board-card-${task.id}`),
      ).toBeVisible();
      await expect(
        testPage.getByTestId("board-column-done").getByTestId(`board-card-${task.id}`),
      ).toHaveCount(0);
      await expect
        .poll(async () => {
          const persisted = await apiClient.getTask(task.id);
          return {
            state: persisted.state,
            stepId: persisted.workflow_step_id,
            queuedFor: persisted.queued_for_step_id,
            admitted: persisted.wip_admitted,
          };
        })
        .toEqual({ state: before.state, stepId: final.id, queuedFor: final.id, admitted: false });
    } finally {
      await apiClient.updateWorkflowStep(final.id, {
        wip_limit: final.wip_limit ?? 0,
        complete_task_on_enter: final.complete_task_on_enter ?? false,
      });
    }
  });
});
