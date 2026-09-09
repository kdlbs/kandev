import { describe, expect, it } from "vitest";
import {
  SETTINGS_COVERAGE_INVENTORY,
  SETTINGS_COVERAGE_RESOURCE_TYPES,
  validateSettingsCoverageInventory,
} from "./coverage-inventory";
import contract from "./contract.generated.json";

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

  it("keeps integration and automation coverage mapped to concrete resources", () => {
    const catalogTypes = new Set(contract.domains.map((domain) => domain.resource_type));
    for (const resourceTypes of Object.values(SETTINGS_COVERAGE_RESOURCE_TYPES)) {
      for (const resourceType of resourceTypes) {
        expect(catalogTypes.has(resourceType), resourceType).toBe(true);
      }
    }
  });
});
