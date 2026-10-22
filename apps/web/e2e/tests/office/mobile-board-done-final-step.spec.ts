import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import { touchDragBetween } from "../../helpers/touch-drag";

/**
 * Phone parity for REQ-OFFICE-HUMAN-DONE-001/-002: the status picker and board drag
 * move a task before its final workflow step to that step, and a
 * pending approver's refusal is shown as visible text.
 */

async function firstAndFinalSteps(apiClient: ApiClient, workflowId: string) {
  const { steps } = await apiClient.listWorkflowSteps(workflowId);
  const sorted = [...steps].sort((a, b) => a.position - b.position);
  return { first: sorted[0], final: sorted[sorted.length - 1] };
}

test.describe("Office status picker Done on mobile", () => {
  test("Done before the final step moves the task there and completes it", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const { first, final } = await firstAndFinalSteps(apiClient, officeSeed.workflowId);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Mobile Done Move", {
      workflow_id: officeSeed.workflowId,
      workflow_step_id: first.id,
    });

    await testPage.goto(`/office/tasks/${task.id}`);
    await testPage.getByTestId("status-picker-trigger").tap();
    const changed = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/tasks/${task.id}$`));
    await testPage.getByTestId("status-picker-option-done").tap();
    const response = await changed;
    expect(response.status()).toBe(200);
    expect(response.request().postDataJSON()).toEqual({ status: "done" });

    await expect(testPage.getByTestId("status-picker-trigger")).toContainText(/Done/i);
    await expect
      .poll(async () => {
        const persisted = await apiClient.getTask(task.id);
        return { state: persisted.state, stepId: persisted.workflow_step_id };
      })
      .toEqual({ state: "COMPLETED", stepId: final.id });
  });

  test("a pending approver refusal is shown and nothing changes", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const approver = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Mobile Done Approver",
      role: "worker",
    })) as { id: string };
    const { first } = await firstAndFinalSteps(apiClient, officeSeed.workflowId);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Mobile Done Gated", {
      workflow_id: officeSeed.workflowId,
      workflow_step_id: first.id,
    });
    const attach = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/approvers`, {
      agent_profile_id: approver.id,
    });
    expect(attach.ok).toBe(true);
    const before = await apiClient.getTask(task.id);

    await testPage.goto(`/office/tasks/${task.id}`);
    const trigger = testPage.getByTestId("status-picker-trigger");
    const initialLabel = (await trigger.textContent()) ?? "";
    await trigger.tap();
    const changed = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/tasks/${task.id}$`));
    await testPage.getByTestId("status-picker-option-done").tap();
    const response = await changed;
    expect(response.status()).toBe(409);
    expect(response.request().postDataJSON()).toEqual({ status: "done" });

    await expect(
      testPage.getByText(/awaiting approval from Mobile Done Approver/i).first(),
    ).toBeVisible();
    await expect(trigger).toHaveText(initialLabel);
    const after = await apiClient.getTask(task.id);
    expect({ state: after.state, stepId: after.workflow_step_id }).toEqual({
      state: before.state,
      stepId: before.workflow_step_id,
    });
  });
});

async function touchDragBlockedCardToDone(page: Page, taskId: string) {
  const viewport = page.viewportSize();
  expect(viewport).not.toBeNull();
  expect(viewport!.width).toBeLessThan(768);
  expect(await page.evaluate(() => navigator.maxTouchPoints)).toBeGreaterThan(0);
  const card = page.getByTestId(`board-card-${taskId}`);
  const target = page.getByTestId("board-column-done");
  await expect(card).toHaveCount(1);
  await card.scrollIntoViewIfNeeded();
  await target.scrollIntoViewIfNeeded();
  await expect(card).toBeInViewport();
  await expect(target).toBeInViewport();
  const to = await target.boundingBox();
  expect(to).not.toBeNull();
  const from = await card.boundingBox();
  expect(from).not.toBeNull();
  const left = Math.max(0, from!.x);
  const right = Math.min(viewport!.width, from!.x + from!.width);
  expect(right - left).toBeGreaterThan(44);
  await touchDragBetween(
    page,
    { x: (left + right) / 2, y: from!.y + from!.height / 2 },
    { x: to!.x + to!.width / 2, y: to!.y + 40 },
    async () => {
      await expect(card).toHaveClass(/opacity-40/);
    },
  );
}

test.describe("Office board Done by phone press-and-hold", () => {
  // @covers AC-OFFICE-HUMAN-DONE-001.5
  test("touch-dragging Done completes a task before its final step", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const { first, final } = await firstAndFinalSteps(apiClient, officeSeed.workflowId);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Phone board Done", {
      workflow_id: officeSeed.workflowId,
      workflow_step_id: first.id,
    });
    // Adjacent status columns allow a real phone drag without an off-screen drop.
    const seeded = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      status: "blocked",
    });
    expect(seeded.status).toBe(200);
    expect((await apiClient.getTask(task.id)).workflow_step_id).toBe(first.id);
    await testPage.goto("/office/tasks");
    await testPage.getByTestId("task-view-board").tap();
    const changed = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/tasks/${task.id}$`));

    await touchDragBlockedCardToDone(testPage, task.id);

    const response = await changed;
    expect(response.status()).toBe(200);
    expect(response.request().postDataJSON()).toEqual({ status: "done" });
    await expect(
      testPage.getByTestId("board-column-done").getByTestId(`board-card-${task.id}`),
    ).toBeVisible();
    await expect
      .poll(async () => {
        const persisted = await apiClient.getTask(task.id);
        return { state: persisted.state, stepId: persisted.workflow_step_id };
      })
      .toEqual({ state: "COMPLETED", stepId: final.id });
  });

  // @covers AC-OFFICE-HUMAN-DONE-001.3 AC-OFFICE-HUMAN-DONE-001.5
  test("pending approval refuses the touch drop and restores the card", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const approver = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Phone board approver",
      role: "worker",
    })) as { id: string };
    const { first } = await firstAndFinalSteps(apiClient, officeSeed.workflowId);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Phone board refused", {
      workflow_id: officeSeed.workflowId,
      workflow_step_id: first.id,
    });
    const seeded = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      status: "blocked",
    });
    expect(seeded.status).toBe(200);
    const attach = await apiClient.rawRequest("POST", `/api/v1/office/tasks/${task.id}/approvers`, {
      agent_profile_id: approver.id,
    });
    expect(attach.ok).toBe(true);
    const before = await apiClient.getTask(task.id);
    expect(before.workflow_step_id).toBe(first.id);
    await testPage.goto("/office/tasks");
    await testPage.getByTestId("task-view-board").tap();
    const changed = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/tasks/${task.id}$`));

    await touchDragBlockedCardToDone(testPage, task.id);

    const response = await changed;
    expect(response.status()).toBe(409);
    expect(response.request().postDataJSON()).toEqual({ status: "done" });
    await expect(testPage.getByText(/awaiting approval from Phone board approver/i)).toBeVisible();
    await expect(
      testPage.getByTestId("board-column-done").getByTestId(`board-card-${task.id}`),
    ).toHaveCount(0);
    await expect(
      testPage.getByTestId("board-column-blocked").getByTestId(`board-card-${task.id}`),
    ).toBeVisible();
    const after = await apiClient.getTask(task.id);
    expect({ state: after.state, stepId: after.workflow_step_id }).toEqual({
      state: before.state,
      stepId: before.workflow_step_id,
    });
  });
});
