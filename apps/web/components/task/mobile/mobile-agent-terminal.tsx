"use client";

import { useEffect, useState } from "react";
import type { Terminal } from "@xterm/xterm";
import {
  getActiveTerminalSender,
  setActiveTerminalSender,
} from "@/lib/terminal/mobile-active-terminal";
import { sendActiveTerminalInput } from "@/lib/terminal/send-shell-input";
import { PassthroughTerminal } from "../passthrough-terminal";

export function AgentTerminalInput({
  sessionId,
  mobile,
  touch,
}: {
  sessionId: string | null | undefined;
  mobile: boolean;
  touch: boolean;
}) {
  const [xterm, setXterm] = useState<Terminal | null>(null);
  const [ws, setWs] = useState<WebSocket | null>(null);

  useEffect(() => {
    if (!mobile || !sessionId || !xterm || !ws || ws.readyState !== WebSocket.OPEN) return;
    const sender = (data: string) => {
      if (ws.readyState !== WebSocket.OPEN) return false;
      ws.send(new TextEncoder().encode(data));
      return true;
    };
    const release = setActiveTerminalSender(sender, sessionId);
    const input = xterm.onData((data) => {
      if (getActiveTerminalSender(sessionId) === sender) sendActiveTerminalInput(sessionId, data);
    });
    ws.addEventListener("close", release);
    return () => {
      ws.removeEventListener("close", release);
      input.dispose();
      release();
    };
  }, [mobile, sessionId, xterm, ws]);

  return (
    <PassthroughTerminal
      sessionId={sessionId}
      mode="agent"
      enableTouchScroll={touch}
      disableWebgl={mobile}
      manualInputRouting={mobile}
      onXtermReady={mobile ? setXterm : undefined}
      onWsReady={mobile ? setWs : undefined}
    />
  );
}
