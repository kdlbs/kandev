import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  assertCompletedCapacityACPTrace,
  assertRetainedACPTrace,
  assertRetainedFailureMessage,
  createRetainedCapacityFixture,
  readMockACPTrace,
  waitForRetainedTurnFailure,
} from "../../helpers/transient-turn-runtime-continuity";
import { pollUntil } from "../../helpers/poll-until";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(300_000);

async function waitForExecutionId(
  apiClient: Parameters<typeof createRetainedCapacityFixture>[1],
  taskId: string,
  sessionId: string,
) {
  return pollUntil(
    async () => {
      const { sessions } = await apiClient.listTaskSessions(taskId);
      return sessions.find((candidate) => candidate.id === sessionId)?.agent_execution_id ?? "";
    },
    (executionId) => executionId.length > 0,
    30_000,
    "waiting for the task session execution identity",
  );
}

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

test("phone: completed tools continue on the same runtime across reload and a second viewer", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createRetainedCapacityFixture(
    backend,
    apiClient,
    seedData,
    "completed-tools",
  );
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toContainText("Continuing in");
    await expect(session.recoveryCancelRetryButton()).toHaveCount(1);
    const cancelBounds = await session.recoveryCancelRetryButton().boundingBox();
    expect(cancelBounds?.height).toBeGreaterThanOrEqual(44);
    expect(cancelBounds?.width).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage);

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
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    const viewer = await testPage.context().newPage();
    try {
      await viewer.goto(`/t/${fixture.taskId}`);
      const otherViewer = new SessionPage(viewer);
      await otherViewer.waitForLoad();
      await expect(otherViewer.transientRetryCard()).toBeVisible();
      await expect(otherViewer.transientRetryCard()).toContainText("Continuing");
    } finally {
      await viewer.close();
    }

    await expect
      .poll(
        () =>
          readMockACPTrace(fixture.tracePath).filter((record) => record.event === "prompt").length,
        {
          timeout: 60_000,
        },
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
            message.content?.includes(
              "continued the unfinished request without repeating completed work",
            ) === true,
        ).length;
      })
      .toBeGreaterThan(0);
    expect(await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId)).toBe(
      executionId,
    );
    assertCompletedCapacityACPTrace(fixture.tracePath, "/capacity-completed-tools");
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});

test("phone: capacity retry can be cancelled during backoff", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createRetainedCapacityFixture(backend, apiClient, seedData, "cancel");
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toContainText("Retrying in");
    await expect(session.recoveryCancelRetryButton()).toBeVisible();
    const bounds = await session.recoveryCancelRetryButton().boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
    expect(bounds?.width).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage);
    await session.recoveryCancelRetryButton().tap();

    const failure = await waitForRetainedTurnFailure(apiClient, fixture.sessionId, "cancelled");
    assertRetainedFailureMessage(failure);
    expect(failure.metadata?.attempts_started).toBe(0);
    await expect(session.transientRetryCard()).toBeHidden();
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    assertRetainedACPTrace(fixture.tracePath, 1);
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});

test("phone: capacity continuation exhausts after five dispatches and keeps Chat available", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createRetainedCapacityFixture(backend, apiClient, seedData, "exhaust");
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
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
    const failure = await waitForRetainedTurnFailure(apiClient, fixture.sessionId, "exhausted");
    assertRetainedFailureMessage(failure);
    expect(failure.metadata?.attempts_started).toBe(5);
    await expect(session.transientRetryCard()).toBeHidden();
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    const feedback = session.activeChat().getByTestId("retained-turn-recovery-feedback");
    await expect(feedback).toBeVisible();
    await expect(feedback).toContainText("5");
    expect(
      await pollUntil(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
          return (
            sessions.find((candidate) => candidate.id === fixture.sessionId)?.agent_execution_id ??
            ""
          );
        },
        (value) => value.length > 0,
        30_000,
        "waiting for the retained task session execution identity",
      ),
    ).toBe(executionId);
    assertRetainedACPTrace(fixture.tracePath, 6);
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});
