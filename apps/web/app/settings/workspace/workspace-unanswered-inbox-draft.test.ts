import { describe, expect, it } from "vitest";
import {
  buildUnansweredInboxUpdates,
  isValidRetentionDays,
  unansweredInboxSavedFrom,
} from "./workspace-unanswered-inbox-draft";

describe("workspace unanswered inbox draft", () => {
  it("defaults to off with 14 days when an older backend omits the fields", () => {
    expect(unansweredInboxSavedFrom({})).toEqual({ enabled: false, retentionDays: 14 });
    expect(
      unansweredInboxSavedFrom({
        clarification_unanswered_inbox_enabled: true,
        clarification_unanswered_retention_days: 30,
      }),
    ).toEqual({ enabled: true, retentionDays: 30 });
  });

  it("accepts whole days from 1 to 90 only", () => {
    for (const value of ["1", "14", "90"]) expect(isValidRetentionDays(value)).toBe(true);
    for (const value of ["0", "91", "", "1.5", "-3", "abc"]) {
      expect(isValidRetentionDays(value)).toBe(false);
    }
  });

  it("sends only changed fields so a partial update keeps the other stored value", () => {
    const saved = { enabled: false, retentionDays: 14 };
    expect(buildUnansweredInboxUpdates({ enabled: true, retentionDays: "14" }, saved)).toEqual({
      clarification_unanswered_inbox_enabled: true,
    });
    expect(buildUnansweredInboxUpdates({ enabled: false, retentionDays: "30" }, saved)).toEqual({
      clarification_unanswered_retention_days: 30,
    });
    expect(buildUnansweredInboxUpdates({ enabled: false, retentionDays: "0" }, saved)).toEqual({});
  });
});
