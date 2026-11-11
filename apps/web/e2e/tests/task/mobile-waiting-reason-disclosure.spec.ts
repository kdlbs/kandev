import { test, expect } from "../../fixtures/test-base";
import type { SidebarTaskPageResponse } from "../../../lib/types/http";
import type { TaskStatusSummary } from "../../../lib/types/task-status-summary";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const WAIT_SUMMARY: TaskStatusSummary = {
  revision: 999,
  updated_at: "2099-01-01T00:00:00Z",
  primary_session: { id: "quota-wait-session", state: "WAITING_FOR_INPUT" },
  quota_wait: {
    session_id: "quota-wait-session",
    reason: "quota_limited",
    deadline: "2099-01-01T12:30:00Z",
  },
};

async function installQuotaWaitSummary(page: import("@playwright/test").Page, taskId: string) {
  await page.addInitScript(
    ({ taskId, summary }) => {
      let boot: unknown;
      Object.defineProperty(window, "__KANDEV_BOOT_PAYLOAD__", {
        configurable: true,
        get: () => boot,
        set: (payload) => {
          const task = payload?.entities?.tasks?.[taskId];
          if (task) {
            task.status_summary = summary;
            task.statusSummary = summary;
          }
          boot = payload;
        },
      });
    },
    { taskId, summary: WAIT_SUMMARY },
  );

  await page.route("**/api/v1/workspaces/*/sidebar/query", async (route) => {
    const response = await route.fetch();
    const pageData = (await response.json()) as SidebarTaskPageResponse;
    for (const entry of pageData.entries) {
      if (entry.task?.id === taskId) entry.task.status_summary = WAIT_SUMMARY;
    }
    await route.fulfill({ response, json: pageData });
  });
}

test("opens a task wait detail from the mobile task picker without selecting the row", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const activeTask = await apiClient.createTask(seedData.workspaceId, "Current task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const waitingTask = await apiClient.createTask(seedData.workspaceId, "Quota wait details", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.seedTaskSession(activeTask.id, { state: "WAITING_FOR_INPUT" });
  await apiClient.seedTaskSession(waitingTask.id, { state: "WAITING_FOR_INPUT" });
  await installQuotaWaitSummary(testPage, waitingTask.id);

  await testPage.goto(`/t/${activeTask.id}`);
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const taskPicker = testPage.getByRole("dialog", { name: "Tasks" });
  const waitingRow = taskPicker
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Quota wait details" });
  const trigger = waitingRow.getByRole("button", { name: "Waiting on a usage limit" });
  await expect(trigger).toBeVisible();
  const triggerBounds = await trigger.boundingBox();
  expect(triggerBounds).not.toBeNull();
  expect(triggerBounds!.height).toBeGreaterThanOrEqual(44);

  await trigger.tap();
  const detail = testPage.getByRole("dialog", { name: "Waiting on a usage limit" });
  await expect(detail).toContainText("The next retry is at");
  await expect(taskPicker).toBeVisible();
  await expect(testPage).toHaveURL(new RegExp(`/t/${activeTask.id}$`));
  await assertNoDocumentHorizontalOverflow(testPage, "mobile task wait detail");
});
