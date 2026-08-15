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
    expect(runtime.postTargets()).toEqual(["0.63.0"]);

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

  test("shows an up-to-date preview and disables approval when versions match", async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      previewResponse: {
        agent_name: "claude-acp",
        package: "@agentclientprotocol/claude-agent-acp",
        current_version: "0.64.0",
        target_version: "0.64.0",
        operation: "up_to_date",
        command: ["npm", "exec"],
        command_string: "npm exec",
      },
    });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).tap();

    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    await expect(
      drawer.getByTestId(`agent-update-version-summary-${runtime.agentName}`).getByText("0.64.0", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(drawer.getByRole("status").filter({ hasText: "Up to date" })).toBeVisible();
    await expect(drawer).not.toContainText("0.64.0 → 0.64.0");
    await expect(testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`)).toBeDisabled();
    expect(runtime.postCount()).toBe(0);
    await prCapture.screenshot("mobile-update-up-to-date", {
      caption: "Mobile update preview when the managed runtime is up to date",
    });
  });

  test("selects an older stable version for rollback in the drawer", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).tap();
    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    const selector = drawer.getByTestId(`agent-update-version-${runtime.agentName}`);
    const selectorBox = await selector.boundingBox();
    expect(selectorBox).not.toBeNull();
    expect(selectorBox!.height).toBeGreaterThanOrEqual(44);
    await selector.selectOption("0.61.0");

    await expect(drawer).toContainText("0.62.0 → 0.61.0");
    await expect(testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`)).toHaveText(
      "Roll back runtime",
    );
    await testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`).tap();
    expect(runtime.postTargets()).toEqual(["0.61.0"]);
    expect(runtime.previewTargets()).toEqual(["", "0.61.0"]);
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
