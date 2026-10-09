import { describe, expect, it } from "vitest";
import {
  getMessagePreview,
  MESSAGE_PREVIEW_MAX_CODE_UNITS,
  MESSAGE_PREVIEW_MAX_LINES,
} from "./message-preview";

describe("getMessagePreview", () => {
  it("keeps content at both limits unchanged", () => {
    const source = Array.from(
      { length: MESSAGE_PREVIEW_MAX_LINES },
      (_, index) => `line-${index}`,
    ).join("\n");

    const result = getMessagePreview(source, {
      maxCodeUnits: source.length,
    });

    expect(result).toEqual({
      content: source,
      truncated: false,
      logicalLines: MESSAGE_PREVIEW_MAX_LINES,
      codeUnits: source.length,
    });
  });

  it("stops before the line after the line budget", () => {
    const source = Array.from(
      { length: MESSAGE_PREVIEW_MAX_LINES + 1 },
      (_, index) => `line-${index}`,
    ).join("\n");

    const result = getMessagePreview(source);

    expect(result.truncated).toBe(true);
    expect(result.logicalLines).toBe(MESSAGE_PREVIEW_MAX_LINES);
    expect(result.content).toBe(
      `${Array.from({ length: MESSAGE_PREVIEW_MAX_LINES }, (_, index) => `line-${index}`).join("\n")}\n`,
    );
    expect(result.content).not.toContain(`line-${MESSAGE_PREVIEW_MAX_LINES}`);
  });

  it("stops at the code-unit budget without splitting a long line", () => {
    const source = "x".repeat(MESSAGE_PREVIEW_MAX_CODE_UNITS + 1);

    const result = getMessagePreview(source);

    expect(result.content).toBe("x".repeat(MESSAGE_PREVIEW_MAX_CODE_UNITS));
    expect(result.codeUnits).toBe(MESSAGE_PREVIEW_MAX_CODE_UNITS);
    expect(result.truncated).toBe(true);
  });

  it("keeps CRLF together at a code-unit boundary", () => {
    const result = getMessagePreview("first\r\nsecond", { maxCodeUnits: 5 });

    expect(result.content).toBe("first");
    expect(result.content.endsWith("\r")).toBe(false);
    expect(result.truncated).toBe(true);
  });

  it("counts CR and LF as separate logical line endings", () => {
    const result = getMessagePreview("first\rsecond\nthird");

    expect(result.logicalLines).toBe(3);
    expect(result.truncated).toBe(false);
  });

  it("does not split a surrogate pair", () => {
    const result = getMessagePreview("a🙂b", { maxCodeUnits: 2 });

    expect(result.content).toBe("a");
    expect(result.content).not.toContain("\uFFFD");
    expect(result.truncated).toBe(true);
  });

  it("returns empty content when no line or code-unit budget remains", () => {
    expect(getMessagePreview("content", { maxLines: 0 }).content).toBe("");
    expect(getMessagePreview("content", { maxCodeUnits: 0 }).content).toBe("");
  });
});
