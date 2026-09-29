import { test, expect } from "../../fixtures/test-base";
import { settledBoundingBox } from "../../helpers/settled-box";
import { installRuntimeUpdateFixture, updateJob } from "./agent-runtime-update-helpers";

test.describe("managed agent runtime updates on mobile", () => {
  test("uses a touch-safe drawer to preview and stream an update without horizontal overflow", async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installRuntimeUpdateFixture(testPage);

    await testPage.goto("/settings/agents");
    const trigger = testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`);
    await expect(
      testPage.getByTestId(`agent-update-available-dot-${runtime.agentName}`),
    ).toBeVisible();
    await trigger.scrollIntoViewIfNeeded();
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
    await trigger.tap();

    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("0.62.0 → 0.63.0");
    const body = drawer.getByTestId(`agent-update-dialog-body-${runtime.agentName}`);
    await expect
      .poll(() => body.evaluate((element) => element.scrollHeight <= element.clientHeight))
      .toBe(true);
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
    const drawerBefore = await settledBoundingBox(drawer);
    await selector.tap();
    const browser = testPage.getByTestId(`agent-update-version-browser-${runtime.agentName}`);
    await expect(browser).toBeVisible();
    await expect(browser).toHaveAttribute("data-slot", "popover-content");
    const browserBox = await settledBoundingBox(browser);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(browserBox.x).toBeGreaterThanOrEqual(0);
    expect(browserBox.y).toBeGreaterThanOrEqual(0);
    expect(browserBox.x + browserBox.width).toBeLessThanOrEqual(viewport!.width);
    expect(browserBox.y + browserBox.height).toBeLessThanOrEqual(viewport!.height);
    const drawerAfter = await settledBoundingBox(drawer);
    expect(Math.abs(drawerAfter.height - drawerBefore.height)).toBeLessThan(2);
    expect(
      await browser.evaluate(
        (element) =>
          element.closest('[data-slot="dialog-content"], [data-slot="drawer-content"]') === null,
      ),
    ).toBe(true);
    const option = browser.getByTestId(`agent-update-version-option-${runtime.agentName}-0.61.0`);
    const optionBox = await option.boundingBox();
    expect(optionBox).not.toBeNull();
    expect(optionBox!.height).toBeGreaterThanOrEqual(44);
    await option.tap();

    await expect(drawer).toContainText("0.62.0 → 0.61.0");
    await expect(testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`)).toHaveText(
      "Roll back runtime",
    );
    await testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`).tap();
    expect(runtime.postTargets()).toEqual(["0.61.0"]);
    expect(runtime.previewTargets()).toEqual(["", "0.61.0"]);
  });

  test("searches the full version history in the drawer", async ({ testPage }) => {
    const runtime = await installRuntimeUpdateFixture(testPage, {
      previewResponse: {
        agent_name: "claude-acp",
        package: "@agentclientprotocol/claude-agent-acp",
        current_version: "0.62.0",
        default_version: "0.64.0",
        active_version: "0.62.0",
        effective_version: "0.62.0",
        target_version: "0.63.0",
        available_versions: Array.from({ length: 12 }, (_, index) => ({
          version: `0.${74 - index}.0`,
          latest: index === 0,
        })),
        command: ["npm", "exec"],
        command_string: "npm exec",
      },
    });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).tap();
    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    await drawer.getByTestId(`agent-update-version-${runtime.agentName}`).tap();
    const browser = testPage.getByTestId(`agent-update-version-browser-${runtime.agentName}`);
    await expect(browser).toBeVisible();
    const search = browser.getByPlaceholder("Search versions");
    await expect(search).toBeVisible();
    await search.fill("0.70");
    const option = browser.getByTestId(`agent-update-version-option-${runtime.agentName}-0.70.0`);
    await expect(option).toBeVisible();
    // The popover entrance animation scales the option row briefly. Poll until
    // the touch target reaches its settled 44px height before tapping it.
    await expect
      .poll(async () => Math.round((await option.boundingBox())?.height ?? 0))
      .toBeGreaterThanOrEqual(44);
    await option.tap();
    await expect(drawer).toContainText("0.62.0 → 0.70.0");
    expect(runtime.previewTargets()).toEqual(["", "0.70.0"]);
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

  test("presents OpenCode migration choices as touch-safe controls in the drawer", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 600 });
    const runtime = await installRuntimeUpdateFixture(testPage, {
      agentName: "opencode-acp",
      displayName: "OpenCode",
      packageName: "opencode-ai",
      currentVersion: "1.18.32",
      defaultVersion: "1.18.32",
      latestVersion: "1.18.32",
      previewResponse: {
        agent_name: "opencode-acp",
        package: "opencode-ai",
        current_version: "1.18.32",
        default_version: "1.18.32",
        active_version: "1.18.32",
        effective_version: "1.18.32",
        target_version: "1.18.32",
        family: "v1",
        source: "managed",
        target_family: "v1",
        runtime_revision: 7,
        migration_available: true,
        operation: "up_to_date",
        command: ["npm", "exec"],
        command_string: "npm exec --package=opencode-ai@1.18.32 -- opencode acp",
      },
      migrationPreviewResponse: {
        agent_name: "opencode-acp",
        package: "@opencode/cli",
        current_version: "1.18.32",
        default_version: "2.0.18",
        active_version: "1.18.32",
        effective_version: "1.18.32",
        target_version: "2.0.18",
        family: "v1",
        source: "managed",
        target_family: "v2",
        runtime_revision: 7,
        migration_available: true,
        operation: "migrate",
        command: ["npm", "exec", "--package=@opencode/cli@2.0.18"],
        command_string:
          "npm exec --yes --prefer-online --package=@opencode/cli@2.0.18 -- opencode acp --print-logs --log-level ERROR",
      },
    });

    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${runtime.agentName}`).tap();
    const drawer = testPage.getByTestId(`agent-update-drawer-${runtime.agentName}`);
    await expect(drawer).toBeVisible();
    const migrationChoice = drawer.getByTestId(`agent-update-migrate-family-${runtime.agentName}`);
    const choiceBox = await migrationChoice.boundingBox();
    expect(choiceBox).not.toBeNull();
    expect(choiceBox!.height).toBeGreaterThanOrEqual(44);
    expect(choiceBox!.width).toBeGreaterThanOrEqual(44);
    expect(runtime.postCount()).toBe(0);
    await migrationChoice.tap();
    await expect(drawer).toContainText("1.18.32 → 2.0.18");
    const info = drawer.getByTestId(`agent-update-info-${runtime.agentName}`);
    const infoBox = await info.boundingBox();
    expect(infoBox!.height).toBeGreaterThanOrEqual(44);
    expect(infoBox!.width).toBeGreaterThanOrEqual(44);
    await info.tap();
    const information = testPage.getByRole("dialog", { name: "Upgrade to managed OpenCode v2" });
    await expect(information).toBeVisible();
    await expect(information).toContainText("The standalone CLI remains unchanged.");
    await information.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(information).toBeHidden();
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("Stop standalone OpenCode v1 processes");
    const body = drawer.getByTestId(`agent-update-dialog-body-${runtime.agentName}`);
    await expect(body).toHaveCSS("overflow-y", "auto");
    const commandDetails = drawer.getByTestId(`agent-update-command-${runtime.agentName}`);
    await expect(commandDetails).not.toHaveAttribute("open");
    if (prCapture.capturing) {
      await drawer.evaluate((element) =>
        Promise.all(
          element
            .getAnimations({ subtree: true })
            .filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity)
            .map((animation) => animation.finished),
        ),
      );
      await prCapture.screenshot("mobile-opencode-migration-preview", {
        caption: "Mobile OpenCode v1-to-v2 migration preview with details available on demand",
      });
    }
    await commandDetails.locator("summary").tap();
    await expect(commandDetails).toHaveAttribute("open", "");
    const command = body.locator("pre");
    await command.scrollIntoViewIfNeeded();
    await expect(command).toBeInViewport();
    const confirm = testPage.getByTestId(`agent-update-confirm-${runtime.agentName}`);
    await expect(confirm).toBeVisible();
    const confirmBox = await confirm.boundingBox();
    expect(confirmBox).not.toBeNull();
    expect(confirmBox!.height).toBeGreaterThanOrEqual(44);
    expect(runtime.previewFamilies()).toEqual(["", "v2"]);
    await confirm.tap();
    expect(runtime.postBodies()).toEqual([
      {
        target_version: "2.0.18",
        target_family: "v2",
        expected_runtime_revision: 7,
      },
    ]);
    await expect(testPage.locator("html")).toHaveJSProperty(
      "scrollWidth",
      await testPage.locator("html").evaluate((element) => element.clientWidth),
    );
  });
});
