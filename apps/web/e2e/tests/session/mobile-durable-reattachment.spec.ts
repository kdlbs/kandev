import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { triggerActualDeliveryDisconnect } from "../../helpers/durable-reattachment";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile durable delivery reattachment", () => {
  test.describe.configure({ retries: 1 });

  test("keeps actual disconnect recovery usable after reload on a phone", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const { traffic, sessionId } = await triggerActualDeliveryDisconnect(
      testPage,
      apiClient,
      seedData,
      "Mobile agentctl disconnect reattachment",
    );
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.reload();
    await session.waitForLoad();

    const layout = testPage.locator("[data-testid='mobile-task-layout']:visible");
    await expect(layout).toBeVisible({ timeout: 30_000 });
    const banner = testPage.getByTestId("failed-session-banner");
    await expect(banner).toBeVisible({ timeout: 30_000 });
    await expect(banner).toContainText(
      "Delivery was interrupted. The prompt outcome is uncertain.",
    );

    const stop = banner.getByTestId("recovery-stop-button");
    await expect(stop).toBeVisible();
    await expect(stop).toBeEnabled();
    const stopBox = await stop.boundingBox();
    expect(stopBox?.height ?? 0).toBeGreaterThanOrEqual(44);

    const promptFramesBeforeRetry = traffic.frames.filter(
      (frame) =>
        frame.direction === "sent" &&
        /prompt|message\.added|chat\.submit/i.test(frame.action ?? ""),
    ).length;
    await banner.getByTestId("recovery-retry-connection-button").tap();
    await expect(banner.getByTestId("session-recovery-error")).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(() =>
        traffic.frames.some(
          (frame) =>
            frame.direction === "sent" &&
            frame.action === "session.recover" &&
            frame.sessionId === sessionId,
        ),
      )
      .toBe(true);
    expect(
      traffic.frames.filter(
        (frame) =>
          frame.direction === "sent" &&
          /prompt|message\.added|chat\.submit/i.test(frame.action ?? ""),
      ),
    ).toHaveLength(promptFramesBeforeRetry);

    await stop.tap();
    await expect(banner.getByTestId("delivery-stop-outcome-unconfirmed")).toBeVisible({
      timeout: 10_000,
    });
    await expect
      .poll(() =>
        traffic.frames.some(
          (frame) =>
            frame.direction === "sent" &&
            frame.action === "session.stop" &&
            frame.sessionId === sessionId,
        ),
      )
      .toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile durable delivery reattachment");
  });
});
