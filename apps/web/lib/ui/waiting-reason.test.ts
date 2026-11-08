import { describe, expect, it } from "vitest";
import { formatDateTime, formatTime } from "@/lib/i18n/formats";
import { formatQuotaDeadline } from "./waiting-reason";

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
