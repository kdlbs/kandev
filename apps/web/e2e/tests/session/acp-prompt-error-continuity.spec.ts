import { test, expect } from "../../fixtures/test-base";
import {
  assertPromptErrorACPTrace,
  createPromptErrorFixture,
  expectNoTurnErrorBadge,
  expectPromptErrorRow,
  expectTurnErrorBadge,
  GENERIC_PROMPT_ERROR,
  originalPromptOccurrences,
  promptEventCount,
  waitForPromptError,
  waitForAgentResponseAfter,
  waitForUserContinueMessages,
} from "../../helpers/acp-prompt-error-continuity";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { KanbanPage } from "../../pages/kanban-page";
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

test("desktop: Dismiss and continue sends one manual continue and preserves the draft", async ({
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
    const executionId = await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId);

    await session.waitForDirectInput();
    const row = await expectPromptErrorRow(session, { continue: true });
    await expectTurnErrorBadge(session, fixture.taskTitle);
    const editor = await session.fillMessage("keep this unsent draft");
    await row.getByTestId("turn-error-continue-button").click();

    await expect.poll(() => promptEventCount(fixture.tracePath), { timeout: 60_000 }).toBe(2);
    const continueMessages = await waitForUserContinueMessages(apiClient, fixture.sessionId);
    expect(continueMessages).toHaveLength(1);
    await waitForAgentResponseAfter(apiClient, fixture.sessionId, continueMessages[0].created_at);
    expect(originalPromptOccurrences(fixture.tracePath, "/prompt-error:1")).toBe(1);
    expect(await waitForExecutionId(apiClient, fixture.taskId, fixture.sessionId)).toBe(
      executionId,
    );
    await expectNoTurnErrorBadge(session, fixture.taskTitle);
    await expect(row.getByTestId("turn-error-continue-button")).toHaveCount(0);
    await expect(row.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
    await expect(editor).toHaveText("keep this unsent draft");
    assertPromptErrorACPTrace(fixture.tracePath, 2);

    await testPage.reload();
    await session.waitForLoad();
    await expectPromptErrorRow(session, { dismiss: false, continue: false });
    await expectNoTurnErrorBadge(session, fixture.taskTitle);
    const viewer = await testPage.context().newPage();
    try {
      await viewer.goto(`/t/${fixture.taskId}`);
      const otherViewer = new SessionPage(viewer);
      await otherViewer.waitForLoad();
      await expectPromptErrorRow(otherViewer, { dismiss: false, continue: false });
      await expect(
        otherViewer.activeChat().getByText("continue", { exact: true }).first(),
      ).toBeVisible();
    } finally {
      await viewer.close();
    }
    const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
    expect(
      messages.filter(
        (message) => message.author_type === "user" && message.content === "continue",
      ),
    ).toHaveLength(1);
  } finally {
    await fixture.dispose();
  }
});

test("desktop: two viewers submitting at once create one continuation", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createPromptErrorFixture(backend, apiClient, seedData);
  const viewer = await testPage.context().newPage();
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await waitForPromptError(apiClient, fixture.sessionId);
    await session.waitForDirectInput();
    const firstRow = await expectPromptErrorRow(session, { continue: true });

    await viewer.goto(`/t/${fixture.taskId}`);
    const otherViewer = new SessionPage(viewer);
    await otherViewer.waitForLoad();
    await otherViewer.waitForDirectInput();
    const secondRow = await expectPromptErrorRow(otherViewer, { continue: true });

    await Promise.allSettled([
      firstRow.getByTestId("turn-error-continue-button").evaluate((element) => {
        (element as HTMLButtonElement).click();
      }),
      secondRow.getByTestId("turn-error-continue-button").evaluate((element) => {
        (element as HTMLButtonElement).click();
      }),
    ]);
    const continued = await waitForUserContinueMessages(apiClient, fixture.sessionId);
    expect(continued).toHaveLength(1);
    await expect.poll(() => promptEventCount(fixture.tracePath), { timeout: 60_000 }).toBe(2);
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
        return messages.filter(
          (message) => message.author_type === "user" && message.content === "continue",
        );
      })
      .toHaveLength(1);
    await expect(firstRow.getByTestId("turn-error-continue-button")).toHaveCount(0);
    await expect(secondRow.getByTestId("turn-error-continue-button")).toHaveCount(0);
  } finally {
    await viewer.close();
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

test("task-backed Quick Chat sends Dismiss and continue from the retained alert", async ({
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
  const continueButton = row.getByTestId("turn-error-continue-button");
  await expect(continueButton).toBeVisible({ timeout: 30_000 });
  await continueButton.click();

  const continued = await waitForUserContinueMessages(apiClient, started.session_id);
  expect(continued).toHaveLength(1);
  await waitForAgentResponseAfter(apiClient, started.session_id, continued[0].created_at);
  await expect(continueButton).toHaveCount(0);
  await expect(row.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
  await expect(row).toBeVisible();
});

test("preview Chat continues the current error", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createPromptErrorFixture(backend, apiClient, seedData);
  try {
    await waitForPromptError(apiClient, fixture.sessionId);
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle(fixture.taskTitle);
    await expect(card.getByRole("button", { name: "Open full page" })).toBeVisible({
      timeout: 15_000,
    });
    await card.click();
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 15_000 });

    const preview = testPage.getByTestId("task-preview-panel");
    const continueButton = preview.getByTestId("turn-error-continue-button");
    await expect(continueButton).toBeVisible({ timeout: 30_000 });
    await continueButton.click();
    const continued = await waitForUserContinueMessages(apiClient, fixture.sessionId);
    expect(continued).toHaveLength(1);
    await waitForAgentResponseAfter(apiClient, fixture.sessionId, continued[0].created_at);
    await expect(continueButton).toHaveCount(0);
    await expect(preview.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
  } finally {
    await apiClient.saveUserSettings({ enable_preview_on_click: false });
    await fixture.dispose();
  }
});

test.describe("narrow fine-pointer error controls", () => {
  test.use({ viewport: { width: 600, height: 900 } });

  test("stacks Dismiss and continue with pseudo-localized labels and no overflow", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const fixture = await createPromptErrorFixture(backend, apiClient, seedData);
    try {
      await testPage
        .context()
        .addCookies([{ name: "kandev_locale", value: "pseudo", url: backend.baseUrl }]);
      const session = new SessionPage(testPage);
      await testPage.goto(`/t/${fixture.taskId}`);
      await session.waitForLoad();
      await waitForPromptError(apiClient, fixture.sessionId);

      const row = session.activeChat().getByTestId("session-recovery-action-message");
      const continueButton = row.getByTestId("turn-error-continue-button");
      const dismissButton = row.getByTestId("turn-error-dismiss-button");
      await expect(continueButton).toBeVisible({ timeout: 30_000 });
      await expect(testPage.locator("html")).toHaveAttribute("lang", "pseudo");

      const continueBox = await continueButton.boundingBox();
      const dismissBox = await dismissButton.boundingBox();
      expect(continueBox?.height ?? 0).toBeGreaterThanOrEqual(44);
      expect(dismissBox?.height ?? 0).toBeGreaterThanOrEqual(44);
      expect(continueBox?.y ?? 0).toBeLessThan(dismissBox?.y ?? 0);
      expect(continueBox?.width ?? 0).toBeLessThanOrEqual(600);
      await assertNoDocumentHorizontalOverflow(testPage);
      expect((await continueButton.textContent())?.trim().length ?? 0).toBeGreaterThan(0);
    } finally {
      await fixture.dispose();
    }
  });
});
