import { useEffect } from "react";
import type { Terminal } from "@xterm/xterm";
import type { IDisposable } from "@xterm/xterm";
import {
  clearTerminalBusy,
  markTerminalInput,
  markTerminalOutput,
} from "@/lib/terminal/terminal-busy-registry";

/** Wire xterm input/output hooks so close handlers can detect running commands. */
export function useTerminalBusyTracking(
  terminalId: string | undefined,
  xtermRef: React.MutableRefObject<Terminal | null>,
  enabled: boolean,
  terminalReady: boolean,
): void {
  useEffect(() => {
    if (!enabled || !terminalId || !terminalReady) return;

    let disposed = false;
    let inputSub: IDisposable | undefined;
    let outputSub: IDisposable | undefined;
    let raf = 0;

    const attach = () => {
      if (disposed) return;
      const terminal = xtermRef.current;
      if (!terminal) {
        raf = requestAnimationFrame(attach);
        return;
      }
      inputSub = terminal.onData((data) => markTerminalInput(terminalId, data));
      outputSub = terminal.onWriteParsed(() => markTerminalOutput(terminalId, terminal));
    };

    attach();

    return () => {
      disposed = true;
      if (raf) cancelAnimationFrame(raf);
      inputSub?.dispose();
      outputSub?.dispose();
      clearTerminalBusy(terminalId);
    };
    // xtermRef object is stable; included to satisfy exhaustive-deps.
  }, [enabled, terminalId, terminalReady, xtermRef]);
}
