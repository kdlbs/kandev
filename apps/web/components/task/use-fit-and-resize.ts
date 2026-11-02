import { useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { log } from "./terminal-debug";

export const MIN_WIDTH = 100;
export const MIN_HEIGHT = 100;

export type FitAndResizeOptions = {
  xtermRef: React.MutableRefObject<Terminal | null>;
  fitAddonRef: React.MutableRefObject<FitAddon | null>;
  terminalRef: React.RefObject<HTMLDivElement | null>;
  lastDimensionsRef: React.MutableRefObject<{ cols: number; rows: number }>;
  sendResize: (cols: number, rows: number) => void;
};

function sendFallbackResize(
  force: boolean,
  terminal: Terminal,
  lastDimensionsRef: React.MutableRefObject<{ cols: number; rows: number }>,
  sendResize: (cols: number, rows: number) => void,
) {
  if (!force) return;
  const dimensions = {
    cols: Math.max(1, terminal.cols || 80),
    rows: Math.max(1, terminal.rows || 24),
  };
  lastDimensionsRef.current = dimensions;
  sendResize(dimensions.cols, dimensions.rows);
}

export function useFitAndResize({
  xtermRef,
  fitAddonRef,
  terminalRef,
  lastDimensionsRef,
  sendResize,
}: FitAndResizeOptions) {
  return useCallback(
    (force = false) => {
      const terminal = xtermRef.current;
      const fitAddon = fitAddonRef.current;
      const container = terminalRef.current;
      if (!terminal || !fitAddon || !container) {
        log("fitAndResize: missing refs");
        return;
      }
      const rect = container.getBoundingClientRect();
      if (rect.width < MIN_WIDTH || rect.height < MIN_HEIGHT) {
        log("fitAndResize: container too small");
        sendFallbackResize(force, terminal, lastDimensionsRef, sendResize);
        return;
      }
      try {
        fitAddon.fit();
        log("fitAndResize: fit done", terminal.cols, "x", terminal.rows);
      } catch (e) {
        log("fitAndResize: fit failed", e);
        return;
      }
      const { cols, rows } = terminal;
      const last = lastDimensionsRef.current;
      const changed = cols !== last.cols || rows !== last.rows;
      const wasZero = last.cols === 0 && last.rows === 0;
      if (force || changed) {
        lastDimensionsRef.current = { cols, rows };
        sendResize(cols, rows);
      }
      // Force full redraw when transitioning from uninitialized/zero dimensions.
      if (wasZero || changed) terminal.refresh(0, terminal.rows - 1);
    },
    [xtermRef, fitAddonRef, terminalRef, lastDimensionsRef, sendResize],
  );
}
