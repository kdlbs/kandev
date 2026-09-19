import { describe, expect, it } from "vitest";
import { isUserSettingsResponseCurrent } from "./user-settings-revision";

describe("isUserSettingsResponseCurrent", () => {
  it("rejects a delayed response after a newer settings revision", () => {
    expect(isUserSettingsResponseCurrent(4, 5, false)).toBe(false);
  });

  it("accepts equal or newer revisions and only accepts unversioned responses for their submitted snapshot", () => {
    expect(isUserSettingsResponseCurrent(5, 5, false)).toBe(true);
    expect(isUserSettingsResponseCurrent(6, 5, false)).toBe(true);
    expect(isUserSettingsResponseCurrent(null, null, false)).toBe(false);
    expect(isUserSettingsResponseCurrent(null, null, true)).toBe(true);
  });
});
