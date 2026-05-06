"use client";

import { memo, useEffect, useState } from "react";
import { PassthroughTerminal } from "../passthrough-terminal";
import { useMobileTerminals } from "@/hooks/domains/session/use-mobile-terminals";
import { setActiveTerminalSender } from "@/lib/terminal/mobile-active-terminal";
import { MobileTerminalsPicker } from "./mobile-terminals-section";
import type { Terminal as XtermTerminal } from "@xterm/xterm";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

function TerminalSlot({
  terminal,
  environmentId,
  isActive,
}: {
  terminal: Terminal;
  environmentId: string | null;
  isActive: boolean;
}) {
  // Track xterm in state so the registration effect re-runs when the instance
  // becomes available. PassthroughTerminal initialises xterm asynchronously
  // when the container starts at 0×0 (ResizeObserver fallback), so a ref-based
  // dependency would silently miss that path on first mount.
  const [xterm, setXterm] = useState<XtermTerminal | null>(null);

  // Register the active terminal sender so the mobile key-bar can target this
  // xterm via paste(), which routes through xterm.onData → AttachAddon → WS.
  useEffect(() => {
    if (!isActive || !xterm) return;
    const sender = (data: string) => xterm.paste(data);
    setActiveTerminalSender(sender);
    return () => setActiveTerminalSender(null);
  }, [isActive, xterm]);

  return (
    <div className={`absolute inset-0 ${isActive ? "block" : "hidden"}`}>
      <PassthroughTerminal
        mode="shell"
        environmentId={environmentId}
        terminalId={terminal.id}
        label={terminal.label}
        autoFocus={isActive}
        disableWebgl
        onXtermReady={setXterm}
      />
    </div>
  );
}

export const MobileTerminalPane = memo(function MobileTerminalPane({
  sessionId,
}: {
  sessionId: string | null;
}) {
  const { terminals, terminalTabValue, environmentId } = useMobileTerminals(sessionId);
  const activeId = terminals.find((t) => t.id === terminalTabValue)?.id ?? terminals[0]?.id;

  if (!sessionId || !environmentId) {
    return (
      <div className="flex-1 flex items-center justify-center text-xs text-muted-foreground">
        Terminal unavailable — no active session.
      </div>
    );
  }

  return (
    <div className="flex-1 min-h-0 flex flex-col">
      <div className="flex items-center px-1 py-2 border-b border-border">
        <MobileTerminalsPicker sessionId={sessionId} fullWidth />
      </div>
      <div className="relative flex-1 min-h-0">
        {terminals.length === 0 && (
          <div className="absolute inset-0 flex items-center justify-center text-xs text-muted-foreground">
            Starting terminal…
          </div>
        )}
        {terminals.map((t) => (
          <TerminalSlot
            key={t.id}
            terminal={t}
            environmentId={environmentId}
            isActive={t.id === activeId}
          />
        ))}
      </div>
    </div>
  );
});
