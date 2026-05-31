import type { Terminal } from "@xterm/xterm";
import { stripAnsi } from "@/lib/utils/ansi";

/** Tracks whether a user shell likely has a foreground command running. */
const busyByTerminalId = new Map<string, boolean>();

// A line "looks like a prompt" when its last non-space glyph is a shell
// sigil ($ # > %) that is either preceded by whitespace (bare prompts like
// "$ ") or sits at the end of a path/host-ish token containing @, ~ or /
// (e.g. "user@host:~/proj$", "/usr/bin%"). Requiring the @/~/ anchor for
// attached sigils keeps progress lines like "Building... 50%" or
// "logger.go:42" from being misread as a returned prompt.
const PROMPT_TAIL = /(?:(?:^|\s)|[\w.@~:/-]*[@~/][\w.@~:/-]*)[$#>%]\s*$/;

export function markTerminalInput(terminalId: string, data: string): void {
  if (data.includes("\r") || data.includes("\n")) {
    busyByTerminalId.set(terminalId, true);
  }
}

export function markTerminalOutput(terminalId: string, terminal: Terminal): void {
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

function bufferLooksAtPrompt(terminal: Terminal): boolean {
  const buf = terminal.buffer.active;
  const line = buf.getLine(buf.baseY + buf.cursorY)?.translateToString(true) ?? "";
  const trimmed = stripAnsi(line).trimEnd();
  if (trimmed === "") return true;
  return PROMPT_TAIL.test(trimmed);
}
