import { test } from "../../fixtures/test-base";
import { createPiProfile, clearPiThinking, assertPiStartup } from "./pi-thinking-helpers";

test("phone Pi thinking selection uses one sheet and saves provider default", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}, testInfo) => {
  test.setTimeout(120_000);
  const profile = await createPiProfile(backend, apiClient);
  const { settings: baselineSettings } = await apiClient.getUserSettings();
  await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
  try {
    await test.step("Show thinking failure and open profile", () =>
      assertPiStartup(testPage, apiClient, seedData, profile.id, true));
    await test.step("Clear and save thinking on phone", () =>
      clearPiThinking(testPage, profile.id, true));
    await testPage.screenshot({
      path: testInfo.outputPath("pi-thinking-profile-phone.png"),
      fullPage: true,
    });
  } finally {
    await backend.restart();
    await apiClient.saveUserSettings({
      prevent_auto_start_agent_on_open: Boolean(baselineSettings.prevent_auto_start_agent_on_open),
    });
    // Removing the mock Pi registration also removes its disposable profiles.
  }
});
