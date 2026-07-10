import type { Terminal } from "@xterm/xterm";
import { stripAnsi } from "@/lib/utils/ansi";

/**
 * Client-only busy tracking for user shells. Mutated from xterm hooks in the
 * browser; guarded so accidental SSR imports never share state across requests.
 */
const busyByTerminalId = new Map<string, boolean>();

// A line "looks like a prompt" when it ends with a shell sigil ($ # > %).
// Bare sigils match only at line start (optional whitespace or a parenthesized
// env marker like "(venv) "). Path-attached sigils require @, ~, or / and
// allow optional prefix text (fish brackets, etc.). Bare `> ` is excluded.
const PROMPT_TAIL =
  /^(?:(?:\([^)\n]*\)\s+|\s*)[$#%]|(?:.*\s)?[\w.@~:/-]*[@~/][\w.@~:/-]*\]?[$#>%])\s*$/;

export type TerminalCloseConfirmOpts = {
  kind?: string;
  type?: string;
};

/** True when closing should warn because a script/non-ordinary shell or a busy command is active. */
export function shouldConfirmTerminalClose(
  terminalId: string,
  opts: TerminalCloseConfirmOpts,
): boolean {
  if (opts.type === "script" || opts.kind === "script") return true;
  if (opts.kind && opts.kind !== "ordinary") return true;
  return isTerminalBusy(terminalId);
}

export function markTerminalInput(terminalId: string, data: string): void {
  if (typeof window === "undefined") return;
  if (data.includes("\r") || data.includes("\n")) {
    busyByTerminalId.set(terminalId, true);
  }
}

export function markTerminalOutput(terminalId: string, terminal: Terminal): void {
  if (typeof window === "undefined") return;
  if (!busyByTerminalId.get(terminalId)) return;
  if (bufferLooksAtPrompt(terminal)) {
    busyByTerminalId.set(terminalId, false);
  }
}

export function isTerminalBusy(terminalId: string): boolean {
  return busyByTerminalId.get(terminalId) ?? false;
}

export function clearTerminalBusy(terminalId: string): void {
  busyByTerminalId.delete(terminalId);
}

/** Test-only: flush all busy state between cases. */
export function resetTerminalBusyRegistry(): void {
  busyByTerminalId.clear();
}

function bufferLooksAtPrompt(terminal: Terminal): boolean {
  const buf = terminal.buffer.active;
  const line = buf.getLine(buf.baseY + buf.cursorY)?.translateToString(true) ?? "";
  const trimmed = stripAnsi(line).trimEnd();
  // Blank lines appear briefly between command output and prompt redraw — do
  // not treat them as "back at the shell".
  if (trimmed === "") return false;
  return PROMPT_TAIL.test(trimmed);
}
