import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  assertCompletedCapacityACPTrace,
  assertRetainedACPTrace,
  assertRetainedFailureMessage,
  createRetainedCapacityFixture,
  expectCompletedCapacityProgress,
  readMockACPTrace,
  waitForRetainedTurnFailure,
} from "../../helpers/transient-turn-runtime-continuity";
import { pollUntil } from "../../helpers/poll-until";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(480_000);

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

async function expectRetainedTurnReady(session: SessionPage) {
  const errorRow = session.activeChat().getByTestId("session-recovery-action-message");
  await expect(errorRow).toHaveCount(1);
  await expect(errorRow).toContainText("Selected model is at capacity.");
  await expect(session.recoveryResumeButton()).toHaveCount(0);
  await expect(session.recoveryFreshButton()).toHaveCount(0);
  await expect(session.activeChat().getByTestId("session-recovery-card")).toHaveCount(0);
  await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
  await expect(
    session.activeChat().getByRole("button", { name: "Session model settings" }),
  ).toBeVisible();
}

async function countRetainedFailures(
  apiClient: Parameters<typeof createRetainedCapacityFixture>[1],
  sessionId: string,
) {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.filter((message) => message.metadata?.runtime_retained === true);
}

test("desktop: capacity after tools keeps one error and the same runtime for model change and follow-up", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createRetainedCapacityFixture(backend, apiClient, seedData, "after-tools");
  const ws = watchWs(testPage);
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    const failure = await waitForRetainedTurnFailure(apiClient, fixture.sessionId, "refused");
    assertRetainedFailureMessage(failure);
    const executionId = await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId);
    await expectRetainedTurnReady(session);

    const details = session
      .activeChat()
      .getByTestId("session-recovery-action-message")
      .locator("details");
    await expect(details).toHaveCount(1);
    await details.locator("summary").click();
    await expect(details.locator("pre")).toContainText("acp_prompt");
    await expect(details.locator("pre")).toContainText("mock-agent");
    await expect(details.locator("pre")).toContainText("-32603");
    expect(await countRetainedFailures(apiClient, fixture.sessionId)).toHaveLength(1);
    assertRetainedACPTrace(fixture.tracePath, 1);

    await testPage.reload();
    await session.waitForLoad();
    await expectRetainedTurnReady(session);
    const viewer = await testPage.context().newPage();
    try {
      await viewer.goto(`/t/${fixture.taskId}`);
      const otherViewer = new SessionPage(viewer);
      await otherViewer.waitForLoad();
      await expectRetainedTurnReady(otherViewer);
    } finally {
      await viewer.close();
    }

    const modelTrigger = session
      .activeChat()
      .getByRole("button", { name: "Session model settings" });
    const modelsUpdated = ws.waitForEvent("session.models_updated", {
      where: (payload) =>
        payload.session_id === fixture.sessionId && payload.current_model_id === "mock-smart",
    });
    await modelTrigger.click();
    const modelList = testPage.getByRole("listbox");
    await expect(modelList).toBeVisible();
    await modelList.getByRole("option", { name: /Mock Smart/ }).click();
    await modelsUpdated;
    await expect(modelTrigger).toContainText("Mock Smart");
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
        return sessions.find((candidate) => candidate.id === fixture.sessionId)?.agent_execution_id;
      })
      .toBe(executionId);
    expect(
      readMockACPTrace(fixture.tracePath).some(
        (record) => record.event === "set_config_option" && record.value === "mock-smart",
      ),
    ).toBe(true);

    await expect(modelList).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(modelList).toBeHidden();
    await session.sendMessage("/e2e:simple-message");
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
      .poll(async () => (await countRetainedFailures(apiClient, fixture.sessionId)).length)
      .toBe(1);
    expect(await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId)).toBe(
      executionId,
    );
    assertRetainedACPTrace(fixture.tracePath, 2);
  } finally {
    await fixture.dispose();
  }
});

test("desktop: eligible retry stays on the live ACP runtime and does not duplicate a failure", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createRetainedCapacityFixture(backend, apiClient, seedData, "retry");
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    const executionId = await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId);
    await expect
      .poll(
        () =>
          readMockACPTrace(fixture.tracePath).filter((record) => record.event === "prompt").length,
        {
          timeout: 90_000,
        },
      )
      .toBe(2);
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
        return sessions.find((candidate) => candidate.id === fixture.sessionId)?.state;
      })
      .toBe("WAITING_FOR_INPUT");
    expect(await countRetainedFailures(apiClient, fixture.sessionId)).toHaveLength(0);
    expect(await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId)).toBe(
      executionId,
    );
    assertRetainedACPTrace(fixture.tracePath, 2);
    await expect(session.recoveryResumeButton()).toHaveCount(0);
  } finally {
    await fixture.dispose();
  }
});

test("desktop: completed tools continue once on the same runtime across reload and a second viewer", async ({
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
    await expectCompletedCapacityProgress(session);
    const executionId = await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId);

    await testPage.reload();
    await session.waitForLoad();
    await expectCompletedCapacityProgress(session);
    const viewer = await testPage.context().newPage();
    try {
      await viewer.goto(`/t/${fixture.taskId}`);
      const otherViewer = new SessionPage(viewer);
      await otherViewer.waitForLoad();
      await expectCompletedCapacityProgress(otherViewer);
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
    expect(await countRetainedFailures(apiClient, fixture.sessionId)).toHaveLength(0);
    assertCompletedCapacityACPTrace(fixture.tracePath, "/capacity-completed-tools");
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
  } finally {
    await fixture.dispose();
  }
});

test("desktop: idle cancellation and retry exhaustion preserve the live runtime with actual attempt counts", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  for (const scenario of ["cancel", "exhaust"]) {
    const fixture = await createRetainedCapacityFixture(
      backend,
      apiClient,
      seedData,
      scenario,
      scenario === "cancel" ? { initialPrompt: "/e2e:simple-message" } : {},
    );
    try {
      const session = new SessionPage(testPage);
      await testPage.goto(`/t/${fixture.taskId}`);
      await session.waitForLoad();
      const executionId = await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId);
      if (scenario === "cancel") {
        await session.sendMessage("/capacity-cancel");
        await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
        await session.recoveryCancelRetryButton().click();
      }
      const disposition = scenario === "cancel" ? "cancelled" : "exhausted";
      const failure = await waitForRetainedTurnFailure(apiClient, fixture.sessionId, disposition);
      assertRetainedFailureMessage(failure);
      expect(failure.metadata?.attempts_started).toBe(scenario === "cancel" ? 0 : 5);
      await expectRetainedTurnReady(session);
      const feedback = session.activeChat().getByTestId("retained-turn-recovery-feedback");
      await expect(feedback).toBeVisible();
      if (scenario === "exhaust") await expect(feedback).toContainText("5");
      expect(await countRetainedFailures(apiClient, fixture.sessionId)).toHaveLength(1);
      expect(await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId)).toBe(
        executionId,
      );
      assertRetainedACPTrace(fixture.tracePath, scenario === "cancel" ? 2 : 6);
    } finally {
      await fixture.dispose();
    }
  }
});
