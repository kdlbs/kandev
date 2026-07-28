import { test, expect } from "../../fixtures/test-base";
import { installRuntimeUpdateFixture, updateJob } from "./agent-runtime-update-helpers";

test.describe("managed agent runtime updates", () => {
  test("previews, approves, and streams an update without putting details on the card", async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);
    let documentNavigations = 0;
    testPage.on("framenavigated", (frame) => {
      if (frame === testPage.mainFrame()) documentNavigations += 1;
    });

    await testPage.goto("/settings/agents");
    const navigationsAfterLoad = documentNavigations;
    const trigger = testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`);
    await expect(trigger).toBeVisible();
    await expect(testPage.getByTestId(`agent-update-control-${runtime.agentName}`)).toHaveCount(0);
    const profileAction = testPage.getByRole("link", { name: "Setup Profile" });
    const [profileActionBox, triggerBox] = await Promise.all([
      profileAction.boundingBox(),
      trigger.boundingBox(),
    ]);
    expect(profileActionBox).not.toBeNull();
    expect(triggerBox).not.toBeNull();
    expect(Math.abs(profileActionBox!.y - triggerBox!.y)).toBeLessThanOrEqual(8);
    expect(triggerBox!.x).toBeGreaterThan(profileActionBox!.x);
    expect(profileActionBox!.height).toBe(28);
    expect(triggerBox!.height).toBe(profileActionBox!.height);

    await trigger.click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("0.62.0 → 0.63.0");
    await expect(dialog).toContainText(
      'npm exec --yes --prefer-online --package=@agentclientprotocol/claude-agent-acp -- node -e ""',
    );
    await expect(dialog).toContainText("Active sessions keep running");
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    await expect
      .poll(() => dialog.evaluate((element) => element.getBoundingClientRect().height))
      .toBeLessThan(viewport!.height * 0.6);
    expect(runtime.previewCount()).toBe(1);
    expect(runtime.postCount()).toBe(0);
    await prCapture.screenshot("desktop-update-preview", {
      caption: "Desktop update preview before approval",
    });

    await testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`).click();
    expect(runtime.postCount()).toBe(1);

    await runtime.emitUpdate(updateJob());
    await runtime.emitOutput("Installed @agentclientprotocol/claude-agent-acp@0.63.0\n");
    await expect(dialog.getByTestId(`agent-update-phase-${runtime.agentName}`)).toContainText(
      "Updating runtime",
    );
    await expect(dialog.getByTestId(`agent-update-log-${runtime.agentName}`)).toContainText(
      "Installed @agentclientprotocol/claude-agent-acp@0.63.0",
    );

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
    await expect(dialog.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "Runtime updated successfully",
    );
    expect(documentNavigations).toBe(navigationsAfterLoad);
    await testPage.reload();
    await expect(trigger).toBeVisible();
    await expect(testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`)).toHaveCount(0);
    await expect(testPage.getByTestId(`agent-update-result-${runtime.agentName}`)).toHaveCount(0);
  });

  test("requires a current runtime version before approval", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      previewResponse: {
        agent_name: "claude-acp",
        package: "@agentclientprotocol/claude-agent-acp",
        current_version: "",
        target_version: "0.63.0",
        command: ["npm", "exec"],
        command_string: "npm exec",
      },
    });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();

    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await expect(dialog).toContainText("Unknown → 0.63.0");
    await expect(testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`)).toBeDisabled();
    expect(runtime.postCount()).toBe(0);
  });

  test("retries an update after its job fails", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const retryUpdate = testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`);
    await retryUpdate.click();
    expect(runtime.postCount()).toBe(1);

    await runtime.emitUpdate(
      updateJob({
        status: "failed",
        error: "The package registry is unavailable",
        finished_at: "2026-07-26T12:01:00.000Z",
      }),
    );

    await expect(retryUpdate).toHaveText("Retry update");
    await expect(retryUpdate).toBeEnabled();
    await retryUpdate.click();
    expect(runtime.postCount()).toBe(2);
  });
});
