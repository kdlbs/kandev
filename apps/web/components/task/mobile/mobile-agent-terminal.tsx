"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { Terminal } from "@xterm/xterm";
import { useAppStore } from "@/components/state-provider";
import { sendPtyInput } from "@/lib/terminal/send-pty-input";
import { useShellModifiersStore } from "@/lib/terminal/shell-modifiers";
import { MobileTerminalKeybar } from "./mobile-terminal-keybar";
import { PassthroughTerminal } from "../passthrough-terminal";

type Props = { sessionId: string | null | undefined; showControls: boolean; touch: boolean };

function MobileAgentTerminal({ sessionId, showControls, touch }: Props) {
  const quickChatOpen = useAppStore((state) => state.quickChat.isOpen);
  const [terminal, setTerminal] = useState<Terminal | null>(null);
  const [socket, setSocket] = useState<WebSocket | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const controlsActive = Boolean(
    showControls &&
    !quickChatOpen &&
    sessionId &&
    terminal &&
    socket &&
    socket.readyState === WebSocket.OPEN,
  );
  const controlsActiveRef = useRef(controlsActive);
  useLayoutEffect(() => {
    controlsActiveRef.current = controlsActive;
  }, [controlsActive]);

  const onSocketReady = useCallback((next: WebSocket) => {
    socketRef.current = next;
    setSocket(next);
  }, []);
  const sendInput = useCallback((data: string) => {
    sendPtyInput(socketRef.current, data, controlsActiveRef.current);
  }, []);
  const focusTerminal = useCallback(() => terminal?.focus(), [terminal]);

  useEffect(() => {
    if (!socket) return;
    socketRef.current = socket;
    const onClose = () => {
      if (socketRef.current !== socket) return;
      socketRef.current = null;
      setSocket((current) => (current === socket ? null : current));
    };
    socket.addEventListener("close", onClose);
    if (socket.readyState !== WebSocket.OPEN) onClose();
    return () => {
      socket.removeEventListener("close", onClose);
      if (socketRef.current === socket) socketRef.current = null;
    };
  }, [socket]);

  useEffect(() => {
    if (!terminal) return;
    const subscription = terminal.onData(sendInput);
    return () => subscription.dispose();
  }, [terminal, sendInput]);

  useEffect(() => {
    if (!controlsActive) return;
    const reset = useShellModifiersStore.getState().reset;
    reset();
    return reset;
  }, [controlsActive, socket]);

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col">
      <div className="flex-1 min-h-0">
        <PassthroughTerminal
          mode="agent"
          sessionId={sessionId}
          disableWebgl
          manualInputRouting
          onXtermReady={setTerminal}
          onWsReady={onSocketReady}
          enableTouchScroll={touch}
        />
      </div>
      <MobileTerminalKeybar
        sessionId={sessionId}
        visible={controlsActive}
        inline
        onSend={sendInput}
        onFocus={focusTerminal}
      />
    </div>
  );
}

export function AgentTerminalInput({
  sessionId,
  mobile,
  touch,
  showControls = true,
}: {
  sessionId: string | null | undefined;
  mobile: boolean;
  touch: boolean;
  showControls?: boolean;
}) {
  if (mobile) {
    return (
      <MobileAgentTerminal
        key={sessionId}
        sessionId={sessionId}
        showControls={showControls}
        touch={touch}
      />
    );
  }
  return <PassthroughTerminal sessionId={sessionId} mode="agent" enableTouchScroll={touch} />;
}
