// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  assertOneTurnStartPromptAndResponse,
  cleanupResumeTurnStartFixture,
  seedResumeTurnStartFixture,
  waitForResumeQueuedPrompt,
  waitForResumeSessionReady,
  waitForResumeTurnStartTransition,
} from "../../helpers/session-resume-turn-start";
import {
  countResumeBootMessages,
  waitForQueuedCount,
  waitForSessionStarting,
} from "../../helpers/session-resume-prompt-queue";

test.describe("mobile: message during resumed workflow turn start", () => {
  test.describe.configure({ retries: 0 });

  test("taps Send during startup and delivers after resume readiness", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const fixture = await seedResumeTurnStartFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      "Mobile resume turn-start auto-run",
    );
    const marker = "mobile resume turn-start marker";

    try {
      const chat = fixture.session.activeChat();
      const editor = chat.getByTestId("chat-input-editor");
      const submit = fixture.session.submitButton();
      await expect(editor).toHaveAttribute("contenteditable", "true");
      await editor.fill(`e2e:message("${marker}")`);
      await expect(submit).toBeEnabled();

      const box = await submit.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      await expect(submit).toBeInViewport();
      await submit.tap();

      await expect(editor).toHaveText("");
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await waitForResumeQueuedPrompt(apiClient, fixture, marker);
      await waitForSessionStarting(
        testPage,
        apiClient,
        fixture.task.id,
        fixture.identity.sessionId,
      );
      await waitForResumeTurnStartTransition(testPage, apiClient, fixture, 0, 2_000);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile resume turn-start startup");

      await waitForResumeSessionReady(
        testPage,
        apiClient,
        fixture.task.id,
        fixture.identity.sessionId,
      );
      await waitForQueuedCount(apiClient, fixture.identity, 0);
      await waitForResumeTurnStartTransition(testPage, apiClient, fixture, 1);

      const response = fixture.session
        .activeChat()
        .locator("[data-agent-message-body][data-message-id]")
        .filter({ hasText: marker });
      await expect(response).toHaveCount(1, { timeout: 60_000 });
      await assertOneTurnStartPromptAndResponse(apiClient, fixture, marker);
      await waitForResumeTurnStartTransition(testPage, apiClient, fixture, 1);
      await expect
        .poll(() => countResumeBootMessages(apiClient, fixture.identity.sessionId))
        .toBe(1);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile resumed turn-start response");
    } finally {
      await cleanupResumeTurnStartFixture(apiClient, fixture);
    }
  });

  test("keeps a startup tap pending while Auto-run is off", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const fixture = await seedResumeTurnStartFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      "Mobile resume turn-start paused queue",
    );
    const marker = "mobile paused resume turn-start marker";

    try {
      await expect(apiClient.setQueueAutoRun(fixture.identity, false)).resolves.toMatchObject({
        auto_run: false,
      });
      const chat = fixture.session.activeChat();
      const editor = chat.getByTestId("chat-input-editor");
      const submit = fixture.session.submitButton();
      await expect(editor).toHaveAttribute("contenteditable", "true");
      await editor.fill(`e2e:message("${marker}")`);
      await expect(submit).toBeEnabled();
      await expect(submit).toBeInViewport();
      await submit.tap();

      await expect(editor).toHaveText("");
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await waitForResumeQueuedPrompt(apiClient, fixture, marker);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile paused resume turn-start queue");

      await testPage.reload();
      await fixture.session.waitForLoad();
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await waitForResumeQueuedPrompt(apiClient, fixture, marker);
      await expect(fixture.session.activeChat().getByTestId("queue-chip")).toBeVisible();
      await waitForResumeSessionReady(
        testPage,
        apiClient,
        fixture.task.id,
        fixture.identity.sessionId,
      );
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await waitForResumeTurnStartTransition(testPage, apiClient, fixture, 0, 2_000);

      const readyChat = fixture.session.activeChat();
      await readyChat.getByTestId("queue-chip").click();
      const autoRun = readyChat.getByTestId("queue-auto-run");
      await expect(autoRun).toHaveAttribute("data-state", "unchecked");
      await autoRun.click();

      await waitForQueuedCount(apiClient, fixture.identity, 0);
      await waitForResumeTurnStartTransition(testPage, apiClient, fixture, 1);
      const response = fixture.session
        .activeChat()
        .locator("[data-agent-message-body][data-message-id]")
        .filter({ hasText: marker });
      await expect(response).toHaveCount(1, { timeout: 60_000 });
      await assertOneTurnStartPromptAndResponse(apiClient, fixture, marker);
      await waitForResumeTurnStartTransition(testPage, apiClient, fixture, 1);
      await expect
        .poll(() => countResumeBootMessages(apiClient, fixture.identity.sessionId))
        .toBe(1);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile enabled resume turn-start queue");
    } finally {
      await cleanupResumeTurnStartFixture(apiClient, fixture);
    }
  });
});
