import { expect, test } from "../../fixtures/test-base";
import {
  assertOpenCodeCapacityTrace,
  createOpenCodeCapacityFixture,
  driveOpenCodeCapacityRetries,
  expectOpenCodeCapacityNotice,
  expectOpenCodeContinuation,
} from "../../helpers/opencode-capacity-retry";
import { advanceCapacityRetries, waitForCapacityRetry } from "../../helpers/capacity-retry-clock";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(480_000);

test("phone continues after OpenCode admission refusals with a reachable Cancel action", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createOpenCodeCapacityFixture(
    backend,
    apiClient,
    seedData,
    "completed-tools",
    { initialPrompt: "/opencode-capacity-completed-tools:6" },
  );
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expectOpenCodeCapacityNotice(session);
    const cancel = session.activeChat().getByTestId("recovery-cancel-retry-button");
    await expect(cancel).toBeVisible();
    const bounds = await cancel.boundingBox();
    expect(bounds!.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "OpenCode capacity notice");

    await driveOpenCodeCapacityRetries(backend, apiClient, fixture.sessionId, 1, 5);
    await waitForCapacityRetry(apiClient, fixture.sessionId, 6);
    await expect(session.transientRetryCard()).toContainText("Attempt 6 of 11");
    await driveOpenCodeCapacityRetries(backend, apiClient, fixture.sessionId, 6, 6);

    await expectOpenCodeContinuation(session);
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "OpenCode capacity continuation");
    assertOpenCodeCapacityTrace(fixture.tracePath, 7);
  } finally {
    await fixture.dispose();
  }
});

test("phone cancels OpenCode capacity recovery and accepts later input", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createOpenCodeCapacityFixture(
    backend,
    apiClient,
    seedData,
    "completed-tools",
    { initialPrompt: "/opencode-capacity-completed-tools:100" },
  );
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expectOpenCodeCapacityNotice(session);
    await driveOpenCodeCapacityRetries(backend, apiClient, fixture.sessionId, 1, 5);
    await waitForCapacityRetry(apiClient, fixture.sessionId, 6);
    const cancel = session.activeChat().getByTestId("recovery-cancel-retry-button");
    await cancel.tap();
    await expect(session.transientRetryCard()).toHaveCount(0);

    await advanceCapacityRetries(backend, 1800);
    await session.sendMessageViaButton("/e2e:simple-message");
    await expect(
      session
        .activeChat()
        .getByText("This is a simple mock response for e2e testing.", { exact: true }),
    ).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "OpenCode capacity after cancel");
    assertOpenCodeCapacityTrace(fixture.tracePath, 7);
  } finally {
    await fixture.dispose();
  }
});
