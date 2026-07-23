import { describe, expect, it } from "vitest";
import { sessionStatusTooltip } from "./sessions-dropdown";

describe("sessionStatusTooltip", () => {
  it("prioritizes permission over clarification for input-capable sessions", () => {
    expect(sessionStatusTooltip("RUNNING", { permission: true, clarification: true })).toBe(
      "Permission requested",
    );
  });

  it("surfaces clarification over activity for input-capable sessions", () => {
    expect(
      sessionStatusTooltip("WAITING_FOR_INPUT", { permission: false, clarification: true }),
    ).toBe("Waiting for input");
  });

  it("labels background-idle sessions as running when no input is pending", () => {
    expect(
      sessionStatusTooltip(
        "WAITING_FOR_INPUT",
        { permission: false, clarification: false },
        "background",
      ),
    ).toBe("Background running");
  });

  it.each([
    ["STARTING", "Running"],
    ["COMPLETED", "Complete"],
    ["FAILED", "Failed"],
    ["CANCELLED", "Cancelled"],
  ] as const)("ignores stale pending input for %s sessions", (state, expected) => {
    expect(sessionStatusTooltip(state, { permission: true, clarification: true })).toBe(expected);
  });
});
