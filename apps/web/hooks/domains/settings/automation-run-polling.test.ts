import { describe, expect, it } from "vitest";
import type { AutomationRun } from "@/lib/types/automation";
import { retryPollDelay } from "./automation-run-polling";

function retryRun(retryScheduledAt?: string): AutomationRun {
  return {
    id: "retry-run",
    automation_id: "automation-1",
    trigger_id: "trigger-1",
    trigger_type: "scheduled",
    status: "scheduled_retry",
    retry_scheduled_at: retryScheduledAt,
  } as AutomationRun;
}

describe("retryPollDelay", () => {
  it("polls just after the earliest due retry", () => {
    const now = Date.now();
    expect(retryPollDelay([retryRun(new Date(now + 10_000).toISOString())], now)).toBe(10_250);
  });

  it("bounds due and distant retries", () => {
    const now = Date.now();
    expect(retryPollDelay([retryRun(new Date(now - 5_000).toISOString())], now)).toBe(1_000);
    expect(retryPollDelay([retryRun(new Date(now + 120_000).toISOString())], now)).toBe(60_000);
  });

  it("uses the default cadence when no scheduled retry timestamp exists", () => {
    expect(retryPollDelay([retryRun()])).toBe(15_000);
  });
});
