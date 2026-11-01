import { describe, expect, it } from "vitest";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import { recoveryCopy } from "./session-recovery-model";
import { storageRecoveryDetailFields } from "./session-storage-recovery";
import { storageCauseFromError } from "@/lib/session-delivery-storage";
import { formatSessionErrorDetails } from "@/lib/session-error-details";

const t = ((key: string, values?: Record<string, unknown>) =>
  values ? `${key} ${JSON.stringify(values)}` : key) as never;

function model(storage: Record<string, unknown>, kind = "generic"): ActiveSessionRecovery {
  return {
    sessionId: "session",
    kind,
    error: {
      message: "Session failed",
      occurredAt: "2026-10-27T10:00:00Z",
      causes: [{ operation: "delivery", code: "delivery_storage", storage }],
    },
  };
}

describe("storage recovery copy", () => {
  it("names the capacity cause and eligible local recovery", () => {
    const copy = recoveryCopy(
      model({
        reason: "disk_space_low",
        location: "local_runtime",
        disk_available_bytes: 1024,
        disk_required_bytes: 2048,
      }),
      t,
    );
    expect(copy.title).toBe("task:storageRecoveryDiskTitle");
    expect(copy.summary).toContain("task:storageRecoveryDiskBodyLocal");
    expect(copy.summary).toContain("task:storageRecoveryAtFailure");
    expect(copy.summary).toContain("task:storageRecoveryReported");
    expect(copy.showSummary).toBe(true);
  });

  it("identifies a remote executor instead of offering local cleanup", () => {
    const copy = recoveryCopy(
      model({
        reason: "filesystem_full",
        location: "executor_runtime",
        executor_label: "sprites",
      }),
      t,
    );
    expect(copy.summary).toContain("task:storageRecoveryFilesystemBodyRemote");
    expect(copy.summary).toContain("sprites");
    expect(copy.summary).not.toContain("task:storageRecoveryFilesystemBodyLocal");
  });

  it("distinguishes quota, full filesystem, and low byte capacity", () => {
    const quota = recoveryCopy(
      model({ reason: "disk_quota_exceeded", location: "local_runtime" }),
      t,
    );
    expect(quota.title).toBe("task:storageRecoveryQuotaTitle");
    expect(quota.summary).toContain("task:storageRecoveryQuotaBodyLocal");
    const filesystem = recoveryCopy(
      model({ reason: "filesystem_full", location: "local_runtime" }),
      t,
    );
    expect(filesystem.title).toBe("task:storageRecoveryFilesystemTitle");
    expect(filesystem.summary).toContain("task:storageRecoveryFilesystemBodyLocal");
  });

  it("copies validated storage fields into the shared details formatter", () => {
    const cause = storageCauseFromError(
      model({
        reason: "disk_quota_exceeded",
        location: "local_runtime",
        executor_label: "worktree",
        observed_at: "2026-10-27T10:00:00Z",
        disk_available_bytes: 1024,
        disk_required_bytes: 2048,
      }).error,
    );
    const detailT = ((key: string) =>
      ({
        "task:storageDetailCause": "Storage cause",
        "task:storageReasonQuota": "Filesystem quota exhausted",
        "task:storageDetailAvailable": "Available at failure",
        "task:storageDetailRequired": "Required at failure",
        "task:storageDetailObservedAt": "Storage observed",
        "task:storageDetailExecutor": "Executor",
      })[key] ?? key) as never;
    const text = formatSessionErrorDetails("", storageRecoveryDetailFields(cause, detailT));
    expect(text).toContain("Storage cause: Filesystem quota exhausted");
    expect(text).toContain("Available at failure: 1.0 KB");
    expect(text).toContain("Required at failure: 2.0 KB");
    expect(text).toContain("Executor: worktree");
    expect(text).not.toContain("undefined");
  });

  it("uses a distinct cause for logical limits and oversized output", () => {
    expect(
      recoveryCopy(model({ reason: "journal_limit_reached", location: "local_runtime" }), t).title,
    ).toBe("task:storageRecoveryLimitTitle");
    expect(
      recoveryCopy(model({ reason: "event_too_large", location: "local_runtime" }), t).title,
    ).toBe("task:storageRecoveryOversizedTitle");
    expect(
      recoveryCopy(model({ reason: "storage_write_failed", location: "unknown" }), t).title,
    ).toBe("task:storageRecoveryGenericTitle");
  });

  it("keeps the generic recovery fallback for untyped failures", () => {
    const copy = recoveryCopy(
      { sessionId: "session", kind: "generic", summary: "Agent stopped" },
      t,
    );
    expect(copy.title).toBe("task:sessionRecoveryFailed");
  });
});
