import { describe, expect, it } from "vitest";
import {
  changeIdentifiedSortRule,
  moveIdentifiedSortRule,
  removeIdentifiedSortRule,
  type IdentifiedSortRule,
} from "./sort-chain-editor-model";

const entries: IdentifiedSortRule[] = [
  { id: "running", rule: { key: "running", direction: "desc" } },
  { id: "red", rule: { key: "color", color: "red", direction: "desc" } },
];

describe("identified sort rules", () => {
  it("keeps identity when a rule field or color changes", () => {
    const colorChanged = changeIdentifiedSortRule(entries, 1, {
      key: "color",
      color: "blue",
      direction: "desc",
    });
    const fieldChanged = changeIdentifiedSortRule(entries, 1, {
      key: "lastActivityAt",
      direction: "desc",
    });

    expect(colorChanged[1]).toEqual({
      id: "red",
      rule: { key: "color", color: "blue", direction: "desc" },
    });
    expect(fieldChanged[1]).toEqual({
      id: "red",
      rule: { key: "lastActivityAt", direction: "desc" },
    });
  });

  it("moves identity with its rule and removes only the selected identity", () => {
    const moved = moveIdentifiedSortRule(entries, 1, -1);
    expect(moved.map((entry) => entry.id)).toEqual(["red", "running"]);
    expect(removeIdentifiedSortRule(moved, 1).map((entry) => entry.id)).toEqual(["red"]);
  });
});
