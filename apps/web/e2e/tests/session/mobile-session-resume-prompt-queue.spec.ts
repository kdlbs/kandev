// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  cleanupDelayedResumeFixture,
  seedDelayedResumeFixture,
  waitForSessionReady,
  waitForQueuedCount,
} from "../../helpers/session-resume-prompt-queue";

test.describe("mobile: Send during session resume", () => {
  test.describe.configure({ retries: 0 });

  test("taps Send during resume and delivers one queued prompt", async ({
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
      "Mobile resume prompt queue",
    );
    const marker = "mobile resume queue marker";

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

      const box = await submit.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      await expect(submit).toBeInViewport();
      await submit.tap();

      await expect(editor).toHaveText("");
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile resume prompt queue");

      await testPage.reload();
      await fixture.session.waitForLoad();
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await expect(fixture.session.activeChat().getByTestId("queue-chip")).toBeVisible();

      await waitForSessionReady(testPage, apiClient, fixture.task.id, fixture.identity.sessionId);
      await expect(apiClient.setQueueAutoRun(fixture.identity, true)).resolves.toMatchObject({
        auto_run: true,
      });
      await waitForQueuedCount(apiClient, fixture.identity, 0);
      const response = fixture.session
        .activeChat()
        .locator("[data-agent-message-body][data-message-id]")
        .filter({ hasText: marker });
      await expect(response).toHaveCount(1, { timeout: 60_000 });
      await assertNoDocumentHorizontalOverflow(testPage, "mobile resumed prompt response");
    } finally {
      await cleanupDelayedResumeFixture(apiClient, fixture);
    }
  });
});
