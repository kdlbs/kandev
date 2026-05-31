import { describe, expect, it, beforeEach } from "vitest";
import type { Terminal } from "@xterm/xterm";
import {
  clearTerminalBusy,
  isTerminalBusy,
  markTerminalInput,
  markTerminalOutput,
} from "./terminal-busy-registry";

function mockTerminal(lineText: string): Terminal {
  return {
    buffer: {
      active: {
        baseY: 0,
        cursorY: 0,
        getLine: () => ({
          translateToString: () => lineText,
        }),
      },
    },
  } as unknown as Terminal;
}

describe("terminal-busy-registry", () => {
  beforeEach(() => {
    clearTerminalBusy("term-1");
  });

  it("starts idle", () => {
    expect(isTerminalBusy("term-1")).toBe(false);
  });

  it("marks busy after Enter", () => {
    markTerminalInput("term-1", "sleep 10\r");
    expect(isTerminalBusy("term-1")).toBe(true);
  });

  it("clears busy when the buffer returns to a shell prompt", () => {
    markTerminalInput("term-1", "echo hi\r");
    markTerminalOutput("term-1", mockTerminal("user@host:~/proj$ "));
    expect(isTerminalBusy("term-1")).toBe(false);
  });

  it("stays busy while output has no prompt tail", () => {
    markTerminalInput("term-1", "make dev\r");
    markTerminalOutput("term-1", mockTerminal("DEBUG logger/logger.go:42"));
    expect(isTerminalBusy("term-1")).toBe(true);
  });
});
