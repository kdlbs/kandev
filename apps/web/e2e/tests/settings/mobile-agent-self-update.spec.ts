import { test, expect } from "../../fixtures/test-base";
import { settledBoundingBox } from "../../helpers/settled-box";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { installRuntimeUpdateFixture, updateJob } from "./agent-runtime-update-helpers";

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4
test.describe("OMP harness-owned updates on phones", () => {
  test("shows the native check result in a bounded touch drawer with a full-width update action", async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      mode: "self_update",
      currentVersion: "18.13.3",
      native: {
        checkState: "update_available",
        latestVersion: "18.22.0-canary.3",
        channel: "canary",
      },
    });
    await testPage.goto("/settings/agents");
    const trigger = testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`);
    await trigger.scrollIntoViewIfNeeded();
    await expect(trigger).toBeVisible();
    await trigger.tap();
    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    await expect(drawer).toContainText("Installed version: 18.13.3");
    await expect(
      drawer.getByTestId(`agent-update-native-check-${runtime.agentName}`),
    ).toContainText("18.22.0-canary.3");
    await expect(drawer.getByTestId(`agent-update-channel-${runtime.agentName}`)).toContainText(
      "canary",
    );
    await expect(
      drawer.getByTestId(`agent-update-stable-reference-${runtime.agentName}`),
    ).toHaveCount(0);
    await expect(drawer).toContainText("own update check for its configured channel");
    await expect(
      drawer.getByTestId(`agent-update-version-picker-${runtime.agentName}`),
    ).toHaveCount(0);
    const drawerBox = await settledBoundingBox(drawer);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(drawerBox.height).toBeLessThanOrEqual(viewport!.height);
    const confirm = drawer.getByTestId(`agent-update-confirm-${runtime.agentName}`);
    const confirmBox = await settledBoundingBox(confirm);
    expect(confirmBox.height).toBeGreaterThanOrEqual(44);
    expect(confirmBox.width).toBeGreaterThanOrEqual(drawerBox.width * 0.8);
    expect(confirmBox.y + confirmBox.height).toBeLessThanOrEqual(viewport!.height);
    await expect(testPage.locator("html")).toHaveJSProperty(
      "scrollWidth",
      await testPage.locator("html").evaluate((element) => element.clientWidth),
    );
    await expect(testPage.getByText("Kandev update available", { exact: true })).toBeHidden();
    await prCapture.screenshot("mobile-omp-update-preview", {
      caption: "OMP drawer shows its own canary update check result",
    });
    const approvalResponse = waitForHttp(testPage, "POST", /\/api\/v1\/agent-update\/omp-acp$/);
    await confirm.tap();
    await approvalResponse;
    expect(runtime.postBodies()).toEqual([{}]);

    await runtime.emitUpdate(
      updateJob({
        update_mode: "self_update",
        agent_name: runtime.agentName,
        current_version: "18.3.4",
        target_version: "",
        output: "Updated to 18.3.4\n".repeat(60),
      }),
    );
    const body = drawer.getByTestId(`agent-update-dialog-body-${runtime.agentName}`);
    await expect
      .poll(() => body.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    const close = drawer.getByRole("button", { name: "Close", exact: true });
    await expect(close).toBeVisible();
    await waitForFiniteAnimations(drawer);
    const closeBox = await close.boundingBox();
    expect(closeBox).not.toBeNull();
    expect(closeBox!.x).toBeGreaterThanOrEqual(0);
    expect(closeBox!.y).toBeGreaterThanOrEqual(0);
    expect(closeBox!.x + closeBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(closeBox!.y + closeBox!.height).toBeLessThanOrEqual(viewport!.height);
    expect((await settledBoundingBox(drawer)).height).toBeLessThanOrEqual(viewport!.height);
    await expect(testPage.locator("html")).toHaveJSProperty(
      "scrollWidth",
      await testPage.locator("html").evaluate((element) => element.clientWidth),
    );
  });

  test("shows a local no-job result through a failed status refresh without polling", async ({
    testPage,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, { mode: "self_update" });
    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).tap();
    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    const confirm = drawer.getByTestId(`agent-update-confirm-${runtime.agentName}`);
    await expect(confirm).toBeEnabled();
    const baselineStatusReads = runtime.statusRequestCount();
    const baselineJobReads = runtime.jobsRequestCount();
    const statusGate = Promise.withResolvers<void>();
    runtime.failNextStatusAfter(statusGate.promise);
    const failedStatusResponse = waitForHttp(testPage, "GET", /\/api\/v1\/agent-update\/status$/, {
      predicate: (response) => response.status() === 503,
    });
    runtime.setPostResponse(
      updateJob({
        update_mode: "self_update",
        agent_name: runtime.agentName,
        job_id: "",
        status: "succeeded",
        operation: "up_to_date",
        current_version: "18.3.2",
        target_version: "",
      }),
    );
    const approvalResponse = waitForHttp(testPage, "POST", /\/api\/v1\/agent-update\/omp-acp$/);
    await confirm.tap();
    await approvalResponse;
    await expect(drawer.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "already up to date",
    );
    await expect.poll(() => runtime.statusRequestCount()).toBe(baselineStatusReads + 1);
    expect(runtime.postBodies()).toEqual([{}]);
    expect(runtime.jobsRequestCount()).toBe(baselineJobReads);
    statusGate.resolve();
    await failedStatusResponse;
    await expect(drawer.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "already up to date",
    );
    await expect(confirm).toHaveCount(0);
  });
});
