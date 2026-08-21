import { describe, expect, it } from "vitest";
import { PANEL_REGISTRY, REUSABLE_PANEL_IDS } from "@/lib/state/layout-manager";
import { placeholderComponents } from "./layout-editor";

describe("layout editor placeholder components", () => {
  it("renders every reusable panel the Add panel menu offers", () => {
    for (const id of REUSABLE_PANEL_IDS) {
      const entry = PANEL_REGISTRY[id];
      expect(entry, `PANEL_REGISTRY missing entry for reusable panel ${id}`).toBeDefined();
      expect(
        placeholderComponents[entry?.component],
        `placeholder for reusable panel ${id} (component ${entry?.component})`,
      ).toBeDefined();
    }
  });
});
