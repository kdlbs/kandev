import { describe, expect, it } from "vitest";
import { formatDateTime, formatTime } from "@/lib/i18n/formats";
import { formatQuotaDeadline, resolveWaitReason } from "./waiting-reason";

describe("resolveWaitReason", () => {
  it("gives a recorded quota wait precedence over admission waits", () => {
    expect(
      resolveWaitReason({
        quotaWait: {
          session_id: "session-1",
          reason: "quota_limited",
          deadline: "2026-11-09T12:30:00Z",
        },
        launchQueue: {
          queued_at: "2026-11-09T12:00:00Z",
          reason: "session_capacity",
          retrying: false,
        },
        wipAdmitted: false,
        queuedForStepId: "step-1",
      }),
    ).toBe("quota");
  });

  it("gives session capacity precedence over a WIP queue", () => {
    expect(
      resolveWaitReason({
        launchQueue: {
          queued_at: "2026-11-09T12:00:00Z",
          reason: "session_capacity",
          retrying: false,
        },
        wipAdmitted: false,
        queuedForStepId: "step-1",
      }),
    ).toBe("session_ceiling");
  });

  it("reports a WIP wait only while the task is not admitted", () => {
    expect(resolveWaitReason({ wipAdmitted: false, queuedForStepId: "step-1" })).toBe("wip_queue");
    expect(resolveWaitReason({ wipAdmitted: true, queuedForStepId: "step-1" })).toBeNull();
    expect(resolveWaitReason({ wipAdmitted: false })).toBeNull();
  });
});

describe("formatQuotaDeadline", () => {
  const now = new Date(2026, 10, 7, 9, 0, 0);

  it("rejects malformed and normalized deadlines", () => {
    expect(formatQuotaDeadline("0", now)).toBeNull();
    expect(formatQuotaDeadline("2026-02-30T10:00:00Z", now)).toBeNull();
  });

  it("shows only the time for a reset today and adds the day otherwise", () => {
    const today = new Date(2026, 10, 7, 18, 30, 0);
    const later = new Date(2026, 10, 9, 18, 30, 0);
    expect(formatQuotaDeadline(today.toISOString(), now)).toBe(formatTime(today));
    expect(formatQuotaDeadline(later.toISOString(), now)).toBe(formatDateTime(later));
  });
});
