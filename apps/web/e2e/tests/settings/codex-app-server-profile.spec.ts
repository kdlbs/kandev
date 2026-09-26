import { expect, test } from "../../fixtures/test-base";
import { openNativeCodexProfile, stubNativeCodexModels } from "./codex-app-server-profile-helpers";

test.describe("Codex app-server profile on desktop", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_CODEX_APP_SERVER: "true",
      KANDEV_MOCK_AGENT: "true",
    });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("opens its separate native profile while the feature flag is enabled", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(60_000);
    await stubNativeCodexModels(testPage);
    const modelsResponse = testPage.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === "/api/v1/agent-models/codex-app-server";
    });
    const { agent, profile } = await openNativeCodexProfile(testPage, apiClient);
    await modelsResponse;

    await expect(testPage.getByRole("heading", { name: /Codex app server/i })).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByTestId("profile-name-input")).toHaveValue(profile.name);
    await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
    await expect(
      testPage.getByRole("button", { name: "Profile start model settings" }),
    ).toContainText("GPT-5 Codex");
    await expect(testPage.getByTestId("profile-capability-status")).toHaveCount(0);
    expect(agent.name).toBe("codex-app-server");
    expect(profile.agentId).toBe(agent.id);
    await prCapture.screenshot("codex-app-server-profile-desktop", {
      caption: "Separate Codex app-server profile settings",
    });
  });
});
