import { test, expect } from "../../fixtures/test-base";
import path from "node:path";
import {
  createContinuationFixture,
  hasAttestedContinuationRefusal,
  waitForContinuationMessage,
  assertNativeContinuationTrace,
  assertNativeNoContinuationTrace,
  assertContinuationSettingRetired,
} from "../../helpers/provider-interruption-continuation";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";

test.setTimeout(300_000);

test("phone: mixed completed and pending foreground tools continue without opt-in", async ({
  testPage,
  backend,
  apiClient,
  seedData,
  prCapture,
}) => {
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "pending-v3");
  try {
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const completion = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.content?.includes("Mock continuation complete:") === true,
    );
    expect(completion.content).toContain("original=1 continuation=1");
    expect(completion.content).toContain("native=");
    assertNativeContinuationTrace(fixture.tracePath, "pending-v3");
    await expect(session.activeChat()).toContainText(
      "Mock interruption: partial history preserved.",
    );
    await expect(session.activeChat()).toContainText("Mock continuation complete:");
    await expect(session.activeChat().getByText("continue", { exact: true })).toHaveCount(0);
    const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
    expect(
      messages.filter(
        (message) => message.author_type === "user" && message.content === "continue",
      ),
    ).toHaveLength(0);
    await assertNoDocumentHorizontalOverflow(testPage);
    await assertContinuationSettingRetired(testPage, backend);
    if (prCapture.capturing) {
      await prCapture.screenshot("retired-continuation-toggle-phone", {
        caption: "Feature Toggles omits interruption continuation on a phone.",
        fullPage: true,
      });
    }
  } finally {
    await fixture.dispose();
  }
});

test("phone: continuation Cancel is visible while running and preserves history", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const gateway = watchWs(testPage);
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "pending-v3-hold");
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
    const screenshot = path.join(backend.tmpDir, "provider-interruption-continuation-phone.png");
    await testPage.screenshot({ path: screenshot });
    await test.info().attach("phone continuation running", {
      path: screenshot,
      contentType: "image/png",
    });
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
    assertNativeContinuationTrace(fixture.tracePath, "pending-v3-hold");
    const cancelResponse = gateway.waitForResponse("session.recover", {
      timeout: 35_000,
      requestWhere: (payload) =>
        payload.action === "cancel_retry" &&
        payload.task_id === fixture.taskId &&
        payload.session_id === fixture.sessionId,
    });
    await session.recoveryCancelRetryButton().tap();
    expect((await cancelResponse).payload.cancelled).toBe(true);
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
    assertNativeContinuationTrace(fixture.tracePath, "pending-v3-hold");
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});

test("phone: pending-tool continuation can be cancelled before dispatch", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const gateway = watchWs(testPage);
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "pending-v3", {
    prepareOnly: true,
  });
  try {
    await testPage.goto(`/t/${fixture.taskId}?sessionId=${fixture.sessionId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.sendMessageViaButton("/continuation-pending-v3");
    await expect(session.transientRetryCard()).toContainText("Continuing in", { timeout: 30_000 });
    const bounds = await session.recoveryCancelRetryButton().boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
    assertNativeNoContinuationTrace(fixture.tracePath, "pending-v3");
    const cancelResponse = gateway.waitForResponse("session.recover", {
      timeout: 35_000,
      requestWhere: (payload) =>
        payload.action === "cancel_retry" &&
        payload.task_id === fixture.taskId &&
        payload.session_id === fixture.sessionId,
    });
    await session.recoveryCancelRetryButton().tap();
    expect((await cancelResponse).payload.cancelled).toBe(true);
    const cancelled = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_disposition === "cancelled",
    );
    expect(cancelled.metadata?.runtime_retained).toBe(true);
    await expect(session.transientRetryCard()).toBeHidden();
    await expect(session.activeChat()).toContainText(
      "Mock interruption: partial history preserved.",
    );
    assertNativeNoContinuationTrace(fixture.tracePath, "pending-v3");
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});

test("phone: legacy pending tools show refusal details without recovery controls", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "pending");
  try {
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const failure = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.runtime_retained === true,
    );
    expect(hasAttestedContinuationRefusal(failure, "unsafe_work", 0)).toBe(true);
    await expect(session.activeChat()).toContainText(
      "Kandev cannot continue automatically because a tool is pending or its outcome is uncertain.",
    );
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await expect(session.transientRetryCard()).toBeHidden();
    assertNativeNoContinuationTrace(fixture.tracePath, "pending");
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await fixture.dispose();
  }
});
