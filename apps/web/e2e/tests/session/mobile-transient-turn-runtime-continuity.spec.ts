import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  assertRetainedACPTrace,
  assertRetainedFailureMessage,
  createRetainedCapacityFixture,
  readMockACPTrace,
  waitForRetainedTurnFailure,
} from "../../helpers/transient-turn-runtime-continuity";
import { pollUntil } from "../../helpers/poll-until";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(180_000);

test("phone: capacity after tools shows one inline error and keeps the composer and runtime usable", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createRetainedCapacityFixture(backend, apiClient, seedData, "after-tools");
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    const failure = await waitForRetainedTurnFailure(apiClient, fixture.sessionId, "refused");
    assertRetainedFailureMessage(failure);
    const executionId = await pollUntil(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
        return (
          sessions.find((candidate) => candidate.id === fixture.sessionId)?.agent_execution_id ?? ""
        );
      },
      (value) => value.length > 0,
      30_000,
      "waiting for the task session execution identity",
    );

    const errorRows = session.activeChat().getByTestId("session-recovery-action-message");
    await expect(errorRows).toHaveCount(1);
    await expect(errorRows).toContainText("Selected model is at capacity.");
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await expect(session.activeChat().getByTestId("session-recovery-card")).toHaveCount(0);
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    await expect(
      session.activeChat().getByRole("button", { name: "Session model settings" }),
    ).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    const details = errorRows.locator("details");
    await expect(details).toHaveCount(1);
    await details.locator("summary").tap();
    await expect(details.locator("pre")).toContainText("acp_prompt");
    await expect(details.locator("pre")).toContainText("-32603");
    await assertNoDocumentHorizontalOverflow(testPage);

    await session.sendMessageViaButton("/e2e:simple-message");
    await expect
      .poll(
        () =>
          readMockACPTrace(fixture.tracePath).filter((record) => record.event === "prompt").length,
      )
      .toBe(2);
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
        return sessions.find((candidate) => candidate.id === fixture.sessionId)?.state;
      })
      .toBe("WAITING_FOR_INPUT");
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
        return messages.filter(
          (message) =>
            message.author_type === "agent" &&
            message.content?.includes("simple mock response") === true,
        ).length;
      })
      .toBeGreaterThan(0);
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
        return messages.filter((message) => message.metadata?.runtime_retained === true).length;
      })
      .toBe(1);
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
        return sessions.find((candidate) => candidate.id === fixture.sessionId)?.agent_execution_id;
      })
      .toBe(executionId);
    assertRetainedACPTrace(fixture.tracePath, 2);
    await assertNoDocumentHorizontalOverflow(testPage);

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.activeChat().getByTestId("session-recovery-action-message")).toHaveCount(
      1,
    );
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});
