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
    // The update trigger leads the row; "Setup Profile" follows it.
    expect(triggerBox!.x).toBeLessThan(profileActionBox!.x);
    expect(profileActionBox!.height).toBe(28);
    expect(triggerBox!.height).toBe(profileActionBox!.height);

    await trigger.click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await expect(dialog).toBeVisible();
    const dialogBox = await dialog.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.width).toBeGreaterThanOrEqual(640);
    const selectorBox = await dialog
      .getByTestId(`agent-update-version-${runtime.agentName}`)
      .boundingBox();
    expect(selectorBox).not.toBeNull();
    expect(selectorBox!.height).toBeLessThanOrEqual(40);
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
    expect(runtime.postTargets()).toEqual(["0.63.0"]);

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
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();

    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await expect(
      dialog.getByTestId(`agent-update-version-summary-${runtime.agentName}`).getByText("0.64.0", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(dialog.getByRole("status").filter({ hasText: "Up to date" })).toBeVisible();
    await expect(dialog).not.toContainText("0.64.0 → 0.64.0");
    await expect(testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`)).toBeDisabled();
    expect(runtime.postCount()).toBe(0);
    await prCapture.screenshot("desktop-update-up-to-date", {
      caption: "Desktop update preview when the managed runtime is up to date",
    });
  });

  test("keeps repair distinct when observed and target versions match", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      previewResponse: {
        agent_name: "claude-acp",
        package: "@agentclientprotocol/claude-agent-acp",
        current_version: "0.64.0",
        target_version: "0.64.0",
        operation: "repair",
        command: ["npm", "exec"],
        command_string: "npm exec",
      },
    });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();

    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    const summary = dialog.getByTestId(`agent-update-version-summary-${runtime.agentName}`);
    await expect(summary).toContainText("Repair runtime");
    await expect(summary).toContainText("0.64.0 → 0.64.0");
    await expect(summary.getByText("Up to date", { exact: true })).toHaveCount(0);
    const confirm = testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`);
    await expect(confirm).toBeEnabled();
    await confirm.click();
    await runtime.emitUpdate(
      updateJob({
        status: "succeeded",
        operation: "repair",
        current_version: "0.64.0",
        target_version: "0.64.0",
        finished_at: "2026-07-26T12:01:00.000Z",
      }),
    );
    await expect(dialog.getByTestId(`agent-update-result-${runtime.agentName}`)).toContainText(
      "Runtime updated successfully",
    );
  });

  test("selects an older stable version for rollback", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await testPage.getByTestId(`agent-update-version-${runtime.agentName}`).selectOption("0.61.0");

    await expect(dialog).toContainText("0.62.0 → 0.61.0");
    await expect(testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`)).toHaveText(
      "Roll back runtime",
    );
    await testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`).click();
    expect(runtime.postTargets()).toEqual(["0.61.0"]);
    expect(runtime.previewTargets()).toEqual(["", "0.61.0"]);
  });

  test("keeps the dialog content visible while changing versions", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, { previewDelayMs: 500 });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    const body = dialog.getByTestId(`agent-update-dialog-body-${runtime.agentName}`);

    await dialog.getByTestId(`agent-update-version-${runtime.agentName}`).selectOption("0.61.0");

    await expect(
      dialog.getByTestId(`agent-update-version-loading-${runtime.agentName}`),
    ).toBeVisible();
    await expect(body).toContainText("Active sessions keep running");
    await expect(body).toContainText("Command that will run");
    await expect(dialog.getByTestId(`agent-update-version-${runtime.agentName}`)).toBeDisabled();
    await expect(
      dialog.getByTestId(`agent-update-version-loading-${runtime.agentName}`),
    ).toBeHidden();
  });

  test("limits the version selector to the ten newest options", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);
    runtime.setPreviewResponse({
      agent_name: "claude-acp",
      package: "@agentclientprotocol/claude-agent-acp",
      current_version: "0.62.0",
      target_version: "0.74.0",
      operation: "update",
      active_version: "0.62.0",
      available_versions: Array.from({ length: 12 }, (_, index) => ({
        version: `0.${74 - index}.0`,
        latest: index === 0,
      })),
      command: ["npm", "exec"],
      command_string: "npm exec",
    });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const options = testPage
      .getByTestId(`agent-update-version-${runtime.agentName}`)
      .locator("option");

    await expect(options).toHaveCount(10);
    expect(
      await options.evaluateAll((elements) =>
        elements.map((element) => (element as HTMLOptionElement).value),
      ),
    ).toEqual([
      "0.74.0",
      "0.73.0",
      "0.72.0",
      "0.71.0",
      "0.70.0",
      "0.69.0",
      "0.68.0",
      "0.67.0",
      "0.66.0",
      "0.65.0",
    ]);
  });

  test("retries a failed preview for the selected target", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      previewFailures: ["0.61.0"],
    });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await testPage.getByTestId(`agent-update-version-${runtime.agentName}`).selectOption("0.61.0");

    await expect(dialog.getByRole("alert")).toContainText("preview temporarily unavailable");
    await dialog.getByRole("button", { name: "Retry version check" }).click();
    await expect(dialog).toContainText("0.62.0 → 0.61.0");
    expect(runtime.previewTargets()).toEqual(["", "0.61.0", "0.61.0"]);
  });

  test("uses the job operation for a stale retry decision", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      postResponse: updateJob({
        status: "failed",
        operation: "up_to_date",
        active_version: "0.63.0",
        current_version: "0.63.0",
        target_version: "0.63.0",
        error: "The package registry is unavailable",
        finished_at: "2026-07-26T12:01:00.000Z",
      }),
    });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    const retryUpdate = testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`);
    await retryUpdate.click();

    await expect(
      dialog.getByTestId(`agent-update-version-summary-${runtime.agentName}`).getByText("0.63.0", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(dialog.getByRole("status").filter({ hasText: "Up to date" })).toBeVisible();
    await expect(retryUpdate).toHaveText("Retry update");
    await expect(retryUpdate).toBeDisabled();
    expect(runtime.postCount()).toBe(1);
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

  test("shows the newly activated version after a successful job", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`).click();
    await runtime.emitUpdate(
      updateJob({
        status: "succeeded",
        current_version: "0.63.0",
        active_version: "0.63.0",
        target_version: "0.63.0",
        finished_at: "2026-07-26T12:01:00.000Z",
      }),
    );

    await expect(dialog).toContainText("Active version: 0.63.0");
    const successOptionText = await dialog
      .getByTestId(`agent-update-version-${runtime.agentName}`)
      .locator("option")
      .allTextContents();
    expect(
      successOptionText.some((text) => text.includes("0.63.0") && text.includes("active")),
    ).toBe(true);
  });

  test("keeps the previous active version after a failed activation", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).click();
    const dialog = testPage.getByTestId(`agent-update-dialog-${runtime.agentName}`);
    await testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`).click();
    await runtime.emitUpdate(
      updateJob({
        status: "failed",
        active_version: "0.62.0",
        error: "candidate ACP probe failed",
        finished_at: "2026-07-26T12:01:00.000Z",
      }),
    );

    await expect(dialog).toContainText("Active version: 0.62.0");
    const failedOptionText = await dialog
      .getByTestId(`agent-update-version-${runtime.agentName}`)
      .locator("option")
      .allTextContents();
    expect(
      failedOptionText.some((text) => text.includes("0.62.0") && text.includes("active")),
    ).toBe(true);
    expect(
      failedOptionText.some((text) => text.includes("0.63.0") && text.includes("active")),
    ).toBe(false);
  });
});
