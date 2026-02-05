'use client';

import { useEffect, useRef, useCallback } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useSession } from '@/hooks/domains/session/use-session';
import { useSessionAgentctl } from '@/hooks/domains/session/use-session-agentctl';
import { terminalTheme, applyTransparentBackground } from '@/lib/terminal-theme';

type ShellTerminalProps = {
  // Interactive shell mode - requires sessionId
  sessionId?: string;
  // Read-only process output mode - requires processOutput
  processOutput?: string;
  processId?: string | null;
  isStopping?: boolean;
};

export function ShellTerminal({
  sessionId: propSessionId,
  processOutput,
  processId,
  isStopping = false,
}: ShellTerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const lastOutputLengthRef = useRef(0);
  const subscriptionIdRef = useRef(0);
  const onDataDisposableRef = useRef<{ dispose: () => void } | null>(null);
  const retryTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const processIdRef = useRef<string | null>(null);
  const outputRef = useRef(processOutput ?? '');
  const storeApi = useAppStoreApi();

  // Determine mode: if processOutput is provided, use read-only mode
  const isReadOnlyMode = processOutput !== undefined;

  // For interactive mode, get sessionId from store if not provided as prop
  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = propSessionId ?? storeSessionId;

  const { session, isActive, isFailed, errorMessage } = useSession(isReadOnlyMode ? null : sessionId);
  // Hook also subscribes to session channel for real-time updates
  useSessionAgentctl(isReadOnlyMode ? null : sessionId);
  const taskId = session?.task_id ?? null;
  const isSessionFailed = !isReadOnlyMode && isFailed;
  const shellOutput = useAppStore((state) =>
    sessionId && !isReadOnlyMode ? state.shell.outputs[sessionId] || '' : ''
  );
  // Don't gate on isAgentctlReady - shell works as long as session is active
  const canSubscribe = Boolean(sessionId && isActive && !isReadOnlyMode);

  // Update output ref when processOutput changes
  useEffect(() => {
    if (isReadOnlyMode) {
      outputRef.current = processOutput ?? '';
    }
  }, [processOutput, isReadOnlyMode]);

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
      cursorBlink: !isReadOnlyMode,
      disableStdin: isReadOnlyMode,
      convertEol: isReadOnlyMode,
      fontSize: isReadOnlyMode ? 12 : 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: terminalTheme,
    });

    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = terminal;
    fitAddonRef.current = fitAddon;

    // Force transparent background on all xterm elements to inherit from parent
    applyTransparentBackground(terminalRef.current);

    // For read-only mode, write initial output
    if (isReadOnlyMode && outputRef.current) {
      terminal.write(outputRef.current);
      lastOutputLengthRef.current = outputRef.current.length;
    }

    // Ensure terminal is properly sized after initial render
    const initialFitTimeout = setTimeout(() => {
      fitAddon.fit();
    }, 100);

    // Handle resize
    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
    });
    resizeObserver.observe(terminalRef.current);

    // Handle visibility changes (e.g., when tab becomes visible)
    const intersectionObserver = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting && fitAddonRef.current) {
            requestAnimationFrame(() => {
              fitAddonRef.current?.fit();
            });
          }
        });
      },
      { threshold: 0.1 }
    );
    intersectionObserver.observe(terminalRef.current);

    // Reset output tracking when session changes (interactive mode only)
    // For read-only mode, we preserve the length set by the initial write above
    if (!isReadOnlyMode) {
      lastOutputLengthRef.current = 0;
    }

    return () => {
      clearTimeout(initialFitTimeout);
      resizeObserver.disconnect();
      intersectionObserver.disconnect();
      terminal.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
    };
  }, [taskId, sessionId, send, isReadOnlyMode]);

  // Handle user input (interactive mode only)
  useEffect(() => {
    if (!xtermRef.current || isReadOnlyMode) return;
    onDataDisposableRef.current?.dispose();
    onDataDisposableRef.current = null;

    if (!taskId || !sessionId) return;

    onDataDisposableRef.current = xtermRef.current.onData((data) => {
      // Filter out terminal query responses (e.g., cursor position reports)
      // These are responses to DSR queries like \x1b[6n and look like \x1b[row;colR
      // They should NOT be sent to the shell as input
      if (/^\x1b\[\d+;\d+R$/.test(data) || /^\x1b\[\d+R$/.test(data)) {
        return;
      }
      send('shell.input', { session_id: sessionId, data });
    });

    return () => {
      onDataDisposableRef.current?.dispose();
      onDataDisposableRef.current = null;
    };
  }, [taskId, sessionId, send, isReadOnlyMode]);

  // Handle processId changes in read-only mode (clear terminal and reset output)
  // Skip on initial mount (processIdRef.current === null) since initialization effect handles it
  useEffect(() => {
    if (!xtermRef.current || !isReadOnlyMode) return;
    if (processIdRef.current === null) {
      // Initial mount - just track the processId, don't write (initialization effect handles it)
      processIdRef.current = processId ?? null;
      return;
    }
    if (processIdRef.current !== processId) {
      processIdRef.current = processId ?? null;
      lastOutputLengthRef.current = 0;
      xtermRef.current.clear();
      if (outputRef.current) {
        xtermRef.current.write(outputRef.current);
        lastOutputLengthRef.current = outputRef.current.length;
      }
    }
  }, [processId, isReadOnlyMode]);

  // Write new output to terminal
  useEffect(() => {
    if (!xtermRef.current) return;

    const output = isReadOnlyMode ? (processOutput ?? '') : shellOutput;

    // Only write new data since last update
    const newData = output.slice(lastOutputLengthRef.current);
    if (newData) {
      xtermRef.current.write(newData);
      lastOutputLengthRef.current = output.length;
    }
  }, [shellOutput, processOutput, isReadOnlyMode]);

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

    if (retryTimeoutRef.current) {
      clearTimeout(retryTimeoutRef.current);
      retryTimeoutRef.current = null;
    }

    let cancelled = false;

    const attemptSubscribe = () => {
      client
        .request<{ success: boolean; buffer?: string }>('shell.subscribe', { session_id: sessionId })
        .then((response) => {
          if (cancelled || subscriptionIdRef.current !== currentSubscriptionId) return;
          if (response.buffer) {
            storeApi.getState().appendShellOutput(sessionId, response.buffer);
          }
          // Send Ctrl+L to clear screen and redraw prompt
          // This ensures a clean state after loading the buffer which may contain garbage escape sequences
          setTimeout(() => {
            if (!cancelled && subscriptionIdRef.current === currentSubscriptionId) {
              send('shell.input', { session_id: sessionId, data: '\x0c' });
            }
          }, 100);
        })
        .catch((err) => {
          if (cancelled || subscriptionIdRef.current !== currentSubscriptionId) return;
          const message = err instanceof Error ? err.message : String(err);
          if (message.includes('no agent running')) {
            retryTimeoutRef.current = setTimeout(() => {
              if (!cancelled && subscriptionIdRef.current === currentSubscriptionId) {
                attemptSubscribe();
              }
            }, 1000);
            return;
          }
          console.error('Failed to subscribe to shell:', err);
        });
    };

    attemptSubscribe();

    return () => {
      subscriptionIdRef.current += 1;
      cancelled = true;
      if (retryTimeoutRef.current) {
        clearTimeout(retryTimeoutRef.current);
        retryTimeoutRef.current = null;
      }
    };
  }, [taskId, sessionId, storeApi, canSubscribe, send]);

  if (isReadOnlyMode) {
    return (
      <div className="h-full w-full rounded-md bg-background relative">
        <div ref={terminalRef} className="p-1 absolute inset-0" />
        {isStopping ? (
          <div className="absolute right-3 top-2 text-xs text-muted-foreground">
            Stopping…
          </div>
        ) : null}
      </div>
    );
  }

  if (isSessionFailed) {
    return (
      <div className="h-full p-4 w-full rounded-md bg-background flex flex-col gap-2">
        <div className="text-sm text-destructive/80">Session failed</div>
        {errorMessage && (
          <div className="text-xs text-muted-foreground">{errorMessage}</div>
        )}
      </div>
    );
  }

  return (
    <div
      ref={terminalRef}
      className="h-full p-1 w-full overflow-hidden rounded-md bg-background"
    />
  );
}
