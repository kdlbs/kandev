import { describe, expect, it } from "vitest";
import { getEffectiveTaskListingView, parseTaskListingView } from "./view-preference";
import { resolveTaskListingNavigation } from "./view-navigation";

describe("Graph listing view", () => {
  it("is an accepted stored view", () => {
    expect(parseTaskListingView(JSON.stringify("graph"))).toBe("graph");
  });

  it("falls back to Kanban on phones", () => {
    expect(getEffectiveTaskListingView("graph", true)).toBe("kanban");
    expect(getEffectiveTaskListingView("graph", false)).toBe("graph");
  });

  it("lives on the Home board and is gated like Pipeline", () => {
    expect(
      resolveTaskListingNavigation({ view: "graph", currentPage: "kanban", workspaceId: "w" }),
    ).toEqual({ view: "graph", href: null });
    expect(
      resolveTaskListingNavigation({
        view: "graph",
        currentPage: "kanban",
        workspaceId: "w",
        allowPipeline: false,
      }),
    ).toBeNull();
  });
});
