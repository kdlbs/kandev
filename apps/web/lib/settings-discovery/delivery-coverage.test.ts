import { describe, expect, it } from "vitest";
import {
  SETTINGS_COVERAGE_INVENTORY,
  validateSettingsCoverageInventory,
} from "./coverage-inventory";

describe("settings delivery coverage", () => {
  it("has no pending eligible inventory entries", () => {
    expect(validateSettingsCoverageInventory(SETTINGS_COVERAGE_INVENTORY)).toEqual([]);
    expect(SETTINGS_COVERAGE_INVENTORY.some((entry) => entry.status === "supported")).toBe(true);
  });

  it("keeps every inventory entry independently traceable", () => {
    const ids = SETTINGS_COVERAGE_INVENTORY.map((entry) => entry.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(
      SETTINGS_COVERAGE_INVENTORY.every(
        (entry) => entry.owner.length > 0 && entry.sourcePaths.length > 0,
      ),
    ).toBe(true);
  });
});
