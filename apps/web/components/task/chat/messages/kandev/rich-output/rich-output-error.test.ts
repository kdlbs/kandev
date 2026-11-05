import { describe, expect, it } from "vitest";
import { getRichOutputErrorSummary, getRichOutputErrorText } from "./rich-output-error";

const FALLBACK = "No error details are available.";
const ERROR_DETAIL = "Error detail.";

describe("rich output error text", () => {
  it.each([
    ["Validation failed.", "Validation failed."],
    [{ message: "Message detail." }, "Message detail."],
    [{ error: ERROR_DETAIL }, ERROR_DETAIL],
    [{ message: "  ", error: ERROR_DETAIL }, ERROR_DETAIL],
  ])("selects supported retained error data", (result, expected) => {
    expect(getRichOutputErrorText(result, FALLBACK)).toBe(expected);
  });

  it.each([undefined, null, 42, [], { details: "Do not expose this object." }])(
    "uses the localized fallback for unsupported data",
    (result) => {
      expect(getRichOutputErrorText(result, FALLBACK)).toBe(FALLBACK);
    },
  );

  it("uses the first nonempty line for the summary and bounds its length", () => {
    expect(getRichOutputErrorSummary("\n  first line\r\nsecond line")).toBe("first line");
    expect(getRichOutputErrorSummary("x".repeat(161))).toBe(`${"x".repeat(159)}…`);
  });
});
