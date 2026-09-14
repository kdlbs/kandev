import { describe, expect, it } from "vitest";
import {
  SETTINGS_COVERAGE_INVENTORY,
  SETTINGS_COVERAGE_RESOURCE_TYPES,
  validateMutableFieldParity,
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

  it("compares catalog fields with independently reflected mutable DTO fields", () => {
    const mutableFields = (
      contract as typeof contract & {
        mutable_fields?: Record<string, string[]>;
      }
    ).mutable_fields;
    expect(mutableFields).toBeDefined();

    expect(validateMutableFieldParity(mutableFields ?? {}, contract.domains)).toEqual([]);
    for (const resourceType of ["agent_profile", "user_settings"]) {
      for (const fieldPath of mutableFields?.[resourceType] ?? []) {
        expect(
          SETTINGS_COVERAGE_INVENTORY.some(
            (entry) => entry.domain === resourceType && entry.fieldPath === fieldPath,
          ),
          `${resourceType}.${fieldPath} missing from coverage inventory`,
        ).toBe(true);
      }
    }
  });

  it("detects added DTO fields and removed catalog descriptors", () => {
    const mutableFields = (
      contract as typeof contract & {
        mutable_fields: Record<string, string[]>;
      }
    ).mutable_fields;
    const userDomain = contract.domains.find((item) => item.resource_type === "user_settings");
    expect(userDomain).toBeDefined();
    const addedField = {
      ...mutableFields,
      user_settings: [...mutableFields.user_settings, "future_mutable_field"],
    };
    expect(validateMutableFieldParity(addedField, contract.domains)).toContain(
      "user_settings.future_mutable_field: mutable DTO field is missing from catalog",
    );

    const catalogWithoutDescriptor = contract.domains.map((domain) =>
      domain.resource_type === "user_settings"
        ? {
            ...domain,
            fields: domain.fields.filter((field) => field.field_path !== "keyboard_shortcuts"),
          }
        : domain,
    );
    expect(validateMutableFieldParity(mutableFields, catalogWithoutDescriptor)).toContain(
      "user_settings.keyboard_shortcuts: mutable DTO field is missing from catalog",
    );
  });
});
