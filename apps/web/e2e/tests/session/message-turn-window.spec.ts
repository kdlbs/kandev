import { expect, test } from "../../fixtures/test-base";
import {
  bootMessageCount,
  captureMessageWindows,
  readMessageWindow,
  waitForInitialMessageWindow,
} from "./message-turn-window-capture";

test("boot and older-message paging retain only the turns covered by each window", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(150_000);
  const task = await apiClient.createTask(seedData.workspaceId, "Message turn window", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const session = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    sessionId: `message-turn-window-${task.id}`,
    agentProfileId: seedData.agentProfileId,
    startedAt: "2026-10-01T00:00:00Z",
  });
  await apiClient.seedSessionMessage(session.session_id, {
    type: "message",
    content: "Windowed turn history oldest",
    createdAt: "2026-10-01T00:00:01Z",
    newTurn: true,
    turnStartedAt: "2026-10-01T00:00:00Z",
    turnMetadata: { model: "window-test" },
  });
  await apiClient.seedAgentMessages(session.session_id, 104, "Windowed turn history");

  const capture = captureMessageWindows(testPage);
  const fullTurnRequests: string[] = [];
  testPage.on("request", (request) => {
    if (request.url().includes(`/api/v1/task-sessions/${session.session_id}/turns`)) {
      fullTurnRequests.push(request.url());
    }
  });
  await testPage.goto(`/t/${task.id}?sessionId=${session.session_id}`);
  await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible();
  expect(await bootMessageCount(testPage, session.session_id)).toBe(50);
  await waitForInitialMessageWindow(testPage, session.session_id);
  await expect(testPage.getByText("Windowed turn history 104", { exact: true })).toBeVisible();
  expect(fullTurnRequests).toEqual([]);

  const chat = testPage.locator(".chat-message-list:visible").first();
  await chat.evaluate((element) => {
    element.scrollTop = 0;
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
  });
  await expect(testPage.getByText("Windowed turn history oldest", { exact: true })).toBeVisible();
  await expect
    .poll(() => readMessageWindow(capture, session.session_id, true), {
      timeout: 15_000,
      message: "older-page reads should request and receive their own turn coverage",
    })
    .toBeTruthy();

  const olderWindow = readMessageWindow(capture, session.session_id, true);
  expect(olderWindow?.response.messages?.length).toBeLessThanOrEqual(50);
  expect(olderWindow?.response.turns).toHaveLength(1);
  expect(olderWindow?.response.turn_coverage?.message_ids).toContain(
    olderWindow?.response.messages?.[0]?.id,
  );
  expect(olderWindow?.response.turns?.[0]?.id).toBe(olderWindow?.response.messages?.[0]?.turn_id);
  expect(fullTurnRequests).toEqual([]);
});
