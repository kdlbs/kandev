import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  bootMessageCount,
  captureMessageWindows,
  readMessageWindow,
  waitForInitialMessageWindow,
} from "./message-turn-window-capture";

test("phone boot and history paging preserve bounded turn context", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(150_000);
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile message turn window", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const session = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    sessionId: `mobile-message-turn-window-${task.id}`,
    agentProfileId: seedData.agentProfileId,
    startedAt: "2026-10-01T00:00:00Z",
  });
  await apiClient.seedSessionMessage(session.session_id, {
    type: "message",
    content: "Mobile windowed turn history oldest",
    createdAt: "2026-10-01T00:00:01Z",
    newTurn: true,
    turnStartedAt: "2026-10-01T00:00:00Z",
  });
  await apiClient.seedAgentMessages(session.session_id, 104, "Mobile windowed turn history");

  const capture = captureMessageWindows(testPage);
  await testPage.goto(`/t/${task.id}?sessionId=${session.session_id}`);
  const layout = testPage.getByTestId("mobile-task-layout");
  await expect(layout).toBeVisible();
  expect(await bootMessageCount(testPage, session.session_id)).toBe(50);
  await waitForInitialMessageWindow(testPage, session.session_id);
  await expect(
    testPage.getByText("Mobile windowed turn history 104", { exact: true }),
  ).toBeVisible();
  await assertNoDocumentHorizontalOverflow(testPage, "phone message turn window");

  const chat = testPage.locator(".chat-message-list:visible").first();
  await chat.evaluate((element) => {
    element.scrollTop = 0;
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
  });
  await expect(
    testPage.getByText("Mobile windowed turn history oldest", { exact: true }),
  ).toBeVisible();
  await expect
    .poll(() => readMessageWindow(capture, session.session_id, true), {
      timeout: 15_000,
      message: "phone history paging should include the bounded turn window",
    })
    .toBeTruthy();

  const olderWindow = readMessageWindow(capture, session.session_id, true);
  expect(olderWindow?.response.messages?.length).toBeLessThanOrEqual(50);
  expect(olderWindow?.response.turns).toHaveLength(1);
  expect(olderWindow?.response.turn_coverage?.message_ids).toContain(
    olderWindow?.response.messages?.[0]?.id,
  );
  await assertNoDocumentHorizontalOverflow(testPage, "phone message turn window after paging");
});
