import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { installRuntimeUpdateFixture, updateJob } from "./agent-runtime-update-helpers";

test.describe("OMP harness-owned updates", () => {
  test("previews the reference, approves an empty request, streams output, and reports the actual channel version", async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, { mode: "self_update" });
    await testPage.goto("/settings/agents");
    const trigger = testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`);
    await expect(trigger).toBeVisible();
    await expect(
      testPage.getByTestId(`agent-update-available-dot-${runtime.agentName}`),
    ).toBeVisible();
    await trigger.click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await expect(dialog).toContainText("Installed version: 18.3.1");
    await expect(
      dialog.getByTestId(`agent-update-stable-reference-${runtime.agentName}`),
    ).toContainText("18.3.2");
    await expect(dialog).toContainText("configured channel and may install a different version");
    await expect(dialog).toContainText("omp update");
    await expect(
      dialog.getByTestId(`agent-update-version-picker-${runtime.agentName}`),
    ).toHaveCount(0);
    await prCapture.screenshot("desktop-omp-update-preview", {
      caption: "OMP stable latest is reference-only",
    });

    const approvalResponse = waitForHttp(testPage, "POST", /\/api\/v1\/agent-update\/omp-acp$/);
    await dialog.getByTestId(`agent-update-confirm-${runtime.agentName}`).click();
    await approvalResponse;
    expect(runtime.postBodies()).toEqual([{}]);
    await runtime.emitOutput("Updated to 18.3.4 on canary channel\n");
    await expect(dialog.getByTestId(`agent-update-log-${runtime.agentName}`)).toContainText(
      "18.3.4",
    );
    runtime.setPersistedRuntimeVersion("18.3.4");
    await runtime.emitUpdate(
      updateJob({
        update_mode: "self_update",
        agent_name: runtime.agentName,
        current_version: "18.3.4",
        target_version: "",
        status: "succeeded",
        output: "Updated to 18.3.4 on canary channel\n",
        finished_at: "2026-09-26T12:01:00Z",
      }),
    );
    await runtime.emitCatalogue([{ id: "omp-next", name: "OMP Next" }]);
    await expect(dialog.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "Runtime updated successfully",
    );
    await expect(dialog.getByTestId(`agent-update-current-${runtime.agentName}`)).toContainText(
      "18.3.4",
    );
    await expect(
      dialog.getByTestId(`agent-update-stable-reference-${runtime.agentName}`),
    ).toContainText("18.3.2");
    await prCapture.screenshot("desktop-omp-update-result", {
      caption: "OMP post-update version differs from stable reference",
    });
  });

  test("shows updater failure without claiming a published version", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, { mode: "self_update" });
    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await dialog.getByTestId(`agent-update-confirm-${runtime.agentName}`).click();
    await runtime.emitUpdate(
      updateJob({
        update_mode: "self_update",
        agent_name: runtime.agentName,
        status: "failed",
        current_version: "18.3.1",
        target_version: "",
        error: "OMP updater declined this installation",
        finished_at: "2026-09-26T12:01:00Z",
      }),
    );
    await expect(dialog.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "OMP updater declined this installation",
    );
    await expect(dialog.getByTestId(`agent-update-current-${runtime.agentName}`)).toContainText(
      "18.3.1",
    );
    await expect(dialog.getByTestId(`agent-update-result-${runtime.agentName}`)).not.toContainText(
      "successfully",
    );
  });

  test("renders approval-time up-to-date locally before a failed status read, without job polling", async ({
    testPage,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, { mode: "self_update" });
    await testPage.goto("/settings/agents");
    const trigger = testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`);
    await expect(
      testPage.getByTestId(`agent-update-available-dot-${runtime.agentName}`),
    ).toBeVisible();
    await trigger.click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await expect(dialog.getByTestId(`agent-update-confirm-${runtime.agentName}`)).toBeEnabled();
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
    await dialog.getByTestId(`agent-update-confirm-${runtime.agentName}`).click();
    await approvalResponse;
    await expect(dialog.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "already up to date",
    );
    await expect.poll(() => runtime.statusRequestCount()).toBe(baselineStatusReads + 1);
    expect(runtime.jobsRequestCount()).toBe(baselineJobReads);
    expect(runtime.postBodies()).toEqual([{}]);
    statusGate.resolve();
    await failedStatusResponse;
    await expect(dialog.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "already up to date",
    );
    await expect(dialog.getByTestId(`agent-update-confirm-${runtime.agentName}`)).toHaveCount(0);
    expect(runtime.jobsRequestCount()).toBe(baselineJobReads);
  });
});
