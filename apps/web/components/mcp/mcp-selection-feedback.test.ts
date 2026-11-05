import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { mcpSelectionErrorKey } from "./mcp-selection-feedback";

vi.mock("@/lib/platform/backend-reload-coordinator", () => ({
  signalBackendReloadRequired: vi.fn(),
}));

describe("MCP selection feedback", () => {
  it("explains an ordinary forbidden save as a connection problem", () => {
    const error = new ApiError("forbidden", 403, { error_code: "forbidden" });
    expect(mcpSelectionErrorKey(error)).toBe("settings:mcpSelectionForbidden");
  });

  it.each([
    new Error("network failure"),
    new ApiError("save failed", 500, { error_code: "forbidden" }),
    new ApiError("access rejected", 403, { error_code: "other_policy" }),
  ])("keeps unrelated save failures generic", (error) => {
    expect(mcpSelectionErrorKey(error)).toBe("settings:mcpSelectionSaveFailed");
  });

  it("leaves handled recovery to its existing owner", () => {
    const error = new ApiError("stale page", 403, {
      error_code: "interim_settings_interlock_required",
    });
    error.handled = true;
    expect(mcpSelectionErrorKey(error)).toBeNull();
  });

  it("does not show an error without a failure", () => {
    expect(mcpSelectionErrorKey(undefined)).toBeNull();
  });
});

it("offers conflict-specific feedback for stale choices", () => {
  expect(
    mcpSelectionErrorKey(new ApiError("changed", 409, { code: "mcp_selection_conflict" })),
  ).toBe("settings:mcpSelectionConflict");
});
