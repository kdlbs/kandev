// Filename starts with "mobile-" so it runs on the mobile-chrome Playwright
// project (Pixel 5 emulation) — see e2e/playwright.config.ts. The transient
// retry flow must keep its yellow card and Cancel action usable on a phone.
import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { listTransientRetryNotices } from "../../helpers/transient-retry";
import { openMobileQuickChatSetup } from "../chat/quick-chat-saved-prompt-delivery-helpers";
import { completeQuickChatOpening } from "../chat/quick-chat-opening-composer-helpers";
import { prepareMockCodexProfile, selectAgentIfNeeded } from "../chat/quick-chat-helpers";

test.describe("mobile: transient provider error retry", () => {
  test("Quick Chat opening prompt recovers from a Codex 429 after reload", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);
    await prepareMockCodexProfile(apiClient, backend);
    await testPage.setViewportSize({ width: 390, height: 844 });
    const dialog = await openMobileQuickChatSetup(testPage);
    await selectAgentIfNeeded(dialog, testPage, "Mock Codex");
    const prompt = "/codex-rate-limit:2";
    const started = await completeQuickChatOpening(testPage, dialog, apiClient, prompt, {
      submit: (button) => button.tap(),
      expectExactPrompt: true,
    });

    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, started.session_id)).length, {
        timeout: 45_000,
        message: "waiting for the Codex Quick Chat retry notice",
      })
      .toBe(1);
    const [firstNotice] = await listTransientRetryNotices(apiClient, started.session_id);
    if (!firstNotice) throw new Error("the persisted Codex retry notice was not found");
    expect(await listTransientRetryNotices(apiClient, started.session_id)).toEqual([
      expect.objectContaining({ id: firstNotice.id, attempt: 1 }),
    ]);
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(started.task_id);
        return sessions.find((session) => session.id === started.session_id)?.state;
      })
      .toBe("WAITING_FOR_INPUT");
    await expect(testPage.getByTestId("transient-retry-card")).toBeVisible();
    await expect(dialog.locator(".tiptap.ProseMirror:visible").last()).toBeEmpty();
    await assertNoDocumentHorizontalOverflow(testPage);

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    await expect
      .poll(
        async () =>
          (await apiClient.listSessionMessages(started.session_id)).messages.filter(
            (message) => message.author_type === "user" && message.content === prompt,
          ).length,
      )
      .toBe(1);
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(started.session_id);
          return messages.some(
            (message) =>
              message.author_type === "agent" &&
              message.content.includes("Codex rate limit cleared after 2 transient failure(s)"),
          );
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    const finalMessages = (await apiClient.listSessionMessages(started.session_id)).messages;
    expect(finalMessages.filter((message) => message.author_type === "user")).toEqual([
      expect.objectContaining({ content: prompt }),
    ]);
    expect(await listTransientRetryNotices(apiClient, started.session_id)).toHaveLength(0);
    await assertNoDocumentHorizontalOverflow(testPage);
  });

  test("phone Cancel stops the retry for a Codex 429 opening prompt", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);
    await prepareMockCodexProfile(apiClient, backend);
    await testPage.setViewportSize({ width: 390, height: 844 });
    const dialog = await openMobileQuickChatSetup(testPage);
    await selectAgentIfNeeded(dialog, testPage, "Mock Codex");
    const prompt = "/codex-rate-limit:9";
    const started = await completeQuickChatOpening(testPage, dialog, apiClient, prompt, {
      submit: (button) => button.tap(),
      expectExactPrompt: true,
    });

    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, started.session_id)).length)
      .toBe(1);
    const [firstNotice] = await listTransientRetryNotices(apiClient, started.session_id);
    if (!firstNotice) throw new Error("the persisted Codex retry notice was not found");
    expect(firstNotice.attempt).toBe(1);
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(started.task_id);
        return sessions.find((session) => session.id === started.session_id)?.state;
      })
      .toBe("WAITING_FOR_INPUT");
    await expect(testPage.getByTestId("transient-retry-card")).toBeVisible();
    await expect(dialog.locator(".tiptap.ProseMirror:visible").last()).toBeEmpty();
    await testPage.getByTestId("recovery-cancel-retry-button").tap();

    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, started.session_id)).length)
      .toBe(0);
    await expect(
      testPage.getByText("Automatic retry was cancelled. You can send another message.", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(dialog.locator(".tiptap.ProseMirror:visible").last()).toBeEditable();
    const beforeReload = (await apiClient.listSessionMessages(started.session_id)).messages;
    expect(beforeReload.filter((message) => message.author_type === "user")).toEqual([
      expect.objectContaining({ content: prompt }),
    ]);

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    const afterReload = (await apiClient.listSessionMessages(started.session_id)).messages;
    expect(afterReload.filter((message) => message.author_type === "user")).toEqual([
      expect.objectContaining({ content: prompt }),
    ]);
    expect(await listTransientRetryNotices(apiClient, started.session_id)).toHaveLength(0);
    await assertNoDocumentHorizontalOverflow(testPage);
  });

  test("yellow retry card + Cancel works on mobile and surfaces recovery", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Mobile Overloaded Test");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");

    await session.sendMessageViaButton("/overloaded:9");
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, sessionId)).length, {
        timeout: 10_000,
        intervals: [250],
        message: "the visible transient retry notice should be persisted",
      })
      .toBe(1);
    const [retryNotice] = await listTransientRetryNotices(apiClient, sessionId);
    if (!retryNotice) throw new Error("the persisted transient retry notice was not found");
    const retryNoticeId = retryNotice.id;

    // Yellow retry card + Cancel button render on the narrow viewport.
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.recoveryCancelRetryButton()).toBeVisible();
    await expect(session.recoveryResumeButton()).toBeHidden();

    expect(retryNotice.attempt).toBeGreaterThanOrEqual(1);
    await assertNoDocumentHorizontalOverflow(testPage);

    // Tap Cancel → red recovery banner.
    await session.recoveryCancelRetryButton().tap();
    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, sessionId)).length, {
        timeout: 30_000,
        message: `retry notice ${retryNoticeId} should be deleted after cancel`,
      })
      .toBe(0);
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toBeHidden();
    await expect(testPage.getByTestId("session-recovery-card")).toContainText(
      "Automatic recovery was cancelled. Resume or start fresh to continue.",
    );
    await expect(testPage.getByTestId("session-recovery-card")).not.toContainText(
      "after several retries",
    );
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
