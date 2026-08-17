import { describe, expect, it } from "vitest";
import { fromStrategyValue, MCP_STRATEGY_NONE, toStrategyValue } from "./mcp-strategy-select";

// Radix Select cannot hold an empty-string value, so "no MCP injection" needs a
// sentinel in the control and "" on the wire. If the mapping leaks, the backend
// receives "__none__", rejects it as an unknown strategy, and the user cannot
// turn MCP off again.
describe("MCP strategy value mapping", () => {
  it("maps an unset strategy to the sentinel", () => {
    expect(toStrategyValue(undefined)).toBe(MCP_STRATEGY_NONE);
    expect(toStrategyValue("")).toBe(MCP_STRATEGY_NONE);
  });

  it("passes a real strategy key through unchanged", () => {
    expect(toStrategyValue("claude")).toBe("claude");
  });

  it("maps the sentinel back to an empty key for the API", () => {
    expect(fromStrategyValue(MCP_STRATEGY_NONE)).toBe("");
  });

  it("passes a real strategy key back unchanged", () => {
    expect(fromStrategyValue("codex")).toBe("codex");
  });

  it("round-trips every value the control can hold", () => {
    for (const key of ["", "claude", "codex", "cursor", "opencode", "pi"]) {
      expect(fromStrategyValue(toStrategyValue(key))).toBe(key);
    }
  });
});
