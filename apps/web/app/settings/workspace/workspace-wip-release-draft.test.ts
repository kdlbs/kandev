import { describe, expect, it } from "vitest";
import {
  buildWipReleaseUpdates,
  DEFAULT_WIP_RELEASE_GRACE_MINUTES,
  isValidGraceMinutes,
  MAX_WIP_RELEASE_GRACE_MINUTES,
  settleDraftField,
  wipReleaseSavedFrom,
} from "./workspace-wip-release-draft";

describe("workspace WIP release draft", () => {
  it("defaults to off with the default grace when an older backend omits the fields", () => {
    expect(wipReleaseSavedFrom({})).toEqual({
      enabled: false,
      graceMinutes: DEFAULT_WIP_RELEASE_GRACE_MINUTES,
    });
  });

  it("sends only the changed fields", () => {
    const saved = { enabled: false, graceMinutes: 10 };
    expect(buildWipReleaseUpdates({ enabled: false, graceMinutes: "10" }, saved)).toEqual({});
    expect(buildWipReleaseUpdates({ enabled: true, graceMinutes: "10" }, saved)).toEqual({
      wip_sessionless_release_enabled: true,
    });
    expect(buildWipReleaseUpdates({ enabled: false, graceMinutes: "30" }, saved)).toEqual({
      wip_sessionless_release_grace_minutes: 30,
    });
  });

  it("never sends an invalid grace", () => {
    const overMax = String(MAX_WIP_RELEASE_GRACE_MINUTES + 1);
    for (const value of ["", "0", "-5", "1.5", "abc", overMax, "200000000"]) {
      expect(isValidGraceMinutes(value)).toBe(false);
      expect(
        buildWipReleaseUpdates(
          { enabled: true, graceMinutes: value },
          { enabled: true, graceMinutes: 10 },
        ),
      ).toEqual({});
    }
  });

  it("accepts the maximum grace", () => {
    expect(isValidGraceMinutes(String(MAX_WIP_RELEASE_GRACE_MINUTES))).toBe(true);
  });

  it("settles unedited fields to the saved values and keeps in-flight edits", () => {
    // Saved from another tab: release turned on while this page saved only grace.
    expect(settleDraftField(false, false, true)).toBe(true);
    expect(settleDraftField("30", "30", "30")).toBe("30");
    // Edited while the save was in flight: the newer edit stays.
    expect(settleDraftField("45", "30", "30")).toBe("45");
  });
});
