import type { Terminal } from "@xterm/xterm";

type TerminalContainerWithBuffer = HTMLDivElement & {
  __xtermReadBuffer?: () => string;
  __xtermReadViewportY?: () => number;
  __xtermGetFontFamily?: () => string;
  __xtermGetFontSize?: () => number;
};

/** Expose buffer reader on the container for e2e tests (xterm renders to canvas). */
export function exposeBufferReader(container: HTMLDivElement, terminal: Terminal) {
  const c = container as TerminalContainerWithBuffer;
  c.__xtermReadBuffer = () => {
    const buf = terminal.buffer.active;
    const lines: string[] = [];
    for (let i = 0; i <= buf.baseY + buf.cursorY; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? "");
    }
    return lines.join("\n");
  };
  c.__xtermReadViewportY = () => terminal.buffer.active.viewportY;
  c.__xtermGetFontFamily = () => terminal.options.fontFamily ?? "";
  c.__xtermGetFontSize = () => terminal.options.fontSize ?? 13;
}

export function clearBufferReader(container: HTMLDivElement) {
  const c = container as TerminalContainerWithBuffer;
  c.__xtermReadBuffer = undefined;
  c.__xtermReadViewportY = undefined;
  c.__xtermGetFontFamily = undefined;
  c.__xtermGetFontSize = undefined;
}
