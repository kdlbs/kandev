import { describe, expect, it } from "vitest";
import { coerceCatchUpMax } from "./catch-up-max";

describe("coerceCatchUpMax", () => {
  it("passes through a valid positive number", () => {
    expect(coerceCatchUpMax(10)).toBe(10);
  });

  it("passes through a valid numeric string", () => {
    expect(coerceCatchUpMax("10")).toBe(10);
  });

  it("coerces zero to 25", () => {
    expect(coerceCatchUpMax(0)).toBe(25);
  });

  it("coerces a negative number to 25", () => {
    expect(coerceCatchUpMax(-5)).toBe(25);
  });

  it("coerces an unparsable string to 25", () => {
    expect(coerceCatchUpMax("abc")).toBe(25);
  });

  it("coerces undefined to 25", () => {
    expect(coerceCatchUpMax(undefined)).toBe(25);
  });

  it("coerces an empty string to 25", () => {
    expect(coerceCatchUpMax("")).toBe(25);
  });
});
