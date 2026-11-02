import { expect, test } from "../../fixtures/test-base";
import {
  assertOpenCodeCapacityTrace,
  assertSingleNoticeAcrossViewers,
  createOpenCodeCapacityFixture,
  driveOpenCodeCapacityRetries,
  expectOpenCodeCapacityNotice,
  expectOpenCodeContinuation,
  expectOpenCodeRetainedRefusal,
  readOpenCodeCapacityTrace,
} from "../../helpers/opencode-capacity-retry";
import { advanceCapacityRetries, waitForCapacityRetry } from "../../helpers/capacity-retry-clock";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(480_000);

function promptCount(tracePath: string) {
  return readOpenCodeCapacityTrace(tracePath).filter((record) => record.event === "prompt").length;
}

test("desktop continues after the fifth OpenCode admission refusal on the same runtime", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createOpenCodeCapacityFixture(
    backend,
    apiClient,
    seedData,
    "completed-tools",
    { initialPrompt: "/opencode-capacity-completed-tools:6" },
  );
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    const card = await expectOpenCodeCapacityNotice(session);
    await expect(session.activeChat().getByTestId("retry-window-deadline")).toBeVisible();
    await expect(card).toContainText("Attempt 1 of 11");

    // Advisory native retry metadata and a browser reload must not dispatch.
    await assertSingleNoticeAcrossViewers(testPage, session, fixture.taskId, () =>
      promptCount(fixture.tracePath),
    );

    await driveOpenCodeCapacityRetries(backend, apiClient, fixture.sessionId, 1, 5);
    await waitForCapacityRetry(apiClient, fixture.sessionId, 6);
    await expect(session.transientRetryCard()).toContainText("Attempt 6 of 11");
    await driveOpenCodeCapacityRetries(backend, apiClient, fixture.sessionId, 6, 6);

    await expectOpenCodeContinuation(session);
    await expect(
      session.activeChat().getByText("Updated fixture.txt before the provider admission refusal.", {
        exact: false,
      }),
    ).toBeVisible();
    assertOpenCodeCapacityTrace(fixture.tracePath, 7);
    const prompts = readOpenCodeCapacityTrace(fixture.tracePath).filter(
      (record) => record.event === "prompt",
    );
    expect(prompts.filter((record) => record.prompt?.includes("/opencode-capacity"))).toHaveLength(
      1,
    );
  } finally {
    await fixture.dispose();
  }
});

test("desktop preserves the episode when the OpenCode continuation fails without new tools", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createOpenCodeCapacityFixture(
    backend,
    apiClient,
    seedData,
    "completed-tools",
    { initialPrompt: "/opencode-capacity-completed-tools:100" },
  );
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expectOpenCodeCapacityNotice(session);

    await driveOpenCodeCapacityRetries(backend, apiClient, fixture.sessionId, 1, 11);
    const failure = await expectOpenCodeRetainedRefusal(apiClient, fixture.sessionId, "exhausted");
    expect(failure.metadata?.attempts_started).toBe(11);
    await expect(session.transientRetryCard()).toHaveCount(0);
    await expect(session.activeChat().getByTestId("retained-turn-recovery-feedback")).toContainText(
      "11",
    );
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.activeChat().getByTestId("retained-turn-recovery-feedback")).toBeVisible();
    await expect(session.transientRetryCard()).toHaveCount(0);
    assertOpenCodeCapacityTrace(fixture.tracePath, 12);
  } finally {
    await fixture.dispose();
  }
});

test("desktop cancels OpenCode capacity recovery and prevents the old timer from dispatching", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createOpenCodeCapacityFixture(
    backend,
    apiClient,
    seedData,
    "completed-tools",
    { initialPrompt: "/opencode-capacity-completed-tools:100" },
  );
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expectOpenCodeCapacityNotice(session);
    await driveOpenCodeCapacityRetries(backend, apiClient, fixture.sessionId, 1, 5);
    await waitForCapacityRetry(apiClient, fixture.sessionId, 6);
    await session.activeChat().getByTestId("recovery-cancel-retry-button").click();
    await expect(session.transientRetryCard()).toHaveCount(0);

    await advanceCapacityRetries(backend, 1800);
    await session.sendMessage("/e2e:simple-message");
    await expect(
      session
        .activeChat()
        .getByText("This is a simple mock response for e2e testing.", { exact: true }),
    ).toBeVisible();
    // The original request, five automatic dispatches, and one human prompt.
    assertOpenCodeCapacityTrace(fixture.tracePath, 7);
  } finally {
    await fixture.dispose();
  }
});

test("desktop keeps mixed completed and pending OpenCode work manual", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createOpenCodeCapacityFixture(backend, apiClient, seedData, "after-tools", {
    initialPrompt: "/opencode-capacity-after-tools",
  });
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expectOpenCodeRetainedRefusal(apiClient, fixture.sessionId, "refused");
    await expect(session.transientRetryCard()).toHaveCount(0);
    await expect(session.activeChat().getByTestId("session-recovery-action-message")).toContainText(
      "ttft_admission_refused",
    );
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    await advanceCapacityRetries(backend, 1800);
    assertOpenCodeCapacityTrace(fixture.tracePath, 1, { completedEffects: 0 });
  } finally {
    await fixture.dispose();
  }
});

test("desktop expires OpenCode capacity recovery without another dispatch", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createOpenCodeCapacityFixture(
    backend,
    apiClient,
    seedData,
    "completed-tools",
    { initialPrompt: "/opencode-capacity-completed-tools:100" },
  );
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expectOpenCodeCapacityNotice(session);
    await advanceCapacityRetries(backend, 1800);
    const failure = await expectOpenCodeRetainedRefusal(
      apiClient,
      fixture.sessionId,
      "window_expired",
    );
    expect(failure.metadata?.attempts_started).toBe(0);
    await expect(session.transientRetryCard()).toHaveCount(0);
    await expect(session.activeChat().getByTestId("retained-turn-recovery-feedback")).toContainText(
      "recovery window ended",
    );
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    assertOpenCodeCapacityTrace(fixture.tracePath, 1);
  } finally {
    await fixture.dispose();
  }
});
