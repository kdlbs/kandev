import { test, expect } from "../../fixtures/test-base";
import { settledBoundingBox } from "../../helpers/settled-box";
import { installRuntimeUpdateFixture } from "./agent-runtime-update-helpers";

const AGENT_NAME = "copilot-acp";

function installCopilotRuntimeFixture(testPage: Parameters<typeof installRuntimeUpdateFixture>[0]) {
  return installRuntimeUpdateFixture(testPage, {
    agentName: AGENT_NAME,
    displayName: "Copilot",
    packageName: "@github/copilot",
    currentVersion: "1.0.104",
    defaultVersion: "1.0.88",
    latestVersion: "9.9.9",
    previewResponse: {
      agent_name: AGENT_NAME,
      update_mode: "pinned",
      package: "@github/copilot",
      current_version: "1.0.104",
      default_version: "1.0.88",
      active_version: "1.0.104",
      effective_version: "1.0.104",
      target_version: "9.9.9",
      operation: "update",
      available_versions: [
        { version: "9.9.9", latest: true },
        { version: "1.0.104", latest: false },
        { version: "1.0.88", latest: false },
      ],
      command: ["npm", "exec"],
      command_string: "npm exec",
      copilot_plugin_compatibility: {
        runtime_version: "9.9.9",
        state: "not_checked",
        selected_profile_count: null,
      },
    },
  });
}

test.describe("Copilot runtime plugin compatibility on mobile", () => {
  test("keeps compatibility advisory inside the drawer without horizontal overflow", async ({
    testPage,
  }) => {
    const runtime = await installCopilotRuntimeFixture(testPage);
    await testPage.goto("/settings/agents");
    const trigger = testPage.getByTestId(`agent-update-trigger-${AGENT_NAME}`);
    await trigger.tap();

    const drawer = testPage.getByTestId(`agent-update-drawer-${AGENT_NAME}`);
    await expect(drawer).toBeVisible();
    const notice = drawer.getByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    await expect(notice).toHaveAttribute("data-state", "not_checked");
    await expect(notice).toContainText("Plugin compatibility for Copilot 9.9.9: unchecked.");
    await expect(notice).toContainText("Saved profile selections could not be counted.");
    await expect(notice).not.toContainText(/qualified versions|will block/i);
    const noticeBox = await settledBoundingBox(notice);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(noticeBox.x).toBeGreaterThanOrEqual(0);
    expect(noticeBox.x + noticeBox.width).toBeLessThanOrEqual(viewport!.width);
    await expect(testPage.locator("html")).toHaveJSProperty(
      "scrollWidth",
      await testPage.locator("html").evaluate((element) => element.clientWidth),
    );
    const confirm = testPage.getByTestId(`agent-update-confirm-${AGENT_NAME}`);
    const confirmBox = await confirm.boundingBox();
    expect(confirmBox).not.toBeNull();
    expect(confirmBox!.height).toBeGreaterThanOrEqual(44);
    await confirm.tap();
    await expect.poll(() => runtime.postCount()).toBe(1);
    expect(runtime.postTargets()).toEqual(["9.9.9"]);
  });
});
