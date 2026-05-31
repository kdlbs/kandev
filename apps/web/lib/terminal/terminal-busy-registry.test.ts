import { describe, expect, it, beforeEach } from "vitest";
import type { Terminal } from "@xterm/xterm";
import {
  isTerminalBusy,
  markTerminalInput,
  markTerminalOutput,
  resetTerminalBusyRegistry,
  shouldConfirmTerminalClose,
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
    resetTerminalBusyRegistry();
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

  it("stays busy when cursor is on a blank line before prompt redraw", () => {
    markTerminalInput("term-1", "make\r");
    markTerminalOutput("term-1", mockTerminal(""));
    expect(isTerminalBusy("term-1")).toBe(true);
  });

  it("requires confirm for script terminals regardless of busy state", () => {
    expect(shouldConfirmTerminalClose("term-1", { type: "script" })).toBe(true);
    expect(shouldConfirmTerminalClose("term-1", { kind: "script" })).toBe(true);
  });

  it("requires confirm when busy for ordinary terminals", () => {
    markTerminalInput("term-1", "npm install\r");
    expect(shouldConfirmTerminalClose("term-1", { kind: "ordinary" })).toBe(true);
  });
});
