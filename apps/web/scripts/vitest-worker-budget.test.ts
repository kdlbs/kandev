import { describe, expect, it } from "vitest";
import { resolveMaxWorkers } from "./vitest-worker-budget";

describe("resolveMaxWorkers", () => {
  it("uses the twenty-percent local default when no override is set", () => {
    expect(resolveMaxWorkers(undefined, false, false, 10)).toBe("20%");
  });

  it("caps an unsafe local percentage override", () => {
    expect(resolveMaxWorkers("100%", false, false, 10)).toBe("20%");
  });

  it("caps an unsafe local numeric override to available capacity", () => {
    expect(resolveMaxWorkers("10", false, false, 10)).toBe(2);
  });

  it("keeps a safe local percentage override", () => {
    expect(resolveMaxWorkers("10%", false, false, 10)).toBe("10%");
  });

  it("falls back for an invalid zero-worker percentage", () => {
    expect(resolveMaxWorkers("0%", false, false, 10)).toBe("20%");
  });

  it("requires an explicit opt-in for unsafe local capacity", () => {
    expect(resolveMaxWorkers("100%", false, true, 10)).toBe("100%");
    expect(resolveMaxWorkers("100%", true, false, 10)).toBe("100%");
  });
});
