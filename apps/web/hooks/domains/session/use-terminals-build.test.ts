import { describe, expect, it } from "vitest";
import { appendTerminalIfMissing } from "./use-terminals-build";
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
