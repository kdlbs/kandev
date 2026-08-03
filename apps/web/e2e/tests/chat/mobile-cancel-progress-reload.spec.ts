import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { waitForActiveSessionCancellationPending } from "../../helpers/session-store";

test.describe("Mobile cancel progress across reloads", () => {
  test("keeps backend-owned cancel progress after a reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedIdleSession(testPage, apiClient, seedData, "Mobile cancel progress");
    await session.sendMessageViaButton("/slow 8s");

    const cancel = session.activeChat().getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible({ timeout: 15_000 });
    await cancel.tap();
    await waitForActiveSessionCancellationPending(testPage, true);
    await expect(cancel).toBeDisabled();
    await expect(cancel.getByRole("status", { name: "Loading" })).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();

    const reloadedCancel = session.activeChat().getByTestId("cancel-agent-button");
    await expect(reloadedCancel).toBeVisible({ timeout: 15_000 });
    await expect(reloadedCancel).toBeDisabled();
    await expect(reloadedCancel.getByRole("status", { name: "Loading" })).toBeVisible();

    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await waitForActiveSessionCancellationPending(testPage, false);
    await expect(reloadedCancel).not.toBeVisible({ timeout: 15_000 });
  });
});
