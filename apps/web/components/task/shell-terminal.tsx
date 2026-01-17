'use client';

import { useEffect, useRef, useCallback } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useSession } from '@/hooks/use-session';
import { useSessionAgentctl } from '@/hooks/use-session-agentctl';

export function ShellTerminal() {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const lastOutputLengthRef = useRef(0);
  const subscriptionIdRef = useRef(0);
  const onDataDisposableRef = useRef<{ dispose: () => void } | null>(null);
  const storeApi = useAppStoreApi();

  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { session, isActive } = useSession(sessionId);
  const { isReady: isAgentctlReady } = useSessionAgentctl(sessionId);
  const taskId = session?.task_id ?? null;
  const shellOutput = useAppStore((state) =>
    sessionId ? state.shell.outputs[sessionId] || '' : ''
  );
  const canSubscribe = Boolean(sessionId && isActive && isAgentctlReady);

  const send = useCallback(
    (action: string, payload: Record<string, unknown>) => {
      const client = getWebSocketClient();
      if (client) {
        client.send({ type: 'request', action, payload });
      }
    },
    []
  );

  // Initialize terminal
  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return;

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        cursorAccent: '#1e1e1e',
        selectionBackground: '#264f78',
        black: '#1e1e1e',
        red: '#f44747',
        green: '#6a9955',
        yellow: '#dcdcaa',
        blue: '#569cd6',
        magenta: '#c586c0',
        cyan: '#4ec9b0',
        white: '#d4d4d4',
        brightBlack: '#808080',
        brightRed: '#f44747',
        brightGreen: '#6a9955',
        brightYellow: '#dcdcaa',
        brightBlue: '#569cd6',
        brightMagenta: '#c586c0',
        brightCyan: '#4ec9b0',
        brightWhite: '#ffffff',
      },
    });

    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = terminal;
    fitAddonRef.current = fitAddon;

    // Handle resize
    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
    });
    resizeObserver.observe(terminalRef.current);

    return () => {
      resizeObserver.disconnect();
      terminal.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
    };
  }, [taskId, sessionId, send]);

  useEffect(() => {
    if (!xtermRef.current) return;
    onDataDisposableRef.current?.dispose();
    onDataDisposableRef.current = null;

    if (!taskId || !sessionId) return;

    onDataDisposableRef.current = xtermRef.current.onData((data) => {
      send('shell.input', { session_id: sessionId, data });
    });

    return () => {
      onDataDisposableRef.current?.dispose();
      onDataDisposableRef.current = null;
    };
  }, [taskId, sessionId, send]);

  // Write new output to terminal
  useEffect(() => {
    if (!xtermRef.current) return;

    // Only write new data since last update
    const newData = shellOutput.slice(lastOutputLengthRef.current);
    if (newData) {
      xtermRef.current.write(newData);
      lastOutputLengthRef.current = shellOutput.length;
    }
  }, [shellOutput]);

  // Subscribe to shell once agentctl is ready.
  useEffect(() => {
    if (!taskId || !sessionId || !canSubscribe) return;

    const currentSubscriptionId = ++subscriptionIdRef.current;

    // Clear any stale output before subscribing to get fresh buffer
    storeApi.getState().clearShellOutput(sessionId);
    lastOutputLengthRef.current = 0;

    // Also clear the terminal display
    if (xtermRef.current) {
      xtermRef.current.clear();
    }

    const client = getWebSocketClient();
    if (!client) return;

    client
      .request<{ success: boolean; buffer?: string }>('shell.subscribe', { session_id: sessionId })
      .then((response) => {
        if (subscriptionIdRef.current !== currentSubscriptionId) return;
        if (response.buffer) {
          storeApi.getState().appendShellOutput(sessionId, response.buffer);
        }
      })
      .catch((err) => {
        if (subscriptionIdRef.current !== currentSubscriptionId) return;
        console.error('Failed to subscribe to shell:', err);
      });

    return () => {
      subscriptionIdRef.current += 1;
    };
  }, [taskId, sessionId, storeApi, canSubscribe]);

  return (
    <div
      ref={terminalRef}
      className="h-full w-full overflow-hidden rounded-md border border-border bg-[#1e1e1e]"
    />
  );
}
