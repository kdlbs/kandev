import { describe, expect, it } from "vitest";
import { appendTerminalIfMissing, computeTerminalTabValue } from "./use-terminals-build";
import type { Terminal } from "./use-terminals-types";

function terminal(id: string, seq: number): Terminal {
  return {
    id,
    type: "shell",
    label: `Terminal ${seq}`,
    closable: true,
    kind: "ordinary",
    seq,
  };
}

describe("appendTerminalIfMissing", () => {
  it("does not duplicate a terminal already inserted by shell-list sync", () => {
    const existing = terminal("shell-1", 1);
    const result = appendTerminalIfMissing([existing], terminal("shell-1", 1));
    expect(result).toEqual([existing]);
  });

  it("appends a distinct terminal", () => {
    const existing = terminal("shell-1", 1);
    const next = terminal("shell-2", 2);
    expect(appendTerminalIfMissing([existing], next)).toEqual([existing, next]);
  });
});

describe("computeTerminalTabValue", () => {
  it("accepts a saved terminal id only after the current scope lists it", () => {
    const own = terminal("shell-own", 1);
    expect(computeTerminalTabValue(undefined, true, "shell-own", [own], true)).toBe("shell-own");
    expect(computeTerminalTabValue(undefined, true, "shell-foreign", [own], false)).toBe(
      "shell-own",
    );
  });

  it("rejects a stale store selection from another task", () => {
    const own = terminal("shell-own", 1);
    expect(computeTerminalTabValue("shell-foreign", false, null, [own], false)).toBe("shell-own");
  });

  it("falls back to commands when the scope has no shell", () => {
    expect(computeTerminalTabValue("shell-foreign", false, null, [], false)).toBe("commands");
  });
});
