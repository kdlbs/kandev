import { describe, expect, it } from "vitest";

import { ensureValidPort } from "./ports";

describe("ensureValidPort", () => {
  it("returns undefined for undefined input", () => {
    expect(ensureValidPort(undefined, "test")).toBeUndefined();
  });

  it("returns the port for valid values", () => {
    expect(ensureValidPort(8080, "test")).toBe(8080);
  });

  it("accepts port 1 (minimum)", () => {
    expect(ensureValidPort(1, "test")).toBe(1);
  });

  it("accepts port 65535 (maximum)", () => {
    expect(ensureValidPort(65535, "test")).toBe(65535);
  });

  it("throws for port 0", () => {
    expect(() => ensureValidPort(0, "backend")).toThrow(
      "backend must be an integer between 1 and 65535",
    );
  });

  it("throws for port above 65535", () => {
    expect(() => ensureValidPort(65536, "web")).toThrow(
      "web must be an integer between 1 and 65535",
    );
  });

  it("throws for negative port", () => {
    expect(() => ensureValidPort(-1, "test")).toThrow();
  });

  it("throws for floating point port", () => {
    expect(() => ensureValidPort(8080.5, "test")).toThrow();
  });

  it("throws for NaN", () => {
    expect(() => ensureValidPort(NaN, "test")).toThrow();
  });
});
