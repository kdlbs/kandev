"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { PassthroughTerminal } from "../passthrough-terminal";
import { setActiveTerminalSender } from "@/lib/terminal/mobile-active-terminal";
import { sendShellInput } from "@/lib/terminal/send-shell-input";
import { MobileTerminalsPicker } from "./mobile-terminals-section";
import { MobileTerminalsProvider, useMobileTerminalsContext } from "./mobile-terminals-context";
import type { Terminal as XtermTerminal } from "@xterm/xterm";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

function TerminalSlot({
  terminal,
  sessionId,
  environmentId,
  isActive,
}: {
  terminal: Terminal;
  sessionId: string;
  environmentId: string | null;
  isActive: boolean;
}) {
  // Track xterm + ws in state so the registration / data-routing effects
  // re-run once each is ready (PassthroughTerminal initialises both
  // asynchronously when the container starts at 0×0). A ref-based dep would
  // silently miss those paths on first mount.
  const [xterm, setXterm] = useState<XtermTerminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  const handleWsReady = useCallback((ws: WebSocket) => {
    wsRef.current = ws;
  }, []);

  // Forward OS-keyboard input through sendShellInput so the key-bar's sticky
  // Ctrl/Shift modifiers are applied before the bytes hit the wire. Without
  // this, AttachAddon would auto-send raw onData and modifiers would be a
  // no-op for OS-keyboard typing on mobile.
  useEffect(() => {
    if (!isActive || !xterm) return;
    const disposable = xterm.onData((data) => sendShellInput(sessionId, data));
    return () => disposable.dispose();
  }, [isActive, xterm, sessionId]);

  // Register the active key-bar sender. It writes raw bytes directly to this
  // terminal's dedicated WS so key-bar taps reach the right shell. The
  // registry write happens via setActiveTerminalSender; sendShellInput reads
  // it and applies modifiers before forwarding.
  useEffect(() => {
    if (!isActive) return;
    const sender = (data: string) => {
      const ws = wsRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    };
    setActiveTerminalSender(sender);
    return () => setActiveTerminalSender(null);
  }, [isActive, terminal.id]);

  return (
    <div className={`absolute inset-0 ${isActive ? "block" : "hidden"}`}>
      <PassthroughTerminal
        mode="shell"
        environmentId={environmentId}
        terminalId={terminal.id}
        label={terminal.label}
        autoFocus={isActive}
        disableWebgl
        manualInputRouting
        onXtermReady={setXterm}
        onWsReady={handleWsReady}
      />
    </div>
  );
}

function MobileTerminalPaneInner({ sessionId }: { sessionId: string | null }) {
  const { terminals, terminalTabValue, environmentId } = useMobileTerminalsContext();
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
            sessionId={sessionId}
            environmentId={environmentId}
            isActive={t.id === activeId}
          />
        ))}
      </div>
    </div>
  );
}

export const MobileTerminalPane = memo(function MobileTerminalPane({
  sessionId,
}: {
  sessionId: string | null;
}) {
  return (
    <MobileTerminalsProvider sessionId={sessionId}>
      <MobileTerminalPaneInner sessionId={sessionId} />
    </MobileTerminalsProvider>
  );
});
