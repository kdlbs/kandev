import type { Terminal } from "@xterm/xterm";

type TerminalContainerWithBuffer = HTMLDivElement & { __xtermReadBuffer?: () => string };

/** Expose buffer reader on the container for e2e tests (xterm renders to canvas). */
export function exposeBufferReader(container: HTMLDivElement, terminal: Terminal) {
  (container as TerminalContainerWithBuffer).__xtermReadBuffer = () => {
    const buf = terminal.buffer.active;
    const lines: string[] = [];
    for (let i = 0; i <= buf.baseY + buf.cursorY; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? "");
    }
    return lines.join("\n");
  };
}

export function clearBufferReader(container: HTMLDivElement) {
  (container as TerminalContainerWithBuffer).__xtermReadBuffer = undefined;
}
