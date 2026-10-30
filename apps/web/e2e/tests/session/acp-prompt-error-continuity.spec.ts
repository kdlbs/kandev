import { test, expect } from "../../fixtures/test-base";
import {
  assertPromptErrorACPTrace,
  createPromptErrorFixture,
  expectNoTurnErrorBadge,
  expectPromptErrorRow,
  expectTurnErrorBadge,
  GENERIC_PROMPT_ERROR,
  promptEventCount,
  waitForPromptError,
} from "../../helpers/acp-prompt-error-continuity";
import { SessionPage } from "../../pages/session-page";
import { openQuickChatSetup, selectAgentIfNeeded } from "../chat/quick-chat-helpers";
import { completeQuickChatOpening } from "../chat/quick-chat-opening-composer-helpers";
import { pollUntil } from "../../helpers/poll-until";

test.setTimeout(480_000);

async function waitForExecutionId(
  apiClient: Parameters<typeof createPromptErrorFixture>[1],
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

test("generic prompt error keeps the runtime and accepts a follow-up prompt", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createPromptErrorFixture(backend, apiClient, seedData);
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();

    const failure = await waitForPromptError(apiClient, fixture.sessionId);
    expect(failure.content).toContain(GENERIC_PROMPT_ERROR);
    expect(failure.metadata).toMatchObject({
      variant: "error",
      failure_scope: "turn",
      runtime_retained: true,
      recovery_actions: false,
    });
    const executionId = await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId);

    await expectPromptErrorRow(session);
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await expect(session.activeChat().getByTestId("session-recovery-card")).toHaveCount(0);
    await expectTurnErrorBadge(session, fixture.taskTitle);

    await session.sendMessage("/e2e:simple-message");
    await expect.poll(() => promptEventCount(fixture.tracePath), { timeout: 60_000 }).toBe(2);
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
        const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
        return sessions.find((candidate) => candidate.id === fixture.sessionId)?.state;
      })
      .toBe("WAITING_FOR_INPUT");
    expect(await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId)).toBe(
      executionId,
    );
    assertPromptErrorACPTrace(fixture.tracePath, 2);

    // The accepted prompt acknowledged the observed error, but the historical
    // row and its readable text remain.
    await expectNoTurnErrorBadge(session, fixture.taskTitle);
    await expectPromptErrorRow(session, { dismiss: false });
  } finally {
    await fixture.dispose();
  }
});

test("focus preserves the task badge and inline dismissal survives reload", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createPromptErrorFixture(backend, apiClient, seedData);
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await waitForPromptError(apiClient, fixture.sessionId);

    await expectPromptErrorRow(session);
    await expectTurnErrorBadge(session, fixture.taskTitle);
    await session.sidebarTaskItem(fixture.taskTitle).first().click();
    await expectTurnErrorBadge(session, fixture.taskTitle);

    await testPage.reload();
    await session.waitForLoad();
    const row = await expectPromptErrorRow(session);
    await row.getByTestId("turn-error-dismiss-button").click();
    await expect(row.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
    await expect(row).toBeVisible();
    await expectNoTurnErrorBadge(session, fixture.taskTitle);

    await testPage.reload();
    await session.waitForLoad();
    const reloaded = session.activeChat().getByTestId("session-recovery-action-message");
    await expect(reloaded).toBeVisible();
    await expect(reloaded.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
    await expectNoTurnErrorBadge(session, fixture.taskTitle);
    const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
    expect(messages.some((message) => message.content?.includes(GENERIC_PROMPT_ERROR))).toBe(true);
  } finally {
    await fixture.dispose();
  }
});

test("task context menu dismisses only the observed turn error", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createPromptErrorFixture(backend, apiClient, seedData);
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await waitForPromptError(apiClient, fixture.sessionId);

    const row = session.sidebarTaskItem(fixture.taskTitle).first();
    await expect(row).toBeVisible();
    await row.click({ button: "right" });
    const dismiss = testPage.getByTestId("task-context-dismiss-turn-error");
    await expect(dismiss).toBeVisible();
    await dismiss.click();

    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
        return messages.some((message) => message.content?.includes(GENERIC_PROMPT_ERROR));
      })
      .toBe(true);
    await expect(
      session
        .activeChat()
        .getByTestId("session-recovery-action-message")
        .getByTestId("turn-error-dismiss-button"),
    ).toHaveCount(0);
    await expect(session.activeChat().getByTestId("session-recovery-action-message")).toBeVisible();
  } finally {
    await fixture.dispose();
  }
});

test("task-backed Quick Chat keeps retained attention without board rows", async ({
  testPage,
  apiClient,
}) => {
  const dialog = await openQuickChatSetup(testPage);
  await selectAgentIfNeeded(dialog, testPage);
  const started = await completeQuickChatOpening(testPage, dialog, apiClient, "/prompt-error:1", {
    submit: (button) => button.click(),
    expectExactPrompt: true,
  });
  await waitForPromptError(apiClient, started.session_id);

  const row = dialog.getByTestId("session-recovery-action-message");
  await expect(row).toHaveCount(1, { timeout: 30_000 });
  const dismiss = row.getByTestId("turn-error-dismiss-button");
  await expect(dismiss).toBeVisible({ timeout: 30_000 });

  const collections = await testPage.evaluate((taskId) => {
    const store = (
      window as unknown as {
        __KANDEV_E2E_STORE__?: {
          getState: () => {
            kanban: { tasks: Array<{ id: string }> };
            taskOverview: { byId: Record<string, unknown> };
            sidebarStatusSummaryByWorkspaceId?: Record<string, Record<string, unknown>>;
          };
        };
      }
    ).__KANDEV_E2E_STORE__;
    const state = store?.getState();
    return {
      inBoard: state?.kanban.tasks.some((task) => task.id === taskId) ?? null,
      inOverview: Boolean(state?.taskOverview.byId[taskId]),
      inRetainedCache: Object.values(state?.sidebarStatusSummaryByWorkspaceId ?? {}).some(
        (summaries) => summaries[taskId] != null,
      ),
    };
  }, started.task_id);
  expect(collections.inBoard).toBe(false);
  expect(collections.inOverview).toBe(false);
  expect(collections.inRetainedCache).toBe(true);

  await dismiss.click();
  await expect(dismiss).toHaveCount(0);
  await expect(row).toBeVisible();

  await testPage.reload();
  await testPage.waitForLoadState("networkidle");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await testPage.keyboard.press(`${modifier}+Shift+q`);
  await expect(dialog).toBeVisible({ timeout: 15_000 });
  const sessionTab = dialog.locator(
    `[data-tab-reference="conversation:${started.session_id}"] [data-testid="quick-chat-tab"]`,
  );
  await expect(sessionTab).toBeVisible({ timeout: 15_000 });
  await sessionTab.click();
  const reloadedRow = dialog.getByTestId("session-recovery-action-message");
  await expect(reloadedRow).toBeVisible({ timeout: 30_000 });
  await expect(reloadedRow.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
});
