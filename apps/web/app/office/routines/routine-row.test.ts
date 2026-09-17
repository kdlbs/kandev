import { describe, expect, it } from "vitest";
import { formatVariableValue } from "./routine-row";

describe("formatVariableValue", () => {
  it("renders a declared variable's default string, not the wrapping object", () => {
    expect(formatVariableValue({ default: "prod" })).toBe("prod");
  });

  it("passes a bare string value through unchanged", () => {
    expect(formatVariableValue("prod")).toBe("prod");
  });

  it("falls back to an empty string when default is missing or not a string", () => {
    expect(formatVariableValue({})).toBe("");
    expect(formatVariableValue({ default: 5 })).toBe("");
    expect(formatVariableValue(null)).toBe("");
    expect(formatVariableValue(undefined)).toBe("");
  });
});
