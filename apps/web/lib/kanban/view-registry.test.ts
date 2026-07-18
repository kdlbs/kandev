import { describe, expect, it } from "vitest";
import { getEffectiveView } from "./view-registry";

describe("mobile Kanban view resolution", () => {
  it("falls back from a saved Pipeline view to Kanban on mobile", () => {
    expect(getEffectiveView("graph2", true).id).toBe("kanban");
  });

  it("keeps a saved Pipeline view on non-mobile layouts", () => {
    expect(getEffectiveView("graph2", false).id).toBe("graph2");
  });
});
