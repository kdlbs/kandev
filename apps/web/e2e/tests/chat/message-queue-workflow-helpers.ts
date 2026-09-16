import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { WsWatcher } from "../../helpers/causal-waits";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { waitForComposerQueueMode } from "../../helpers/type-while-busy";
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

function activeQueuedMessage(
  marker: string,
  holdMs = 15_000,
  completionMarker = `${marker} finished`,
): string {
  return [
    `e2e:message("${marker}")`,
    'e2e:tool_use("Check", {})',
    'e2e:tool_result("checked")',
    `e2e:delay(${holdMs})`,
    `e2e:message("${completionMarker}")`,
  ].join("\n");
}

// @covers AC-UI-MESSAGE-QUEUE-SEND-NOW-001.1
// @covers AC-UI-MESSAGE-QUEUE-SEND-NOW-001.2
// @covers AC-UI-MESSAGE-QUEUE-SEND-NOW-001.9
// @covers AC-UI-MESSAGE-QUEUE-SEND-NOW-001.11
export async function expectSendNowInterruptsRunningFIFOTurn(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
  gateway: WsWatcher,
): Promise<void> {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    mobile ? "Mobile FIFO Send Now" : "Desktop FIFO Send Now",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Task did not return a session ID");
  const sessionId = task.session_id;
  const identity = await api.getQueueSessionIdentity(task.id, sessionId);
  await api.setQueueAutoMerge(identity, false);
  await api.setQueueAutoRun(identity, true);

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await waitForSessionState(api, {
    taskId: task.id,
    sessionId,
    expectedState: "WAITING_FOR_INPUT",
    message: "FIFO Send Now fixture should start idle",
  });

  // The initial turn only opens a busy window. A is delivered later by the
  // ordinary Auto-run path, which is the behavior under test.
  if (mobile) {
    await session.sendMessageViaButton("/slow 3s");
  } else {
    await session.sendMessage("/slow 3s");
  }
  await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
  await waitForComposerQueueMode(page);
  const markerA = "fifo A response";
  const markerB = "fifo B response";
  const markerC = "fifo C response";
  const completionA = "fifo A finished";
  const completionB = "fifo B finished";
  const completionC = "fifo C finished";
  await api.queueMessage(identity, activeQueuedMessage(markerA, 15_000, completionA));
  await waitForAgentMessage(api, sessionId, markerA, 60_000);
  await waitForSessionState(api, {
    taskId: task.id,
    sessionId,
    expectedState: "RUNNING",
    message: "FIFO A should own the running turn before B is queued",
  });
  const workflowStepBefore = (await api.getTask(task.id)).workflow_step_id;

  await api.queueMessage(identity, activeQueuedMessage(markerB, 10_000, completionB));
  await api.queueMessage(identity, activeQueuedMessage(markerC, 250, completionC));

  const chat = session.activeChat();
  const chip = chat.getByTestId("queue-chip");
  if (mobile) await chip.tap();
  else await chip.click();
  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel).toBeVisible();
  await expect(panel.getByTestId("queue-entry-text")).toHaveCount(2);
  const target = panel.getByTestId("queue-entry").filter({ hasText: markerB });
  await expect(target).toBeVisible();
  if (!mobile) await target.hover();
  const sendNow = target.getByTestId("queue-entry-send-now");
  await expect(sendNow).toBeVisible();
  await expect(sendNow).toBeEnabled();
  if (mobile) {
    const box = await sendNow.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
  }

  const sendNowResponse = gateway.waitForResponse("message.queue.send_now");
  if (mobile) await sendNow.tap();
  else await sendNow.click();
  await sendNowResponse;

  await expect(panel.getByTestId("queue-entry-text")).toHaveCount(1);
  await expect(panel.getByTestId("queue-entry-text")).toContainText(markerC);
  await expect(panel.getByTestId("queue-auto-run")).toHaveAttribute("data-state", "checked");
  await expect(chat.getByTestId("user-message-bubble").filter({ hasText: markerB })).toHaveCount(1);
  await waitForAgentMessage(api, sessionId, markerB, 30_000);
  await waitForSessionState(api, {
    taskId: task.id,
    sessionId,
    expectedState: "RUNNING",
    message: "Send Now replacement B should remain active while C is pending",
  });
  expect((await api.getTask(task.id)).workflow_step_id).toBe(workflowStepBefore);
  await expect(panel.getByTestId("queue-entry-text")).toHaveCount(1);
  await expect(panel.getByTestId("queue-entry-text")).toContainText(markerC);

  await waitForAgentMessage(api, sessionId, markerC, 60_000);
  await waitForSessionState(api, {
    taskId: task.id,
    sessionId,
    expectedState: "WAITING_FOR_INPUT",
    message: "FIFO C should settle after replacement B",
  });
  await expect.poll(async () => (await api.getQueueStatus(identity)).count).toBe(0);
  const agentBodies = chat.locator("[data-agent-message-body][data-message-id]");
  await expect(agentBodies.filter({ hasText: markerA })).toHaveCount(1);
  await expect(agentBodies.filter({ hasText: markerB })).toHaveCount(1);
  await expect(agentBodies.filter({ hasText: markerC })).toHaveCount(1);
  await expect(agentBodies.filter({ hasText: completionA })).toHaveCount(0);
  await expect(agentBodies.filter({ hasText: completionB })).toHaveCount(1);
  await expect(agentBodies.filter({ hasText: completionC })).toHaveCount(1);
  await expect(chat).not.toContainText("Turn cancelled by user");
}
