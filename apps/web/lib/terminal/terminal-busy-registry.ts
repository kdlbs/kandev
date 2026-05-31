import type { Terminal } from "@xterm/xterm";
import { stripAnsi } from "@/lib/utils/ansi";

/**
 * Client-only busy tracking for user shells. Mutated from xterm hooks in the
 * browser; guarded so accidental SSR imports never share state across requests.
 */
const busyByTerminalId = new Map<string, boolean>();

// A line "looks like a prompt" when its last non-space glyph is a shell
// sigil ($ # > %) that is either preceded by whitespace (bare prompts like
// "$ ") or sits at the end of a path/host-ish token containing @, ~ or /
// (e.g. "user@host:~/proj$", "/usr/bin%"). Requiring the @/~/ anchor for
// attached sigils keeps progress lines like "Building... 50%" or
// "logger.go:42" from being misread as a returned prompt.
const PROMPT_TAIL = /(?:(?:^|\s)|[\w.@~:/-]*[@~/][\w.@~:/-]*)[$#>%]\s*$/;

export type TerminalCloseConfirmOpts = {
  kind?: string;
  type?: string;
  initialCommand?: string;
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
