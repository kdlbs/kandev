import { describe, expect, it } from "vitest";
import type { StorageQuarantineEntry } from "@/lib/types/system";
import { isQuarantineEligible, quarantineCounts } from "./storage-quarantine";

const entry = (id: string, deleteAfter: string, bytes: number): StorageQuarantineEntry => ({
  id,
  resource_type: "task_workspace",
  original_path: `/tmp/${id}`,
  quarantine_path: `/tmp/trash/${id}`,
  size_bytes: bytes,
  state: "quarantined",
  quarantined_at: "2026-07-29T00:00:00Z",
  delete_after: deleteAfter,
  last_error: "",
  metadata: {},
});

describe("storage quarantine eligibility", () => {
  const now = new Date("2026-07-29T12:00:00Z");

  it("treats the retention deadline as inclusive", () => {
    expect(isQuarantineEligible(entry("at", "2026-07-29T12:00:00Z", 1), now)).toBe(true);
    expect(isQuarantineEligible(entry("later", "2026-07-29T12:00:01Z", 1), now)).toBe(false);
  });

  it("aggregates eligible and protected counts and bytes", () => {
    expect(
      quarantineCounts(
        [entry("old", "2026-07-28T12:00:00Z", 10), entry("new", "2026-07-30T12:00:00Z", 20)],
        now,
      ),
    ).toEqual({ eligible: 1, eligibleBytes: 10, protected: 1, protectedBytes: 20 });
  });
});
