import { describe, expect, it } from "vitest";
import {
  formatStorageUsage,
  isRuntimeStorageStale,
  normalizeAgentErrorStorage,
  runtimeStorageUsagePercent,
  shouldAcceptRuntimeStorageHealth,
  storageCauseFromError,
  storageCauseKind,
  storageAdvisoryVisible,
} from "./session-delivery-storage";
import type { RuntimeStorageHealth } from "./types/http";
import { readLastAgentError } from "./session-last-agent-error";

const health: RuntimeStorageHealth = {
  state: "warning",
  location: "local_runtime",
  total_bytes: 1000,
  available_bytes: 100,
  observed_at: "2026-10-27T10:00:00Z",
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
