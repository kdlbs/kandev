import { describe, expect, it } from "vitest";
import { createDefaultUserSettings, mapUserSettingsData } from "@/lib/ssr/user-settings";
import type { UserSettingsData } from "@/lib/ssr/user-settings";

function settingValue(settings: object): unknown {
  return (settings as Record<string, unknown>).showTurnChangedFiles;
}

describe("turn changed-files user setting hydration", () => {
  it("defaults to disabled when the saved field is missing", () => {
    expect(settingValue(createDefaultUserSettings())).toBe(false);
    expect(settingValue(mapUserSettingsData({}))).toBe(false);
  });

  it("preserves an explicit enabled value and omitted partial updates", () => {
    const settings = mapUserSettingsData({ show_turn_changed_files: true } as UserSettingsData);
    expect(settingValue(settings)).toBe(true);
    expect(settingValue(mapUserSettingsData({}, settings))).toBe(true);
  });

  it("preserves an explicit disabled value", () => {
    const settings = JSON.parse('{"show_turn_changed_files":false}') as UserSettingsData;
    expect(settingValue(mapUserSettingsData(settings))).toBe(false);
  });
});
