import { test, expect } from "../../fixtures/test-base";
import { installRuntimeUpdateFixture } from "./agent-runtime-update-helpers";

const AGENT_NAME = "copilot-acp";
const COPILOT_PACKAGE = "@github/copilot";

function copilotPreview(selectedProfileCount: number | null) {
  return {
    agent_name: AGENT_NAME,
    update_mode: "pinned" as const,
    package: COPILOT_PACKAGE,
    current_version: "1.0.104",
    default_version: "1.0.88",
    active_version: "1.0.104",
    effective_version: "1.0.104",
    target_version: "9.9.9",
    operation: "update" as const,
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
      selected_profile_count: selectedProfileCount,
    },
  };
}

function installCopilotRuntimeFixture(
  testPage: Parameters<typeof installRuntimeUpdateFixture>[0],
  selectedProfileCount: number | null,
) {
  return installRuntimeUpdateFixture(testPage, {
    agentName: AGENT_NAME,
    displayName: "Copilot",
    packageName: COPILOT_PACKAGE,
    currentVersion: "1.0.104",
    defaultVersion: "1.0.88",
    latestVersion: "9.9.9",
    previewResponse: copilotPreview(selectedProfileCount),
  });
}

test.describe("Copilot runtime plugin compatibility", () => {
  test("keeps preview compatibility unchecked before approval across target changes", async ({
    testPage,
  }) => {
    const runtime = await installCopilotRuntimeFixture(testPage, 2);
    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${AGENT_NAME}`).click();

    const dialog = testPage.getByTestId(`agent-update-dialog-${AGENT_NAME}`);
    const notice = dialog.getByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    await expect(notice).toHaveAttribute("data-state", "not_checked");
    await expect(notice).toContainText("Plugin compatibility for Copilot 9.9.9: unchecked.");
    await expect(notice).toContainText("2 saved profiles select plugins.");
    await expect(notice).not.toContainText(/qualified versions|will block/i);
    await expect(testPage.getByTestId(`agent-update-confirm-${AGENT_NAME}`)).toBeEnabled();
    expect(runtime.postCount()).toBe(0);

    await dialog.getByTestId(`agent-update-quick-default-${AGENT_NAME}`).click();
    await expect(notice).toHaveAttribute("data-state", "not_checked");
    await expect(notice).toContainText("Plugin compatibility for Copilot 1.0.88: unchecked.");

    await dialog.getByTestId(`agent-update-quick-latest-${AGENT_NAME}`).click();
    await expect(notice).toHaveAttribute("data-state", "not_checked");
    await expect(notice).toContainText("2 saved profiles select plugins.");
    await testPage.getByTestId(`agent-update-confirm-${AGENT_NAME}`).click();
    await expect.poll(() => runtime.postCount()).toBe(1);
    expect(runtime.postTargets()).toEqual(["9.9.9"]);
  });

  test("shows unknown impact without claiming zero affected profiles", async ({ testPage }) => {
    await installCopilotRuntimeFixture(testPage, null);
    await testPage.goto("/settings/agents");
    await testPage.getByTestId(`agent-update-trigger-${AGENT_NAME}`).click();

    const notice = testPage.getByTestId(`agent-update-copilot-compatibility-${AGENT_NAME}`);
    await expect(notice).toBeVisible();
    await expect(notice).toContainText("Saved profile selections could not be counted.");
    await expect(notice).not.toContainText("No saved profiles select plugins.");
  });
});
