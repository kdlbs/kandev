import { describe, expect, it } from "vitest";
import type { PriorityMeta } from "@/lib/state/slices/office/types";
import { buildPriorityOptions } from "./new-task-bottom-bar";

function makePriorityMeta(id: string, order: number): PriorityMeta {
  return { id, label: id, order, color: "", value: order };
}

describe("buildPriorityOptions", () => {
  it("accepts canonical priorities and rejects non-task metadata", () => {
    const priorities = ["critical", "high", "medium", "low", "none", "custom"].map((id, order) =>
      makePriorityMeta(id, order),
    );

    expect(buildPriorityOptions(priorities, (key) => key).map((option) => option.value)).toEqual([
      "critical",
      "high",
      "medium",
      "low",
    ]);
  });

  it("uses all canonical priorities when metadata is absent", () => {
    expect(buildPriorityOptions(undefined, (key) => key).map((option) => option.value)).toEqual([
      "critical",
      "high",
      "medium",
      "low",
    ]);
  });
});
