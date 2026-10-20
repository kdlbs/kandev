import type { Terminal } from "@xterm/xterm";
import { expect, it } from "vitest";
import { exposeBufferReader } from "./terminal-buffer-reader";

it.each([
  { left: "RESTORED_STOPPED_SHEL", right: "L", wrapped: true, expected: "RESTORED_STOPPED_SHELL" },
  {
    left: "RESTORED_STOPPED_SHEL",
    right: "L",
    wrapped: false,
    expected: "RESTORED_STOPPED_SHEL\nL",
  },
  { left: "word ", right: "next", wrapped: true, expected: "word next" },
])(
  "preserves logical output with a wrapped row: $wrapped",
  ({ left, right, wrapped, expected }) => {
    const rows = [
      { isWrapped: false, translateToString: (trim: boolean) => (trim ? left.trimEnd() : left) },
      { isWrapped: wrapped, translateToString: () => right },
    ];
    const terminal = {
      buffer: { active: { baseY: 0, cursorY: 1, getLine: (index: number) => rows[index] } },
      options: {},
    } as unknown as Terminal;
    const container = {} as HTMLDivElement & { __xtermReadBuffer: () => string };
    exposeBufferReader(container, terminal);
    expect(container.__xtermReadBuffer()).toBe(expected);
  },
);
