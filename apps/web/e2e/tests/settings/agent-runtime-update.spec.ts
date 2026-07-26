import { test, expect } from "../../fixtures/test-base";
import { installRuntimeUpdateFixture, updateJob } from "./agent-runtime-update-helpers";

test.describe("managed agent runtime updates", () => {
  test("streams a successful update and renders refreshed models without a document reload", async ({
    testPage,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);
    let documentNavigations = 0;
    testPage.on("framenavigated", (frame) => {
      if (frame === testPage.mainFrame()) documentNavigations += 1;
    });

    await testPage.goto("/settings/agents");
    const navigationsAfterLoad = documentNavigations;
    const control = testPage.getByTestId(`agent-update-control-${runtime.agentName}`);
    await expect(control).toBeVisible();
    await expect(control).toContainText("Current version: 0.62.0");

    await testPage.getByTestId(`agent-update-button-${runtime.agentName}`).click();
    expect(runtime.postCount()).toBe(1);
    await expect(testPage.getByTestId(`agent-update-phase-${runtime.agentName}`)).toContainText(
      "Checking latest version",
    );

    await runtime.emitUpdate(updateJob());
    await runtime.emitOutput("Installed @agentclientprotocol/claude-agent-acp@0.63.0\n");
    await expect(testPage.getByTestId(`agent-update-phase-${runtime.agentName}`)).toContainText(
      "Updating runtime",
    );
    await expect(testPage.getByTestId(`agent-update-log-${runtime.agentName}`)).toContainText(
      "Installed @agentclientprotocol/claude-agent-acp@0.63.0",
    );
    await expect(control).toContainText("0.62.0 → 0.63.0");

    await runtime.emitUpdate(
      updateJob({
        status: "succeeded",
        output: "Installed @agentclientprotocol/claude-agent-acp@0.63.0\n",
        finished_at: "2026-07-26T12:01:00.000Z",
      }),
    );
    await runtime.emitCatalogue([
      { id: "claude-sonnet-4-6", name: "Claude Sonnet 4.6" },
      { id: "claude-opus-5", name: "Claude Opus 5" },
    ]);

    await expect(testPage.getByText("Claude refreshed", { exact: true })).toBeVisible();
    await expect(testPage.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "Runtime updated successfully",
    );
    expect(documentNavigations).toBe(navigationsAfterLoad);
    await testPage.getByRole("link", { name: "Setup Profile" }).click();
    await expect(testPage).toHaveURL(/\/settings\/agents\/claude-acp$/);
    await expect(testPage.getByRole("heading", { name: "Claude" })).toBeVisible();
    const modelPicker = testPage.getByRole("button", { name: "Profile start model settings" });
    await expect(modelPicker).toBeVisible();
    await modelPicker.click();
    await expect(testPage.getByText("Claude Opus 5", { exact: true })).toBeVisible();
  });

  test("rehydrates a retained failure and lets the operator retry it", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      retainedJobs: [
        updateJob({
          status: "failed",
          error: "Registry lookup failed",
          finished_at: "2026-07-26T12:01:00.000Z",
        }),
      ],
      postResponse: updateJob({
        job_id: "runtime-update-job-2",
        status: "resolving",
        target_version: undefined,
        started_at: "2026-07-26T12:02:00.000Z",
      }),
    });

    await testPage.goto("/settings/agents");
    await expect(testPage.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "Registry lookup failed",
    );
    const retry = testPage.getByTestId(`agent-update-button-${runtime.agentName}`);
    await expect(retry).toHaveText("Retry update");
    await retry.click();
    expect(runtime.postCount()).toBe(1);
    await expect(testPage.getByTestId(`agent-update-phase-${runtime.agentName}`)).toContainText(
      "Checking latest version",
    );
  });
});
