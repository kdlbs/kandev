import { useEffect, type RefObject } from "react";
import type { Terminal } from "@xterm/xterm";
import type { ResolvedTheme } from "@/components/theme/app-theme";
import { getTerminalTheme } from "@/lib/theme/terminal-theme";

type UseTerminalThemeOptions = {
  terminalRef: RefObject<Terminal | null>;
  containerRef: RefObject<HTMLElement | null>;
  isTerminalReady: boolean;
  resolvedTheme: ResolvedTheme;
};

/** Keep an open xterm instance in sync without replacing its buffer or socket. */
export function useTerminalTheme({
  terminalRef,
  containerRef,
  isTerminalReady,
  resolvedTheme,
}: UseTerminalThemeOptions): void {
  useEffect(() => {
    if (!isTerminalReady) return;

    const terminal = terminalRef.current;
    const container = containerRef.current;
    if (!terminal || !container || !terminal.element) return;

    let cancelled = false;
    let secondFrame: number | null = null;
    const firstFrame = requestAnimationFrame(() => {
      secondFrame = requestAnimationFrame(() => {
        if (cancelled) return;
        const currentTerminal = terminalRef.current;
        const currentContainer = containerRef.current;
        if (!currentTerminal || !currentContainer || !currentTerminal.element) return;
        currentTerminal.options.theme = getTerminalTheme(currentContainer, resolvedTheme);
      });
    });

    return () => {
      cancelled = true;
      cancelAnimationFrame(firstFrame);
      if (secondFrame !== null) cancelAnimationFrame(secondFrame);
    };
  }, [containerRef, isTerminalReady, resolvedTheme, terminalRef]);
}
