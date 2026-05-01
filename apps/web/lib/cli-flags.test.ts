import { describe, it, expect } from "vitest";
import { areCLIFlagsEqual, seedDefaultCLIFlags } from "./cli-flags";
import type { CLIFlag, PermissionSetting } from "@/lib/types/http";

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

const setting = (overrides: Partial<PermissionSetting> = {}): PermissionSetting => ({
  supported: true,
  default: true,
  label: "Allow indexing",
  description: "Enable workspace indexing without confirmation",
  apply_method: "cli_flag",
  cli_flag: "--allow-indexing",
  ...overrides,
});

describe("seedDefaultCLIFlags", () => {
  it("returns empty list when no permission settings target a CLI flag", () => {
    expect(seedDefaultCLIFlags({})).toEqual([]);
  });

  it("seeds curated cli_flag entries with their defaults", () => {
    const out = seedDefaultCLIFlags({ allow_indexing: setting() });
    expect(out).toEqual([
      {
        description: "Enable workspace indexing without confirmation",
        flag: "--allow-indexing",
        enabled: true,
      },
    ]);
  });

  it("skips unsupported, non-cli_flag, and missing-cli_flag entries", () => {
    const out = seedDefaultCLIFlags({
      unsupported: setting({ supported: false }),
      modeBased: setting({ apply_method: "mode" }),
      missingFlag: setting({ cli_flag: "" }),
    });
    expect(out).toEqual([]);
  });

  it("appends cli_flag_value to flag text when present", () => {
    const out = seedDefaultCLIFlags({
      allow_all: setting({ cli_flag: "--allow", cli_flag_value: "all" }),
    });
    expect(out[0].flag).toBe("--allow all");
  });

  it("falls back to label when description is empty", () => {
    const out = seedDefaultCLIFlags({
      allow_indexing: setting({ description: "" }),
    });
    expect(out[0].description).toBe("Allow indexing");
  });

  it("sorts results by flag text for stable order", () => {
    const out = seedDefaultCLIFlags({
      b: setting({ cli_flag: "--b-flag" }),
      a: setting({ cli_flag: "--a-flag" }),
    });
    expect(out.map((f) => f.flag)).toEqual(["--a-flag", "--b-flag"]);
  });
});
