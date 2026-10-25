import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { waitForActiveSessionCancellationPending } from "../../helpers/session-store";

test.describe("Mobile cancel progress across reloads", () => {
  let releaseBackendEnv: (() => Promise<void>) | undefined;

  test.beforeEach(async ({ backend }) => {
    releaseBackendEnv = await backend.useEnv({
      KANDEV_E2E_PROMPT_CANCEL_JOIN_TIMEOUT: "12s",
    });
  });

  test.afterEach(async () => {
    await releaseBackendEnv?.();
    releaseBackendEnv = undefined;
  });

  test("keeps backend-owned cancel progress after a reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedIdleSession(testPage, apiClient, seedData, "Mobile cancel progress");
    // /e2e:cancel-hold keeps an acknowledged backend cancellation pending through reload hydration.
    await session.sendMessageViaButton("/e2e:cancel-hold");

    const cancel = session.activeChat().getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible({ timeout: 15_000 });
    await cancel.tap();
    await waitForActiveSessionCancellationPending(testPage, true);
    await expect(cancel).toBeDisabled();
    await expect(cancel.getByRole("status", { name: "Cancelling..." })).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();

    const reloadedCancel = session.activeChat().getByTestId("cancel-agent-button");
    await expect(reloadedCancel).toBeVisible({ timeout: 15_000 });
    await expect(reloadedCancel).toBeDisabled();
    await expect(reloadedCancel.getByRole("status", { name: "Cancelling..." })).toBeVisible();

    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await waitForActiveSessionCancellationPending(testPage, false);
    await expect(reloadedCancel).not.toBeVisible({ timeout: 15_000 });
  });
});
