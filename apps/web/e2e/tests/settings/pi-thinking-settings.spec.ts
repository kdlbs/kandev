import { test, expect } from "../../fixtures/test-base";
import { createPiProfile, clearPiThinking, assertPiStartup } from "./pi-thinking-helpers";

test("Pi saved thinking level can use provider default without changing the model", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(120_000);
  const profile = await createPiProfile(backend, apiClient);
  const { settings: baselineSettings } = await apiClient.getUserSettings();
  await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
  try {
    await test.step("Show thinking failure and open profile", () =>
      assertPiStartup(testPage, apiClient, seedData, profile.id, true));
    await clearPiThinking(testPage, profile.id, false);
    const stored = await apiClient.getAgentProfile(profile.id);
    expect(stored.mode ?? "").toBe("");
    expect(stored.configOptions?.thought_level).toBeUndefined();
    expect(stored.model).toBe("mock-fast");
    expect(stored.requireExactModel).toBe(true);
    await assertPiStartup(testPage, apiClient, seedData, profile.id, false);
  } finally {
    await backend.restart();
    await apiClient.saveUserSettings({
      prevent_auto_start_agent_on_open: Boolean(baselineSettings.prevent_auto_start_agent_on_open),
    });
    // Removing the mock Pi registration also removes its disposable profiles.
  }
});
