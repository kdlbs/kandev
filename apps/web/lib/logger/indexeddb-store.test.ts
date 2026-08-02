import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";
import { retentionPlan } from "./indexeddb-store";

describe("IndexedDB log retention", () => {
  it("prunes age first and then oldest records until count and byte caps hold", () => {
    const now = Date.UTC(2026, 6, 30);
    const records = [
      {
        id: 1,
        identity_scope: "a",
        timestamp_ms: now - 4 * 86_400_000,
        bytes: 1,
        entry: {} as never,
      },
      ...Array.from({ length: 10_001 }, (_, index) => ({
        id: index + 2,
        identity_scope: "a",
        timestamp_ms: now - 1_000 + index,
        bytes: 10,
        entry: {} as never,
      })),
    ];
    const result = retentionPlan(records, now);
    expect(result.removeIDs).toEqual([1, 2]);
    expect(result.retainedCount).toBe(10_000);
  });

  it("walks the timestamp index without materializing the full store", () => {
    const source = readFileSync("lib/logger/indexeddb-store.ts", "utf8");

    expect(source).toContain('index("timestamp_ms")');
    expect(source).toContain("index.openCursor()");
    expect(source).not.toContain("store.getAll()");
  });
});
