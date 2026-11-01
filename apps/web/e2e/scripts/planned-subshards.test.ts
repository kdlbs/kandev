import { describe, expect, it } from "vitest";
import type { PlannedUnit, Shard } from "./plan-shards";
import { partitionShard, processCount } from "./planned-subshards";

function unit(file: string, seconds: number, project = "chromium"): PlannedUnit {
  return {
    id: `${project}:${file}`,
    project,
    file,
    fileHash: "hash",
    testCount: 1,
    estimatedSeconds: seconds,
    classification: "profile",
  };
}

function shard(units: PlannedUnit[]): Shard {
  return {
    index: 7,
    files: [...new Set(units.map((entry) => entry.file))],
    units,
    predictedSeconds: units.reduce((total, entry) => total + entry.estimatedSeconds, 0),
  };
}

describe("bounded partitions within a planned shard", () => {
  it.each([
    ["3", "3", 4, "normal", 3],
    ["4", "4", 4, "normal", 4],
    ["4", "3", 4, "normal", 3],
    ["4", "2", 4, "normal", 2],
    ["4", "4", 3, "normal", 2],
    ["4", "4", 2, "normal", 1],
    ["4", "4", 4, "containers", 1],
    ["4", "invalid", 4, "normal", 1],
    ["3", "2", 4, "normal", 2],
    ["3", "1", 4, "normal", 1],
    ["3", "3", 2, "normal", 1],
    ["3", "3", 1, "normal", 1],
    ["1", "3", 4, "normal", 1],
    ["3", "3", 4, "containers", 1],
    ["3", "invalid", 4, "normal", 1],
    ["3", "3", Number.NaN, "normal", 1],
  ] as const)(
    "bounds %s requested processes by memory %s, CPUs %s, and cohort %s",
    (requested, memory, cpus, cohort, expected) => {
      expect(processCount(requested, memory, cpus, cohort)).toBe(expected);
    },
  );

  it.each(["0", "5", "1.5", "invalid"])("rejects an invalid process request %s", (requested) => {
    expect(() => processCount(requested, "3", 4, "normal")).toThrow(/count/);
  });

  it.each([
    [2, 2],
    [3, 3],
    [4, 4],
  ] as const)(
    "balances %i project-specific partitions without cross-project file runs",
    (count, partCount) => {
      const source = shard([
        unit("shared.spec.ts", 10),
        unit("shared.spec.ts", 9, "mobile-chrome"),
        unit("second.spec.ts", 18),
        unit("third.spec.ts", 4),
        unit("fourth.spec.ts", 3),
      ]);
      const before = structuredClone(source);
      const parts = partitionShard(source, count);
      expect(parts).toHaveLength(partCount);
      expect(
        parts
          .flatMap((part) => part.units)
          .map((entry) => `${entry.project}::${entry.file}`)
          .sort(),
      ).toEqual(source.units.map((entry) => `${entry.project}::${entry.file}`).sort());
      for (const part of parts) {
        expect(new Set(part.units.map((entry) => entry.project)).size).toBe(1);
        expect(part.files).toEqual([...new Set(part.units.map((entry) => entry.file))]);
      }
      expect(source).toEqual(before);
    },
  );

  it("keeps separate project invocations when the requested process count is one", () => {
    const parts = partitionShard(
      shard([unit("shared.spec.ts", 2), unit("shared.spec.ts", 3, "mobile-chrome")]),
      1,
    );

    expect(parts).toHaveLength(2);
    expect(parts.map((part) => part.units[0]!.project).sort()).toEqual([
      "chromium",
      "mobile-chrome",
    ]);
  });

  it("uses deterministic file ordering when estimated durations tie", () => {
    const units = [unit("c.spec.ts", 1), unit("b.spec.ts", 1), unit("a.spec.ts", 1)];
    expect(partitionShard(shard(units), 2).map((part) => part.files)).toEqual([
      ["a.spec.ts", "c.spec.ts"],
      ["b.spec.ts"],
    ]);
    expect(partitionShard(shard(units), 2)).toEqual(partitionShard(shard([...units].reverse()), 2));
  });

  it("starts only nonempty partitions and preserves an empty parent selection", () => {
    expect(partitionShard(shard([unit("one.spec.ts", 1)]), 3)).toHaveLength(1);
    expect(partitionShard(shard([]), 3)).toEqual([]);
  });

  it.each([0, 5, 1.5, Number.NaN])("rejects an invalid process count %s", (count) => {
    expect(() => partitionShard(shard([unit("one.spec.ts", 1)]), count)).toThrow(/count/);
  });
});
