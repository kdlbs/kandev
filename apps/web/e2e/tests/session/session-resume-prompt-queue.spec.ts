import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  cleanupDelayedResumeFixture,
  seedDelayedResumeFixture,
  waitForSessionReady,
  waitForQueuedCount,
} from "../../helpers/session-resume-prompt-queue";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { openQuickChatSetup, selectAgentIfNeeded } from "../chat/quick-chat-helpers";

test.describe("Send during session resume", () => {
  test.describe.configure({ retries: 0 });

  test("keeps a startup prompt pending when Auto-run is off", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const fixture = await seedDelayedResumeFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      "Resume prompt queue Auto-run test",
    );
    const marker = "resume queue paused marker";

    try {
      await expect(apiClient.setQueueAutoRun(fixture.identity, false)).resolves.toMatchObject({
        auto_run: false,
      });

      const editor = fixture.session.activeChat().getByTestId("chat-input-editor");
      const submit = fixture.session.submitButton();
      await expect(editor).toHaveAttribute("contenteditable", "true");
      await editor.fill(`e2e:message("${marker}")`);
      await expect(submit).toBeEnabled();
      await submit.click();

      await expect(editor).toHaveText("");
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await waitForSessionReady(testPage, apiClient, fixture.task.id, fixture.identity.sessionId);
      await waitForQueuedCount(apiClient, fixture.identity, 1, 30_000);

      const chat = fixture.session.activeChat();
      await chat.getByTestId("queue-chip").click();
      const autoRun = chat.getByTestId("queue-auto-run");
      await expect(autoRun).toHaveAttribute("data-state", "unchecked");
      await autoRun.click();

      await expect
        .poll(() => apiClient.getQueueStatus(fixture.identity).then((status) => status.count), {
          timeout: 30_000,
        })
        .toBe(0);
      const response = chat
        .locator("[data-agent-message-body][data-message-id]")
        .filter({ hasText: marker });
      await expect(response).toHaveCount(1, { timeout: 60_000 });
    } finally {
      await cleanupDelayedResumeFixture(apiClient, fixture);
    }
  });

  test("uses the shared startup composer in Quick Chat", async ({ testPage, apiClient }) => {
    test.setTimeout(120_000);

    const dialog = await openQuickChatSetup(testPage);
    await selectAgentIfNeeded(dialog, testPage);
    const started = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname.endsWith("/quick-chat"),
    );
    await dialog.getByTestId("quick-chat-start").click();
    const startResponse = await started;
    const { task_id: taskId, session_id: sessionId } = (await startResponse.json()) as {
      task_id: string;
      session_id: string;
    };

    const identity = await apiClient.getQueueSessionIdentity(taskId, sessionId);
    await expect(apiClient.setQueueAutoRun(identity, false)).resolves.toMatchObject({
      auto_run: false,
    });
    const editor = dialog.locator(".tiptap.ProseMirror:visible").first();
    const submit = dialog.getByTestId("submit-message-button");
    const marker = "quick chat startup marker";
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
    await editor.fill(`e2e:message("${marker}")`);
    await expect(submit).toBeEnabled({ timeout: 30_000 });
    await submit.click();

    await expect
      .poll(() => apiClient.getQueueStatus(identity).then((status) => status.count), {
        timeout: 30_000,
      })
      .toBe(1);
    await expect(apiClient.setQueueAutoRun(identity, true)).resolves.toMatchObject({
      auto_run: true,
    });
    await expect(
      dialog.locator("[data-agent-message-body][data-message-id]").filter({ hasText: marker }),
    ).toHaveCount(1, { timeout: 60_000 });
    await expect
      .poll(() => apiClient.getQueueStatus(identity).then((status) => status.count), {
        timeout: 30_000,
      })
      .toBe(0);
  });

  test("keeps the resumed desktop layout within its viewport", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const fixture = await seedDelayedResumeFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      "Resume prompt queue layout test",
    );

    try {
      await assertNoDocumentHorizontalOverflow(testPage, "desktop resume prompt queue");
    } finally {
      await cleanupDelayedResumeFixture(apiClient, fixture);
    }
  });
});
