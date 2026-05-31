import { useEffect } from "react";
import type { Terminal } from "@xterm/xterm";
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
): void {
  useEffect(() => {
    if (!enabled || !terminalId) return;
    const terminal = xtermRef.current;
    if (!terminal) return;

    const inputSub = terminal.onData((data) => markTerminalInput(terminalId, data));
    const outputSub = terminal.onWriteParsed(() => markTerminalOutput(terminalId, terminal));

    return () => {
      inputSub.dispose();
      outputSub.dispose();
      clearTerminalBusy(terminalId);
    };
  }, [enabled, terminalId, xtermRef]);
}
