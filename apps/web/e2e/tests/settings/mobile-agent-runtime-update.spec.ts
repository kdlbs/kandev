import { test, expect } from "../../fixtures/test-base";
import { installRuntimeUpdateFixture, updateJob } from "./agent-runtime-update-helpers";

test.describe("managed agent runtime updates on mobile", () => {
  test("uses a touch-safe drawer to preview and stream an update without horizontal overflow", async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);

    await testPage.goto("/settings/agents");
    const trigger = testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`);
    await trigger.scrollIntoViewIfNeeded();
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
    await trigger.tap();

    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("0.62.0 → 0.63.0");
    expect(runtime.postCount()).toBe(0);
    await prCapture.screenshot("mobile-update-preview", {
      caption: "Mobile update preview before approval",
    });
    await testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`).tap();
    expect(runtime.postCount()).toBe(1);

    await runtime.emitUpdate(
      updateJob({
        output: Array.from(
          { length: 60 },
          (_, index) => `Downloading runtime chunk ${index + 1} for the latest agent package`,
        ).join("\n"),
      }),
    );

    await expect(drawer.getByTestId(`agent-update-phase-${runtime.agentName}`)).toContainText(
      "Updating runtime",
    );
    const body = drawer.getByTestId(`agent-update-dialog-body-${runtime.agentName}`);
    await body.scrollIntoViewIfNeeded();
    await expect(
      body.evaluate((element) => element.scrollHeight > element.clientHeight),
    ).resolves.toBe(true);
    await expect(testPage.locator("html")).toHaveJSProperty(
      "scrollWidth",
      await testPage.locator("html").evaluate((element) => element.clientWidth),
    );
  });

  test("keeps retry available after an update job fails", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).tap();
    const retryUpdate = testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`);
    await retryUpdate.tap();
    expect(runtime.postCount()).toBe(1);

    await runtime.emitUpdate(
      updateJob({
        status: "failed",
        error: "The package registry is unavailable",
        finished_at: "2026-07-26T12:01:00.000Z",
      }),
    );

    await expect(retryUpdate).toHaveText("Retry update");
    await retryUpdate.tap();
    expect(runtime.postCount()).toBe(2);
  });
});
