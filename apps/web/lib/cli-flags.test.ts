import { describe, it, expect } from "vitest";
import { areCLIFlagsEqual } from "./cli-flags";
import type { CLIFlag } from "@/lib/types/http";

const flag = (f: string, enabled = true, description = ""): CLIFlag => ({
  flag: f,
  enabled,
  description,
});

describe("areCLIFlagsEqual", () => {
  it("two empty lists are equal", () => {
    expect(areCLIFlagsEqual([], [])).toBe(true);
  });

  it("null and undefined are treated as empty", () => {
    expect(areCLIFlagsEqual(null, undefined)).toBe(true);
    expect(areCLIFlagsEqual(null, [])).toBe(true);
    expect(areCLIFlagsEqual([], undefined)).toBe(true);
  });

  it("same flag same state", () => {
    expect(areCLIFlagsEqual([flag("--x")], [flag("--x")])).toBe(true);
  });

  it("different lengths are not equal", () => {
    expect(areCLIFlagsEqual([flag("--x")], [])).toBe(false);
    expect(areCLIFlagsEqual([], [flag("--x")])).toBe(false);
  });

  it("same flag different enabled state", () => {
    expect(areCLIFlagsEqual([flag("--x", true)], [flag("--x", false)])).toBe(false);
  });

  it("same flag different description", () => {
    expect(areCLIFlagsEqual([flag("--x", true, "a")], [flag("--x", true, "b")])).toBe(false);
  });

  it("null description equals empty string description", () => {
    const a: CLIFlag[] = [{ flag: "--x", enabled: true, description: null as unknown as string }];
    const b: CLIFlag[] = [{ flag: "--x", enabled: true, description: "" }];
    expect(areCLIFlagsEqual(a, b)).toBe(true);
  });

  it("order matters", () => {
    const a = [flag("--a"), flag("--b")];
    const b = [flag("--b"), flag("--a")];
    expect(areCLIFlagsEqual(a, b)).toBe(false);
  });
});
