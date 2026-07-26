import { test, expect } from "../../fixtures/test-base";
import { installRuntimeUpdateFixture, updateJob } from "./agent-runtime-update-helpers";

test.describe("managed agent runtime updates on mobile", () => {
  test("keeps progress, bounded output, and retry touch-reachable without horizontal overflow", async ({
    testPage,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      retainedJobs: [
        updateJob({
          output: Array.from(
            { length: 60 },
            (_, index) => `Downloading runtime chunk ${index + 1} for the latest agent package`,
          ).join("\n"),
        }),
      ],
    });

    await testPage.goto("/settings/agents");
    const control = testPage.getByTestId(`agent-update-control-${runtime.agentName}`);
    const log = testPage.getByTestId(`agent-update-log-${runtime.agentName}`);
    const button = testPage.getByTestId(`agent-update-button-${runtime.agentName}`);

    await control.scrollIntoViewIfNeeded();
    await expect(control).toContainText("0.62.0 → 0.63.0");
    await expect(testPage.getByTestId(`agent-update-phase-${runtime.agentName}`)).toContainText(
      "Updating runtime",
    );
    await expect(log).toBeInViewport({ ratio: 1 });
    await expect(
      log.evaluate((element) => element.scrollHeight > element.clientHeight),
    ).resolves.toBe(true);
    await expect(button).toBeInViewport({ ratio: 1 });
    const runningButtonBox = await button.boundingBox();
    expect(runningButtonBox).not.toBeNull();
    expect(runningButtonBox!.height).toBeGreaterThanOrEqual(44);
    await expect(testPage.locator("html")).toHaveJSProperty(
      "scrollWidth",
      await testPage.locator("html").evaluate((element) => element.clientWidth),
    );

    await runtime.emitUpdate(
      updateJob({
        status: "failed",
        error: "Registry lookup failed",
        finished_at: "2026-07-26T12:01:00.000Z",
      }),
    );
    const result = testPage.getByTestId(`agent-update-result-${runtime.agentName}`);
    await result.scrollIntoViewIfNeeded();
    await expect(result).toContainText("Registry lookup failed");
    await expect(button).toHaveText("Retry update");
    await button.tap();
    expect(runtime.postCount()).toBe(1);
  });
});
