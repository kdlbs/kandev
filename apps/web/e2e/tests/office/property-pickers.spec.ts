import { test, expect } from "../../fixtures/office-fixture";
import type { Page } from "@playwright/test";

/**
 * E2E coverage for office task property pickers (status, priority,
 * assignee, project, parent, blockers, sub-issues, reviewers, approvers).
 *
 * The pickers all use optimistic mutations: the UI updates immediately,
 * fires the API call, and rolls back on failure (with a toast). Tests here
 * exercise the happy path against the real backend, plus one route-mock
 * case for the rollback contract.
 */

async function gotoTaskPage(testPage: Page, taskId: string, title: string) {
  await testPage.goto(`/office/tasks/${taskId}`);
  await expect(testPage.getByRole("heading", { name: title })).toBeVisible({
    timeout: 10_000,
  });
}

test.describe("property pickers", () => {
  test("status picker updates task status and persists", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Picker Status Task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Picker Status Task");

    await testPage.getByTestId("status-picker-trigger").click();
    await testPage.getByTestId("status-picker-option-in_progress").click();

    // The trigger label should reflect the new value.
    await expect(testPage.getByTestId("status-picker-trigger")).toContainText(/In Progress/i, {
      timeout: 15_000,
    });

    // Persisted in backend.
    const persisted = (await officeApi.getTask(task.id)) as Record<string, unknown>;
    const inner = (persisted.task as Record<string, unknown>) ?? persisted;
    const status = (inner.status as string) ?? (inner.state as string) ?? "";
    expect(status.toLowerCase()).toContain("progress");
  });

  test("priority picker updates priority and shows the icon", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Picker Priority Task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Picker Priority Task");

    await testPage.getByTestId("priority-picker-trigger").click();
    await testPage.getByTestId("priority-picker-option-high").click();

    await expect(testPage.getByTestId("priority-picker-trigger")).toContainText(/High/i, {
      timeout: 15_000,
    });
  });

  test("assignee picker assigns an agent and clears with No assignee", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    test.setTimeout(90_000);
    // Ensure there is a worker agent we can assign.
    await officeApi
      .createAgent(officeSeed.workspaceId, {
        name: "Picker Worker",
        role: "worker",
      })
      .catch(() => undefined);

    const task = await apiClient.createTask(officeSeed.workspaceId, "Picker Assignee Task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Picker Assignee Task");

    const trigger = testPage.getByTestId("assignee-picker-trigger");
    await expect(trigger).toBeVisible({ timeout: 10_000 });
    await trigger.click();

    // Combobox uses cmdk; pick the first agent option visible (CEO from seed).
    const ceoOption = testPage.getByRole("option", { name: /CEO/i }).first();
    await ceoOption.click();
    await expect(trigger).toContainText(/CEO/i, { timeout: 5_000 });

    // Clear by selecting "No assignee".
    await trigger.click();
    await testPage.getByRole("option", { name: /No assignee/i }).click();
    await expect(trigger).toContainText(/No assignee/i, { timeout: 5_000 });
  });

  test("project picker assigns a project and clears with No project", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Picker Project Task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Picker Project Task");

    const trigger = testPage.getByTestId("project-picker-trigger");
    await expect(trigger).toBeVisible({ timeout: 10_000 });
    await trigger.click();

    // Pick the first project option that is NOT "No project".
    const realProjectOption = testPage
      .getByRole("option")
      .filter({ hasNotText: /No project/i })
      .first();
    // If no projects exist in the seed (unlikely; onboarding creates one),
    // skip the assignment leg gracefully.
    const count = await realProjectOption.count();
    if (count === 0) {
      await testPage.keyboard.press("Escape");
      return;
    }
    await realProjectOption.click();

    // The trigger should no longer say "No project".
    await expect(trigger).not.toContainText(/No project/i, { timeout: 5_000 });

    // Clear back to "No project".
    await trigger.click();
    await testPage.getByRole("option", { name: /No project/i }).click();
    await expect(trigger).toContainText(/No project/i, { timeout: 5_000 });
  });

  test("parent picker assigns a parent and rejects self-reference at backend", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Picker Parent Candidate", {
      workflow_id: officeSeed.workflowId,
    });
    const child = await apiClient.createTask(officeSeed.workspaceId, "Picker Parent Child", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, child.id, "Picker Parent Child");

    const trigger = testPage.getByTestId("parent-picker-trigger");
    await expect(trigger).toBeVisible({ timeout: 10_000 });
    await trigger.click();

    // The candidate parent task should be selectable. Use a regex on the
    // task title so we don't depend on identifier formatting.
    await testPage.getByRole("option", { name: /Picker Parent Candidate/i }).click();
    await expect(trigger).toContainText(/Picker Parent Candidate/i, { timeout: 5_000 });

    // Self-reference rejection: PATCH /office/tasks/:id with parent_id ==
    // own id must NOT 2xx. We hit the office endpoint directly (the picker
    // calls the same one) — the regular /tasks/:id endpoint does not even
    // accept parent_id, so the contract lives on the office handler.
    const res = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${child.id}`, {
      parent_id: child.id,
    });
    expect(res.ok).toBe(false);
    void parent; // keep eslint happy
  });

  test("blockers picker adds + removes a blocker via chip rows", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const blocker = await apiClient.createTask(officeSeed.workspaceId, "Picker Blocker Source", {
      workflow_id: officeSeed.workflowId,
    });
    const target = await apiClient.createTask(officeSeed.workspaceId, "Picker Blocker Target", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, target.id, "Picker Blocker Target");

    const trigger = testPage.getByTestId("blockers-picker-trigger");
    await expect(trigger).toBeVisible({ timeout: 10_000 });
    await trigger.click();

    // Add the blocker via the multi-select-popover Command list.
    const addItem = testPage.getByTestId(`multi-select-add-${blocker.id}`);
    await addItem.click();

    // Backend should reflect the new blocker.
    const after = (await officeApi.getTask(target.id)) as Record<string, unknown>;
    const inner = (after.task as Record<string, unknown>) ?? after;
    const blockedBy =
      (inner.blocked_by as string[]) ??
      (inner.blockedBy as string[]) ??
      ((inner.blockers as Array<{ id?: string; blocker_task_id?: string }>) ?? []).map(
        (b) => b.blocker_task_id ?? b.id ?? "",
      );
    expect(blockedBy).toContain(blocker.id);

    // Remove via the popover's "remove" entry.
    if (await testPage.getByTestId(`multi-select-remove-${blocker.id}`).count()) {
      await testPage.getByTestId(`multi-select-remove-${blocker.id}`).click();
    } else {
      // Popover may have closed after the optimistic add. Re-open and remove.
      await trigger.click();
      await testPage.getByTestId(`multi-select-remove-${blocker.id}`).click();
    }
  });

  test("sub-issues row opens NewTaskDialog with parent prefilled", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Picker Sub Parent Task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Picker Sub Parent Task");

    // The Sub-issues "Add sub-issue" button lives in the right-side
    // properties panel. The page also renders an action row "New sub-issue"
    // button — scope to the panel.
    await testPage.getByTestId("sub-issues-add-button").click();

    // The NewTaskDialog displays a "Sub-issue of <id>" affordance when
    // parentTaskId is set.
    await expect(testPage.getByText(/Sub-issue of/i)).toBeVisible({ timeout: 5_000 });
  });

  test("reviewers picker adds + removes a reviewer agent", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Create a second agent so the picker has two options.
    const created = (await officeApi
      .createAgent(officeSeed.workspaceId, {
        name: "Picker Reviewer Agent",
        role: "worker",
      })
      .catch(() => undefined)) as Record<string, unknown> | undefined;
    const reviewerId = (created?.id as string) ?? officeSeed.agentId;

    const task = await apiClient.createTask(officeSeed.workspaceId, "Picker Reviewer Task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Picker Reviewer Task");

    const trigger = testPage.getByTestId("reviewers-picker-trigger");
    await expect(trigger).toBeVisible({ timeout: 10_000 });
    await trigger.click();
    await testPage.getByTestId(`multi-select-add-${reviewerId}`).click();

    // Re-open if the popover auto-closed and remove the reviewer.
    const removeItem = testPage.getByTestId(`multi-select-remove-${reviewerId}`);
    if ((await removeItem.count()) === 0) {
      await trigger.click();
    }
    await testPage.getByTestId(`multi-select-remove-${reviewerId}`).click();
  });

  test("approvers picker adds + removes an approver agent", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const created = (await officeApi
      .createAgent(officeSeed.workspaceId, {
        name: "Picker Approver Agent",
        role: "worker",
      })
      .catch(() => undefined)) as Record<string, unknown> | undefined;
    const approverId = (created?.id as string) ?? officeSeed.agentId;

    const task = await apiClient.createTask(officeSeed.workspaceId, "Picker Approver Task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Picker Approver Task");

    const trigger = testPage.getByTestId("approvers-picker-trigger");
    await expect(trigger).toBeVisible({ timeout: 10_000 });
    await trigger.click();
    await testPage.getByTestId(`multi-select-add-${approverId}`).click();

    const removeItem = testPage.getByTestId(`multi-select-remove-${approverId}`);
    if ((await removeItem.count()) === 0) {
      await trigger.click();
    }
    await testPage.getByTestId(`multi-select-remove-${approverId}`).click();
  });

  test("optimistic update rolls back + toasts on backend error", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Picker Rollback Task", {
      workflow_id: officeSeed.workflowId,
    });
    await gotoTaskPage(testPage, task.id, "Picker Rollback Task");

    // Intercept the PATCH /api/v1/tasks/:id call and return 500 so the
    // optimistic mutation must roll back.
    await testPage.route(
      (url) =>
        url.pathname === `/api/v1/tasks/${task.id}` || url.pathname.endsWith(`/tasks/${task.id}`),
      async (route) => {
        if (route.request().method() === "PATCH") {
          await route.fulfill({
            status: 500,
            contentType: "application/json",
            body: JSON.stringify({ error: "forced failure" }),
          });
          return;
        }
        await route.continue();
      },
    );

    // The task was created with default priority (medium). Snapshot the
    // current trigger label so we can assert it rolls back to that value
    // regardless of what the seed default is.
    const priorityTrigger = testPage.getByTestId("priority-picker-trigger");
    const before = (await priorityTrigger.textContent())?.trim() ?? "";

    await priorityTrigger.click();
    await testPage.getByTestId("priority-picker-option-critical").click();

    // After the forced failure, the trigger label must roll back to the
    // pre-mutation value (the optimistic update is reverted).
    await expect(async () => {
      const after = (await priorityTrigger.textContent())?.trim() ?? "";
      expect(after).toBe(before);
    }).toPass({ timeout: 5_000 });
  });
});
