import { type Page, expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

type QueuedWorkflowScenario = {
  session: SessionPage;
  sessionId: string;
};

const WORKFLOW_REVIEW_STEP = "Review";

export async function seedQueuedWorkflowMessageScenario(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
): Promise<QueuedWorkflowScenario> {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${name} Workflow`);
  const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Working", 0);
  const reviewStep = await apiClient.createWorkflowStep(workflow.id, WORKFLOW_REVIEW_STEP, 1);

  await apiClient.updateWorkflowStep(reviewStep.id, {
    prompt: 'e2e:message("workflow queued response")',
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `${name} Task`,
    seedData.agentProfileId,
    {
      description: 'e2e:delay(8000)\ne2e:message("initial response")',
      workflow_id: workflow.id,
      workflow_step_id: sourceStep.id,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return session_id");

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  // Confirm the agent is mid-turn before the manual move via the backend session
  // state, not the transient "Agent is starting/running" badge. The badge depends
  // on the client receiving the state over WS, and under the WS-subscribe race the
  // client can miss it when its subscription registers after the transition fans
  // out; the backend session state is the source of truth and avoids that race.
  // The e2e:delay(8000) keeps the first turn busy long enough to observe a STARTING
  // or RUNNING state (the mock agent can jump STARTING->WAITING_FOR_INPUT without
  // ever surfacing RUNNING, so accept either busy state).
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((s) => s.id === task.session_id)?.state ?? "";
      },
      { timeout: 20_000, message: "Waiting for agent to start its turn" },
    )
    .toMatch(/^(STARTING|RUNNING)$/);

  await apiClient.moveTask(task.id, workflow.id, reviewStep.id);

  return { session, sessionId: task.session_id };
}

export async function expectWorkflowQueueBadge(session: SessionPage) {
  const chat = session.activeChat();
  await expect(chat.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
  await chat.getByTestId("queue-chip").click();

  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel).toBeVisible({ timeout: 5_000 });
  await expect(panel.getByTestId("workflow-message-badge")).toContainText(WORKFLOW_REVIEW_STEP);
  await expect(panel.getByTestId("workflow-message-dot")).toBeVisible();
  await expect(panel.getByTestId("sender-task-badge")).toHaveCount(0);
}

export async function expectDeliveredWorkflowMessage(
  apiClient: ApiClient,
  session: SessionPage,
  sessionId: string,
) {
  const chat = session.activeChat();
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) =>
            message.author_type === "user" &&
            message.metadata?.workflow_message === true &&
            message.metadata?.workflow_step_name === WORKFLOW_REVIEW_STEP,
        );
      },
      { timeout: 30_000 },
    )
    .toBe(true);

  await expect(chat.getByTestId("queued-ghost-list")).toHaveCount(0, { timeout: 10_000 });
  await expect(chat.getByText(/^workflow queued response$/)).toBeVisible({ timeout: 30_000 });
  await expect(
    chat.getByTestId("workflow-message-badge").filter({ hasText: WORKFLOW_REVIEW_STEP }),
  ).toBeVisible({ timeout: 10_000 });
}

export async function expectNoQueuePanelHorizontalOverflow(page: Page) {
  const hasNoOverflow = await page.getByTestId("queued-ghost-list").evaluate((panel) => {
    const rect = panel.getBoundingClientRect();
    return rect.left >= 0 && rect.right <= window.innerWidth + 1;
  });
  expect(hasNoOverflow).toBe(true);
}
