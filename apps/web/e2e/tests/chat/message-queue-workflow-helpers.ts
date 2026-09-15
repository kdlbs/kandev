import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

// @covers AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.5
// @covers AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.6
export async function expectSendNowWorkflowRunning(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
): Promise<void> {
  const workflow = await api.createWorkflow(seed.workspaceId, "Queued turn state");
  const review = await api.createWorkflowStep(workflow.id, "Review", 0, { is_start_step: true });
  const working = await api.createWorkflowStep(workflow.id, "In Progress", 1);
  await api.createWorkflowStep(workflow.id, "Done", 2);
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Queued workflow state",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: workflow.id,
      workflow_step_id: review.id,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Task did not return a session ID");
  const sessionId = task.session_id;
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await waitForSessionState(api, {
    taskId: task.id,
    sessionId,
    expectedState: "WAITING_FOR_INPUT",
    message: "queued workflow session should settle before queue setup",
  });
  await api.updateWorkflowStep(review.id, {
    events: { on_turn_start: [{ type: "move_to_step", config: { step_id: working.id } }] },
  });
  const identity = await api.getQueueSessionIdentity(task.id, sessionId);
  await api.setQueueAutoRun(identity, false);
  await api.queueMessage(
    identity,
    [
      'e2e:message("replacement is active")',
      // Flush the text buffer before holding the mock turn open.
      'e2e:tool_use("Check", {})',
      'e2e:tool_result("checked")',
      "e2e:delay(15000)",
      'e2e:message("replacement finished")',
    ].join("\n"),
  );
  const chat = session.activeChat();
  const chip = chat.getByTestId("queue-chip");
  if (mobile) await chip.tap();
  else await chip.click();
  const row = chat.getByTestId("queue-entry");
  await expect(row).toHaveCount(1);
  if (!mobile) await row.hover();
  const sendNow = row.getByTestId("queue-entry-send-now");
  if (mobile) await sendNow.tap();
  else await sendNow.click();

  const agentBodies = chat.locator("[data-agent-message-body][data-message-id]");
  await expect(agentBodies.filter({ hasText: "replacement is active" })).toBeVisible();
  await waitForSessionState(api, {
    taskId: task.id,
    sessionId,
    expectedState: "RUNNING",
    message: "Send Now should keep the queued workflow turn running",
  });
  const runningTask = await api.getTask(task.id);
  expect(runningTask.state).toBe("IN_PROGRESS");
  expect(runningTask.workflow_step_id).toBe(working.id);
  await expect(session.cancelAgentButton()).toBeVisible();

  if (mobile) await page.getByTestId("mobile-session-menu").tap();
  const surface = mobile
    ? page.getByRole("dialog", { name: "Tasks", exact: true })
    : session.sidebar;
  const filters = new SidebarFilterPopoverPage(page);
  if (mobile) await surface.getByTestId("sidebar-filter-gear").tap();
  else await filters.open();
  await filters.setGroup("State");
  await filters.close();
  await expect(
    surface.locator('[data-testid="sidebar-group-header"][data-group-key="IN_PROGRESS"]'),
  ).toBeVisible();
  await expect(
    surface.locator(`[data-task-row-id="${task.id}"]`).getByTestId("task-state-running"),
  ).toBeVisible();
  if (mobile) await page.keyboard.press("Escape");

  await waitForSessionState(api, {
    taskId: task.id,
    sessionId,
    expectedState: "WAITING_FOR_INPUT",
    message: "queued workflow turn should settle after its replacement prompt",
  });
  await expect(agentBodies.filter({ hasText: "replacement finished" })).toBeVisible();
  await expect.poll(async () => (await api.getTask(task.id)).state).toBe("REVIEW");
  await expect(session.cancelAgentButton()).toBeHidden();
}
