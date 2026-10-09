import { describe, expect, it } from "vitest";
import type { ToolPayloadRetentionStatus } from "@/lib/types/tool-payload-retention";
import {
  observeBackupPreparation,
  type BackupPreparationCorrelation,
} from "./backup-preparation-correlation";

const INITIAL: BackupPreparationCorrelation = { activeRevision: null, settledRevision: -1 };

function status(
  revision: number,
  state: ToolPayloadRetentionStatus["preparation"]["state"],
  choice: ToolPayloadRetentionStatus["preparation"]["choice"] = "backup",
): ToolPayloadRetentionStatus {
  return {
    supported: true,
    policy: { enabled: true, age: { value: 4, unit: "months" }, revision },
    preparation: { state, choice },
  };
}

function apply(
  current: BackupPreparationCorrelation,
  next: ToolPayloadRetentionStatus,
  saveCandidateRevision?: number,
) {
  return observeBackupPreparation(current, next, saveCandidateRevision);
}

describe("backup preparation list-refresh correlation", () => {
  it("ignores an initial historical terminal state without a matching save or active observation", () => {
    expect(apply(INITIAL, status(5, "ready"))).toEqual({
      correlation: INITIAL,
      shouldInvalidate: false,
    });
    expect(apply(INITIAL, status(5, "failed"))).toEqual({
      correlation: INITIAL,
      shouldInvalidate: false,
    });
  });

  it("settles a terminal save response once and does not re-arm its revision", () => {
    const first = apply(INITIAL, status(1, "ready"), 1);
    expect(first.shouldInvalidate).toBe(true);
    expect(first.correlation).toEqual({ activeRevision: null, settledRevision: 1 });
    expect(apply(first.correlation, status(1, "ready"), 1)).toEqual({
      correlation: first.correlation,
      shouldInvalidate: false,
    });
    expect(apply(first.correlation, status(1, "pending"))).toEqual({
      correlation: first.correlation,
      shouldInvalidate: false,
    });
  });

  it("settles observed success or failure once across repeated terminal polls", () => {
    const pending = apply(INITIAL, status(1, "pending"));
    expect(pending).toEqual({
      correlation: { activeRevision: 1, settledRevision: -1 },
      shouldInvalidate: false,
    });
    const failed = apply(pending.correlation, status(1, "failed"));
    expect(failed).toEqual({
      correlation: { activeRevision: null, settledRevision: 1 },
      shouldInvalidate: true,
    });
    expect(apply(failed.correlation, status(1, "failed"))).toEqual({
      correlation: failed.correlation,
      shouldInvalidate: false,
    });
  });

  it("settles a published attempt when cancellation clears Preparation or a newer policy replaces it", () => {
    const pending = apply(INITIAL, status(1, "running"));
    expect(apply(pending.correlation, status(1, "none", ""))).toEqual({
      correlation: { activeRevision: null, settledRevision: 1 },
      shouldInvalidate: true,
    });
    expect(apply(pending.correlation, status(2, "none", ""))).toEqual({
      correlation: { activeRevision: null, settledRevision: 1 },
      shouldInvalidate: true,
    });
  });

  it("settles an older backup attempt before ignoring skip choice or unrelated cleanup", () => {
    const pending = apply(INITIAL, status(1, "running"));
    const skipped = apply(pending.correlation, status(2, "none", "skip"));
    expect(skipped.shouldInvalidate).toBe(true);
    expect(apply(INITIAL, status(1, "none", "skip"))).toEqual({
      correlation: INITIAL,
      shouldInvalidate: false,
    });
  });

  it("accepts a later distinct backup attempt only at a newer revision", () => {
    const settled = apply(INITIAL, status(1, "ready"), 1).correlation;
    expect(apply(settled, status(1, "pending"))).toEqual({
      correlation: settled,
      shouldInvalidate: false,
    });
    expect(apply(settled, status(2, "pending"))).toEqual({
      correlation: { activeRevision: 2, settledRevision: 1 },
      shouldInvalidate: false,
    });
    const next = apply(settled, status(2, "failed"), 2);
    expect(next).toEqual({
      correlation: { activeRevision: null, settledRevision: 2 },
      shouldInvalidate: true,
    });
  });

  it("does not let an older status clear a newer active attempt", () => {
    const active = { activeRevision: 3, settledRevision: 2 };
    expect(apply(active, status(2, "ready"))).toEqual({
      correlation: active,
      shouldInvalidate: false,
    });
  });
});
