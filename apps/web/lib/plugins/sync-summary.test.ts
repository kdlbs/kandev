import { describe, expect, it } from "vitest";
import { summarizeSyncResult } from "./sync-summary";
import type { SyncResult } from "@/lib/types/plugins";

function result(overrides: Partial<SyncResult> = {}): SyncResult {
  return { added: [], installed: [], missing: [], errors: [], ...overrides };
}

describe("summarizeSyncResult", () => {
  it("returns 'Everything up to date' when nothing changed", () => {
    expect(summarizeSyncResult(result())).toBe("Everything up to date");
  });

  it("summarizes a single category", () => {
    expect(summarizeSyncResult(result({ added: ["a"] }))).toBe("Sync: 1 sideloaded");
  });

  it("summarizes multiple categories in added/installed/missing order", () => {
    expect(
      summarizeSyncResult(result({ added: ["a", "b"], installed: ["c"], missing: ["d"] })),
    ).toBe("Sync: 2 sideloaded, 1 installed, 1 missing");
  });

  it("ignores errors alone when nothing else changed", () => {
    expect(summarizeSyncResult(result({ errors: [{ path: "/x.tar.gz", reason: "bad" }] }))).toBe(
      "Everything up to date",
    );
  });

  it("counts installed and missing without added", () => {
    expect(summarizeSyncResult(result({ installed: ["a"], missing: ["b", "c"] }))).toBe(
      "Sync: 1 installed, 2 missing",
    );
  });
});
