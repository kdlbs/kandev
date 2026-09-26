import { expect, test } from "../../fixtures/test-base";
import { openNativeCodexProfile, stubNativeCodexModels } from "./codex-app-server-profile-helpers";

test.describe("Codex app-server profile on mobile", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_CODEX_APP_SERVER: "true",
      KANDEV_MOCK_AGENT: "true",
    });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("keeps the native profile editor usable on a phone", async ({
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
    const { profile } = await openNativeCodexProfile(testPage, apiClient);
    await modelsResponse;

    const title = testPage.getByRole("heading", { name: /Codex app server/i });
    await expect(title).toBeVisible({ timeout: 15_000 });
    const nameInput = testPage.getByTestId("profile-name-input");
    await expect(nameInput).toHaveValue(profile.name);
    await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
    await expect(
      testPage.getByRole("button", { name: "Profile start model settings" }),
    ).toContainText("GPT-5 Codex");
    await expect(testPage.getByTestId("profile-capability-status")).toHaveCount(0);
    const inputBox = await nameInput.boundingBox();
    expect(inputBox).not.toBeNull();
    expect(inputBox!.width).toBeGreaterThanOrEqual(240);

    const hasHorizontalOverflow = await testPage.evaluate(() => {
      const root = document.scrollingElement ?? document.documentElement;
      return root.scrollWidth > root.clientWidth + 1;
    });
    expect(hasHorizontalOverflow).toBe(false);
    await prCapture.screenshot("mobile-codex-app-server-profile", {
      caption: "Codex app-server profile settings on a phone",
    });
  });
});
