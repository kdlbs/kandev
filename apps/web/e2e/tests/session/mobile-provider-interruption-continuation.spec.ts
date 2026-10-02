import { test, expect } from "../../fixtures/test-base";
import {
  createContinuationFixture,
  waitForContinuationMessage,
  assertNativeContinuationTrace,
} from "../../helpers/provider-interruption-continuation";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(120_000);

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
    await expect(session.recoveryCancelRetryButton()).toHaveCount(1);
    const bounds = await session.recoveryCancelRetryButton().boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage);
    assertNativeContinuationTrace(fixture.tracePath, "read-hold");
    await session.recoveryCancelRetryButton().tap();
    await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_disposition === "cancelled",
    );
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
    for (const action of [session.recoveryResumeButton(), session.recoveryFreshButton()]) {
      const target = await action.boundingBox();
      expect(target?.height).toBeGreaterThanOrEqual(44);
    }
    await expect(session.activeChat()).toContainText("partial history preserved");
    const recovery = testPage.getByTestId("session-recovery-card");
    await expect(recovery).not.toContainText("exhausted");
    const details = recovery.getByText("Technical details", { exact: true });
    await details.click();
    await expect(recovery).toContainText("Automatic continuation");
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});
