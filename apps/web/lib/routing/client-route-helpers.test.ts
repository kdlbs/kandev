import { describe, expect, it } from "vitest";

import { toRouteErrorState } from "./client-route-helpers";

describe("toRouteErrorState", () => {
  it("uses the error message when available", () => {
    expect(toRouteErrorState(new Error("network down"))).toEqual({
      status: "error",
      message: "network down",
    });
  });

  it("falls back for non-Error values", () => {
    expect(toRouteErrorState("offline", "Failed to load routine")).toEqual({
      status: "error",
      message: "Failed to load routine",
    });
  });
});
