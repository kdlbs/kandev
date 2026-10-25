import type { Terminal } from "@xterm/xterm";

type TerminalContainerWithBuffer = HTMLDivElement & {
  __xtermReadBuffer?: () => string;
  __xtermReadViewportY?: () => number;
  __xtermGetFontFamily?: () => string;
  __xtermGetFontSize?: () => number;
  __xtermReadTheme?: () => Record<string, string | number | string[] | undefined>;
};

/** Expose buffer reader on the container for e2e tests (xterm renders to canvas). */
export function exposeBufferReader(container: HTMLDivElement, terminal: Terminal) {
  const c = container as TerminalContainerWithBuffer;
  c.__xtermReadBuffer = () => {
    const buf = terminal.buffer.active;
    const lastRow = buf.baseY + buf.cursorY;
    let text = "";
    for (let i = 0; i <= lastRow; i++) {
      const line = buf.getLine(i);
      if (i > 0 && !line?.isWrapped) text += "\n";
      const continues = i < lastRow && buf.getLine(i + 1)?.isWrapped;
      text += line?.translateToString(!continues) ?? "";
    }
    return text;
  };
  c.__xtermReadViewportY = () => terminal.buffer.active.viewportY;
  c.__xtermGetFontFamily = () => terminal.options.fontFamily ?? "";
  c.__xtermGetFontSize = () => terminal.options.fontSize ?? 13;
  c.__xtermReadTheme = () => ({
    ...(terminal.options.theme ?? {}),
    minimumContrastRatio: terminal.options.minimumContrastRatio,
  });
}

export function clearBufferReader(container: HTMLDivElement) {
  const c = container as TerminalContainerWithBuffer;
  c.__xtermReadBuffer = undefined;
  c.__xtermReadViewportY = undefined;
  c.__xtermGetFontFamily = undefined;
  c.__xtermGetFontSize = undefined;
  c.__xtermReadTheme = undefined;
}
