import { describe, expect, it } from "vitest";
import { formatVariableValue, templateText } from "./routine-row";

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

describe("templateText", () => {
  it("passes a string value through unchanged", () => {
    expect(templateText("Nightly summary")).toBe("Nightly summary");
  });

  it("falls back to an empty string for a non-string value instead of rendering it", () => {
    // task_template is an unvalidated backend string; a malformed value like
    // {"title":{"nested":1}} must not reach a JSX text node as an object.
    expect(templateText({ nested: 1 })).toBe("");
    expect(templateText(5)).toBe("");
    expect(templateText(null)).toBe("");
    expect(templateText(undefined)).toBe("");
  });
});
