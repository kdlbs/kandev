import { useEffect } from "react";
import type { Terminal } from "@xterm/xterm";
import { attachTouchScroll } from "@/lib/terminal/touch-scroll";

export type TouchScrollOptions = {
  terminalRef: React.RefObject<HTMLDivElement | null>;
  xtermRef: React.MutableRefObject<Terminal | null>;
  /** When false, the hook is a no-op (desktop / non-mobile callers). */
  enabled: boolean;
  /** Gate to wait until the terminal instance has been created. */
  isTerminalReady: boolean;
};

/**
 * Wire touch-drag-to-scrollback on the terminal container while `enabled`.
 * No-op on desktop callers — touchstart/touchmove never fire from mouse input.
 */
export function useTouchScroll({
  terminalRef,
  xtermRef,
  enabled,
  isTerminalReady,
}: TouchScrollOptions) {
  useEffect(() => {
    if (!enabled || !isTerminalReady) return;
    const container = terminalRef.current;
    const terminal = xtermRef.current;
    if (!container || !terminal) return;
    return attachTouchScroll(container, terminal);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refs are stable
  }, [enabled, isTerminalReady]);
}
