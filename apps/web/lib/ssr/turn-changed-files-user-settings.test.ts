import { describe, expect, it } from "vitest";
import { createDefaultUserSettings, mapUserSettingsData } from "@/lib/ssr/user-settings";
import type { UserSettingsData } from "@/lib/ssr/user-settings";

function settingValue(settings: object): unknown {
  return (settings as Record<string, unknown>).showTurnChangedFiles;
}

describe("turn changed-files user setting hydration", () => {
  it("defaults to enabled when the saved field is missing", () => {
    expect(settingValue(createDefaultUserSettings())).toBe(true);
    expect(settingValue(mapUserSettingsData({}))).toBe(true);
  });

  it("preserves an explicit disabled value", () => {
    const settings = JSON.parse('{"show_turn_changed_files":false}') as UserSettingsData;
    expect(settingValue(mapUserSettingsData(settings))).toBe(false);
  });
});
