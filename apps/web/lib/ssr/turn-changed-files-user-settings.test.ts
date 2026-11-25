import { describe, expect, it } from "vitest";
import { createDefaultUserSettings, mapUserSettingsData } from "@/lib/ssr/user-settings";

describe("turn changed-files user setting hydration", () => {
  it("defaults to enabled when the saved field is missing", () => {
    expect(createDefaultUserSettings().showTurnChangedFiles).toBe(true);
    expect(mapUserSettingsData({}).showTurnChangedFiles).toBe(true);
  });

  it.each([true, false])("preserves a saved %s value across partial hydration", (value) => {
    const settings = mapUserSettingsData({ show_turn_changed_files: value });
    expect(settings.showTurnChangedFiles).toBe(value);
    expect(mapUserSettingsData({}, settings).showTurnChangedFiles).toBe(value);
  });
});
