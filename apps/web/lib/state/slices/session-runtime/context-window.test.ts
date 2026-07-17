import { describe, expect, it } from "vitest";
import { parseContextWindowEntry } from "./context-window";

describe("parseContextWindowEntry", () => {
  it.each(["acp", "api"] as const)("retains the %s source", (source) => {
    expect(
      parseContextWindowEntry({
        size: 258_400,
        used: 95_100,
        remaining: 163_300,
        efficiency: 36.8,
        source,
      }),
    ).toEqual({
      size: 258_400,
      used: 95_100,
      remaining: 163_300,
      efficiency: 36.8,
      source,
      timestamp: undefined,
    });
  });

  it("drops an unknown source from older or malformed metadata", () => {
    expect(parseContextWindowEntry({ size: 128_000, source: "cache" })).toEqual(
      expect.objectContaining({ source: undefined }),
    );
  });

  it("prefers a caller-supplied timestamp over embedded metadata", () => {
    expect(
      parseContextWindowEntry(
        {
          size: 128_000,
          used: 64_000,
          remaining: 64_000,
          efficiency: 50,
          timestamp: "stored",
        },
        "live-2026-07-17T11:00:00.000Z",
      ),
    ).toEqual(expect.objectContaining({ timestamp: "live-2026-07-17T11:00:00.000Z" }));
  });

  it("rejects non-object metadata", () => {
    expect(parseContextWindowEntry(null)).toBeNull();
  });
});
