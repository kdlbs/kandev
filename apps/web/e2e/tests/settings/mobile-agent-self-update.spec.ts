import { test, expect } from "../../fixtures/test-base";
import { settledBoundingBox } from "../../helpers/settled-box";
import { waitForHttp } from "../../helpers/causal-waits";
import { installRuntimeUpdateFixture, updateJob } from "./agent-runtime-update-helpers";

test.describe("OMP harness-owned updates on phones", () => {
  test("uses a bounded touch drawer with a reachable targetless update action", async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, { mode: "self_update" });
    await testPage.goto("/settings/agents");
    const trigger = testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`);
    await trigger.scrollIntoViewIfNeeded();
    await expect(trigger).toBeVisible();
    await trigger.tap();
    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    await expect(drawer).toContainText("Installed version: 18.3.1");
    await expect(
      drawer.getByTestId(`agent-update-stable-reference-${runtime.agentName}`),
    ).toContainText("18.3.2");
    await expect(drawer).toContainText("configured channel and may install a different version");
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
    expect(confirmBox.y + confirmBox.height).toBeLessThanOrEqual(viewport!.height);
    await expect(testPage.getByText("Kandev update available", { exact: true })).toBeHidden();
    await prCapture.screenshot("mobile-omp-update-preview", {
      caption: "OMP reference-only update drawer",
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
    const footerAction = drawer.getByRole("button", { name: "Cancel" });
    await expect(footerAction).toBeVisible();
    const footerBox = await settledBoundingBox(footerAction);
    expect(footerBox.y + footerBox.height).toBeLessThanOrEqual(viewport!.height);
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
