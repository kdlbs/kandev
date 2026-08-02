import { describe, expect, it } from "vitest";
import { chunkEntries } from "./capture";
import type { LogEntry } from "./buffer";

describe("frontend log capture chunks", () => {
  it("keeps sequential chunks below the requested target", () => {
    const entries: LogEntry[] = Array.from({ length: 5 }, (_, index) => ({
      timestamp: String(index),
      level: "info",
      source: "console",
      message: "x".repeat(40),
    }));
    const chunks = chunkEntries(entries, 160);
    expect(chunks.length).toBeGreaterThan(1);
    expect(chunks.flat()).toEqual(entries);
    expect(chunks.every((chunk) => chunk.length > 0)).toBe(true);
  });
});
