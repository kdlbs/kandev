import { afterEach, describe, expect, it, vi } from "vitest";
import {
  formatStorageUsage,
  isRuntimeStoragePressureStale,
  isRuntimeStorageStale,
  normalizeAgentErrorStorage,
  mergeRuntimeStorageHealth,
  normalizeRuntimeStorageHealth,
  runtimeStorageUsagePercent,
  shouldAcceptRuntimeStorageHealth,
  storageCauseFromError,
  storageCauseKind,
  storageAdvisoryVisible,
} from "./session-delivery-storage";
import type { RuntimeStorageHealth } from "./types/http";
import { readLastAgentError } from "./session-last-agent-error";

const OBSERVED_AT = "2026-10-27T10:00:00Z";

const health: RuntimeStorageHealth = {
  state: "warning",
  location: "local_runtime",
  total_bytes: 1000,
  available_bytes: 100,
  observed_at: OBSERVED_AT,
  revision: 1,
  epoch: 5,
};

describe("session delivery storage", () => {
  it("accepts only closed storage reasons and safe measurements", () => {
    expect(
      normalizeAgentErrorStorage({
        reason: "disk_space_low",
        location: "local_runtime",
        disk_available_bytes: 1024,
        disk_required_bytes: 2048,
      }),
    ).toEqual({
      reason: "disk_space_low",
      location: "local_runtime",
      diskAvailableBytes: 1024,
      diskRequiredBytes: 2048,
    });
    expect(normalizeAgentErrorStorage({ reason: "provider_said_disk" })).toBeNull();
    expect(
      normalizeAgentErrorStorage({ reason: "disk_space_low", disk_available_bytes: -1 }),
    ).toEqual({
      reason: "disk_space_low",
      location: "unknown",
    });
    expect(normalizeAgentErrorStorage({ reason: "disk_space_low", limit_bytes: 0 })).toEqual({
      reason: "disk_space_low",
      location: "unknown",
    });
    for (const malformed of ["0", "2026-02-30T10:00:00Z", "2026-10-27 10:00:00Z"]) {
      expect(
        normalizeAgentErrorStorage({ reason: "disk_space_low", observed_at: malformed }),
      ).toEqual({ reason: "disk_space_low", location: "unknown" });
    }
  });

  it("reads a typed cause without inferring from text", () => {
    const error = {
      message: "Agent failed",
      causes: [
        {
          operation: "delivery",
          code: "delivery_storage",
          storage: {
            reason: "filesystem_full",
            location: "executor_runtime",
            executor_label: "sprites",
          },
        },
      ],
    };
    const cause = storageCauseFromError(error);
    expect(cause?.location).toBe("executor_runtime");
    expect(cause?.executorLabel).toBe("sprites");
    expect(storageCauseKind(cause?.reason)).toBe("capacity");
    expect(storageCauseFromError({ message: "no space left on device" })).toBeNull();
    const normalized = readLastAgentError({
      last_agent_error: {
        message: "Session interrupted",
        occurred_at: "2026-10-27T10:00:00Z",
        causes: error.causes,
      },
    });
    expect(storageCauseFromError(normalized)?.reason).toBe("filesystem_full");
    expect(storageCauseFromError(normalized)?.executorLabel).toBe("sprites");
  });

  it("computes usage and stale labels from validated values", () => {
    expect(runtimeStorageUsagePercent(health)).toBe(90);
    expect(runtimeStorageUsagePercent({ total_bytes: 1000, available_bytes: 2000 })).toBeNull();
    expect(formatStorageUsage(90)).toBe("90");
    expect(formatStorageUsage(89.94)).toBe("89.9");
    expect(isRuntimeStorageStale(0, 30_000)).toBe(false);
    expect(isRuntimeStorageStale(0, 90_001)).toBe(true);
    expect(isRuntimeStorageStale(60, 30_001)).toBe(true);
    expect(isRuntimeStorageStale(undefined, 0)).toBe(false);
    expect(isRuntimeStoragePressureStale(0, 0)).toBe(false);
    expect(isRuntimeStoragePressureStale(0, 90_001)).toBe(true);
    expect(isRuntimeStoragePressureStale(120, 0)).toBe(true);
    expect(isRuntimeStoragePressureStale(undefined, 0)).toBe(true);
    expect(isRuntimeStoragePressureStale(undefined, 30_000)).toBe(true);
    expect(storageAdvisoryVisible(health)).toBe(true);
    expect(storageAdvisoryVisible({ ...health, state: "normal" })).toBe(false);
    expect(storageAdvisoryVisible({ ...health, state: "normal", write_pressure: true })).toBe(true);
  });

  it("accepts newer revisions and rejects stale or foreign epochs", () => {
    expect(shouldAcceptRuntimeStorageHealth(null, health)).toBe(true);
    expect(shouldAcceptRuntimeStorageHealth(health, { ...health, revision: 2 })).toBe(true);
    expect(shouldAcceptRuntimeStorageHealth(health, { ...health, revision: 0 })).toBe(false);
    expect(shouldAcceptRuntimeStorageHealth(health, { ...health, epoch: 6, revision: 0 })).toBe(
      true,
    );
    expect(shouldAcceptRuntimeStorageHealth(health, { ...health, epoch: 4, revision: 9 })).toBe(
      false,
    );
    expect(shouldAcceptRuntimeStorageHealth(health, null)).toBe(false);
  });
});

describe("runtime storage freshness ages", () => {
  it("keeps capacity and pressure freshness independent", () => {
    const normalized = normalizeRuntimeStorageHealth({
      ...health,
      observed_at: OBSERVED_AT,
      age_seconds: 120,
      pressure_observed_at: "2026-10-27T10:02:00Z",
      pressure_age_seconds: 0,
      write_pressure: true,
    });
    expect(normalized?.age_seconds).toBe(120);
    expect(normalized?.pressure_age_seconds).toBe(0);
    expect(normalized?.pressure_observed_at).toBe("2026-10-27T10:02:00Z");
    const legacy = normalizeRuntimeStorageHealth({ ...health, age_seconds: 0 });
    expect(legacy?.pressure_age_seconds).toBeUndefined();
    expect(legacy?.pressure_observed_at).toBeUndefined();
    const zero = normalizeRuntimeStorageHealth({ ...health, pressure_age_seconds: 0 });
    expect(zero?.pressure_age_seconds).toBe(0);
    const future = normalizeRuntimeStorageHealth({
      ...health,
      pressure_observed_at: "2027-01-01T00:00:00Z",
    });
    expect(future?.pressure_observed_at).toBe("2027-01-01T00:00:00Z");
  });
});

describe("runtime storage cache receipt", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("preserves the browser receipt for the same snapshot and resets it for a newer age", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-27T10:00:00Z"));
    const first = mergeRuntimeStorageHealth(null, health);
    expect(first?.client_received_at_ms).toBe(Date.now());
    vi.advanceTimersByTime(60_000);
    const same = mergeRuntimeStorageHealth(first, health);
    expect(same?.client_received_at_ms).toBe(first?.client_received_at_ms);
    const changed = mergeRuntimeStorageHealth(same, {
      ...health,
      revision: 2,
      pressure_age_seconds: 60,
    });
    expect(changed?.client_received_at_ms).toBe(Date.now());
  });
});
