import { test, expect } from "../../fixtures/test-base";
import {
  createContinuationFixture,
  waitForContinuationMessage,
  assertNativeContinuationTrace,
} from "../../helpers/provider-interruption-continuation";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(300_000);

test("phone: continuation Cancel is visible while running and preserves history", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "read-hold");
  try {
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.content?.includes("Mock continuation accepted:") === true,
    );
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.transientRetryCard()).toContainText("Continuing");
    await expect(session.activeChat()).toContainText(
      "Mock interruption: partial history preserved.",
    );
    await expect(session.activeChat().getByText("continue", { exact: true })).toHaveCount(0);
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toContainText("Continuing");
    await expect(session.activeChat()).toContainText(
      "Mock interruption: partial history preserved.",
    );
    await expect(session.activeChat().getByText("continue", { exact: true })).toHaveCount(0);
    const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
    expect(
      messages.filter(
        (message) => message.author_type === "user" && message.content === "continue",
      ),
    ).toHaveLength(0);
    await expect(session.recoveryCancelRetryButton()).toHaveCount(1);
    const bounds = await session.recoveryCancelRetryButton().boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage);
    assertNativeContinuationTrace(fixture.tracePath, "read-hold");
    await session.recoveryCancelRetryButton().tap();
    const cancelled = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_disposition === "cancelled",
    );
    expect(cancelled.metadata?.runtime_retained).toBe(true);
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await expect(session.transientRetryCard()).toBeHidden();
    await expect(session.activeChat()).toContainText("partial history preserved");
    await expect(testPage.getByTestId("session-recovery-card")).toHaveCount(0);
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
        return sessions.find((candidate) => candidate.id === fixture.sessionId)?.state;
      })
      .toBe("WAITING_FOR_INPUT");
    assertNativeContinuationTrace(fixture.tracePath, "read-hold");
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});
