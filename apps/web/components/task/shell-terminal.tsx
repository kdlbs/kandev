'use client';

import { useEffect, useRef, useCallback } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';

type ShellTerminalProps = {
  taskId: string;
  sessionId: string | null;
};

export function ShellTerminal({ taskId, sessionId }: ShellTerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const lastOutputLengthRef = useRef(0);
  const subscriptionIdRef = useRef(0);
  const storeApi = useAppStoreApi();

  const shellOutput = useAppStore((state) => state.shell.outputs[taskId] || '');

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

    // Handle user input
    terminal.onData((data) => {
      send('shell.input', { task_id: taskId, session_id: sessionId, data });
    });

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

  // Subscribe to shell on mount to start the shell stream
  // Uses retry logic to handle cases where agent isn't ready yet (e.g., session resumption)
  useEffect(() => {
    // Increment subscription ID to invalidate any pending requests
    const currentSubscriptionId = ++subscriptionIdRef.current;
    let retryCount = 0;
    const maxRetries = 10;
    const retryDelay = 1000; // 1 second between retries
    let retryTimeout: ReturnType<typeof setTimeout> | null = null;

    // Clear any stale output before subscribing to get fresh buffer
    storeApi.getState().clearShellOutput(taskId);
    lastOutputLengthRef.current = 0;

    // Also clear the terminal display
    if (xtermRef.current) {
      xtermRef.current.clear();
    }

    const attemptSubscribe = () => {
      const client = getWebSocketClient();
      if (!client) return;

      // Check if this subscription is still valid
      if (subscriptionIdRef.current !== currentSubscriptionId) return;

      client
        .request<{ success: boolean; buffer?: string }>('shell.subscribe', { task_id: taskId, session_id: sessionId })
        .then((response) => {
          // Only process if this is still the current subscription (handles React Strict Mode)
          if (subscriptionIdRef.current !== currentSubscriptionId) return;

          // Write buffered output from response
          if (response.buffer) {
            storeApi.getState().appendShellOutput(taskId, response.buffer);
          }
        })
        .catch((err) => {
          if (subscriptionIdRef.current !== currentSubscriptionId) return;

          // Retry if agent not ready yet (common during session resumption)
          if (retryCount < maxRetries) {
            retryCount++;
            console.log(`[ShellTerminal] Retrying shell subscription (${retryCount}/${maxRetries})...`);
            retryTimeout = setTimeout(attemptSubscribe, retryDelay);
          } else {
            console.error('Failed to subscribe to shell after retries:', err);
          }
        });
    };

    attemptSubscribe();

    return () => {
      if (retryTimeout) {
        clearTimeout(retryTimeout);
      }
    };
  }, [taskId, sessionId, storeApi]);

  return (
    <div
      ref={terminalRef}
      className="h-full w-full overflow-hidden rounded-md border border-border bg-[#1e1e1e]"
    />
  );
}

