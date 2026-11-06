import { describe, expect, it } from "vitest";
import {
  buildWipReleaseUpdates,
  DEFAULT_WIP_RELEASE_GRACE_MINUTES,
  isValidGraceMinutes,
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
    for (const value of ["", "0", "-5", "1.5", "abc"]) {
      expect(isValidGraceMinutes(value)).toBe(false);
      expect(
        buildWipReleaseUpdates(
          { enabled: true, graceMinutes: value },
          { enabled: true, graceMinutes: 10 },
        ),
      ).toEqual({});
    }
  });
});
