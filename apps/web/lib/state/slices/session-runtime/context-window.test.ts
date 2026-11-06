import { describe, expect, it } from "vitest";
import {
  clearContextWindowGauge,
  mergeCompactionCountValue,
  mergeCompactionReportingValue,
  mergeContextWindowEntry,
  parseContextWindowEntry,
  patchContextWindowCompaction,
  reconcileContextWindowMetadata,
} from "./context-window";

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
      compactionCount: 0,
      source,
      timestamp: undefined,
    });
  });

  it("drops an unknown source from older or malformed metadata", () => {
    expect(parseContextWindowEntry({ size: 128_000, source: "cache" })).toEqual(
      expect.objectContaining({ source: undefined }),
    );
  });

  it("parses a caller-supplied persisted inferred compaction count", () => {
    expect(
      parseContextWindowEntry(
        {
          size: 128_000,
          used: 64_000,
          remaining: 64_000,
          efficiency: 50,
        },
        undefined,
        3,
      ),
    ).toEqual(expect.objectContaining({ compactionCount: 3 }));
  });

  it.each([
    { label: "missing", value: undefined },
    { label: "invalid", value: "3" },
    { label: "negative", value: -2 },
  ])("normalizes a $label compaction count to zero", ({ value }) => {
    expect(
      parseContextWindowEntry(
        { size: 128_000, used: 64_000, compaction_count: value },
        undefined,
        value,
      ),
    ).toEqual(expect.objectContaining({ compactionCount: 0 }));
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

describe("compaction reporting and monotonic lifetime counts", () => {
  const observed = {
    source: "copilot_native" as const,
    availability: "available" as const,
    coverage: "observed" as const,
    revision: 2,
  };

  it("parses provider reporting metadata", () => {
    expect(
      parseContextWindowEntry(
        { size: 272_000, used: 12_171, remaining: 259_829, efficiency: 4.5 },
        undefined,
        4,
        observed,
      ),
    ).toEqual(expect.objectContaining({ compactionCount: 4, compactionReporting: observed }));
  });

  it("drops malformed reporting metadata", () => {
    expect(
      parseContextWindowEntry({ size: 1, used: 0 }, undefined, 0, {
        source: "copilot_native",
        availability: "unknown",
        coverage: "observed",
        revision: 1,
      }),
    ).toEqual(expect.objectContaining({ compactionReporting: undefined }));
  });

  it("keeps a newer count when a delayed gauge arrives", () => {
    const merged = mergeContextWindowEntry(
      { size: 272_000, used: 9_000, remaining: 263_000, efficiency: 3, compactionCount: 3 },
      { size: 272_000, used: 12_000, remaining: 260_000, efficiency: 4, compactionCount: 2 },
    );
    expect(merged.compactionCount).toBe(3);
    expect(merged.used).toBe(12_000);
  });

  it("rejects stale reporting revisions without lowering a newer count", () => {
    const merged = mergeContextWindowEntry(
      {
        size: 272_000,
        used: 9_000,
        remaining: 263_000,
        efficiency: 3,
        compactionCount: 5,
        compactionReporting: { ...observed, revision: 4, coverage: "partial" },
      },
      {
        size: 272_000,
        used: 12_000,
        remaining: 260_000,
        efficiency: 4,
        compactionCount: 4,
        compactionReporting: { ...observed, revision: 3 },
      },
    );
    expect(merged.compactionCount).toBe(5);
    expect(merged.compactionReporting?.revision).toBe(4);
    expect(merged.compactionReporting?.coverage).toBe("partial");
  });

  it("patches count and reporting without changing the gauge or inventing one", () => {
    expect(
      patchContextWindowCompaction(
        { size: 272_000, used: 12_171, remaining: 259_829, efficiency: 4, compactionCount: 1 },
        2,
        { ...observed, revision: 2 },
      ),
    ).toEqual(expect.objectContaining({ size: 272_000, used: 12_171, compactionCount: 2 }));
    expect(patchContextWindowCompaction(undefined, 2, observed)).toBeNull();
  });
});

describe("compaction freshness helpers", () => {
  it("merges the lifetime count monotonically", () => {
    expect(mergeCompactionCountValue(5, 2)).toBe(5);
    expect(mergeCompactionCountValue(undefined, 3)).toBe(3);
    expect(mergeCompactionCountValue(undefined, undefined)).toBeUndefined();
  });

  it("keeps the newer reporting revision", () => {
    expect(
      mergeCompactionReportingValue(
        { source: "copilot_native", availability: "available", coverage: "partial", revision: 4 },
        { source: "copilot_native", availability: "available", coverage: "observed", revision: 3 },
      ),
    ).toEqual(expect.objectContaining({ revision: 4, coverage: "partial" }));
  });

  it("keeps sticky partial coverage on an equal revision", () => {
    expect(
      mergeCompactionReportingValue(
        { source: "copilot_native", availability: "available", coverage: "partial", revision: 4 },
        { source: "copilot_native", availability: "available", coverage: "observed", revision: 4 },
      ),
    ).toEqual(expect.objectContaining({ revision: 4, coverage: "partial" }));
  });

  it("clears only the gauge and retains lifetime state", () => {
    expect(
      clearContextWindowGauge({
        size: 272_000,
        used: 9_695,
        remaining: 262_305,
        efficiency: 4,
        compactionCount: 3,
        compactionReporting: {
          source: "copilot_native",
          availability: "available",
          coverage: "observed",
          revision: 2,
        },
      }),
    ).toEqual(
      expect.objectContaining({
        size: 0,
        used: 0,
        compactionCount: 3,
        compactionReporting: expect.objectContaining({ revision: 2 }),
      }),
    );
  });
});

describe("reconcileContextWindowMetadata", () => {
  const entry = {
    size: 272_000,
    used: 9_000,
    remaining: 263_000,
    efficiency: 3,
    compactionCount: 3,
    compactionReporting: {
      source: "copilot_native" as const,
      availability: "available" as const,
      coverage: "observed" as const,
      revision: 4,
    },
  };

  it("applies every live gauge even when count and revision are unchanged", () => {
    const next = reconcileContextWindowMetadata({
      current: entry,
      gauge: { size: 272_000, used: 12_000, remaining: 260_000, efficiency: 4, source: "acp" },
      hasGauge: true,
      count: 3,
      reporting: entry.compactionReporting,
      timestamp: "live",
      allowExplicitClear: true,
    });
    expect(next).toEqual(
      expect.objectContaining({ used: 12_000, efficiency: 4, source: "acp", timestamp: "live" }),
    );
    expect(next?.compactionCount).toBe(3);
  });

  it("allows a newer count-only patch from a retained null gauge during hydration", () => {
    const next = reconcileContextWindowMetadata({
      current: entry,
      gauge: null,
      hasGauge: true,
      count: 4,
      reporting: entry.compactionReporting,
      timestamp: undefined,
      allowExplicitClear: false,
    });
    expect(next).toEqual(
      expect.objectContaining({ used: 9_000, compactionCount: 4, timestamp: undefined }),
    );
  });

  it("still ignores an older retained null gauge", () => {
    expect(
      reconcileContextWindowMetadata({
        current: entry,
        gauge: null,
        hasGauge: true,
        count: 2,
        reporting: { ...entry.compactionReporting, revision: 1 },
        timestamp: undefined,
        allowExplicitClear: false,
      }),
    ).toBeUndefined();
  });
});
