import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  assertOneTurnStartPromptAndResponse,
  captureTurnStartMessageBaseline,
  cleanupResumeTurnStartFixture,
  seedResumeTurnStartFixture,
  waitForResumeSessionReady,
  waitForResumeTurnStartTransition,
} from "../../helpers/session-resume-turn-start";
import {
  countResumeBootMessages,
  waitForNewSessionMessage,
  waitForSessionStarting,
} from "../../helpers/session-resume-prompt-queue";

test.describe("Desktop message during resumed workflow turn start", () => {
  test.describe.configure({ retries: 0 });

  test("keeps startup intact and delivers the direct message once", async ({
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
      "Desktop resume turn-start race",
    );
    const marker = "desktop resume turn-start marker";

    try {
      const previousMessageIds = await captureTurnStartMessageBaseline(apiClient, fixture, marker);
      await apiClient.addUserMessage(
        fixture.task.id,
        fixture.identity.sessionId,
        `e2e:message("${marker}")`,
      );

      await waitForNewSessionMessage(
        apiClient,
        fixture.identity.sessionId,
        previousMessageIds,
        marker,
      );
      await waitForSessionStarting(
        testPage,
        apiClient,
        fixture.task.id,
        fixture.identity.sessionId,
        15_000,
      );
      await waitForResumeTurnStartTransition(apiClient, fixture, 1);

      const duringStartup = await apiClient.listTaskSessions(fixture.task.id);
      const startingSession = duringStartup.sessions.find(
        (session) => session.id === fixture.identity.sessionId,
      );
      expect(startingSession?.state).toBe("STARTING");
      expect(startingSession?.error_message ?? "").toBe("");

      await waitForResumeSessionReady(
        testPage,
        apiClient,
        fixture.task.id,
        fixture.identity.sessionId,
      );
      const response = fixture.session
        .activeChat()
        .locator("[data-agent-message-body][data-message-id]")
        .filter({ hasText: marker });
      await expect(response).toHaveCount(1, { timeout: 60_000 });
      await assertOneTurnStartPromptAndResponse(apiClient, fixture, marker);
      await waitForResumeTurnStartTransition(apiClient, fixture, 1);
      await expect
        .poll(() => countResumeBootMessages(apiClient, fixture.identity.sessionId))
        .toBe(1);
    } finally {
      await cleanupResumeTurnStartFixture(apiClient, fixture);
    }
  });
});
