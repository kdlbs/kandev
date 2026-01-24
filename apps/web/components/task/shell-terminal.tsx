'use client';

import { useEffect, useRef, useCallback } from 'react';
import type { Terminal as TerminalType } from 'ghostty-web';
import { loadGhostty } from '@/lib/terminal/ghostty-loader';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useSession } from '@/hooks/domains/session/use-session';
import { useSessionAgentctl } from '@/hooks/domains/session/use-session-agentctl';

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
  const xtermRef = useRef<TerminalType | null>(null);
  const fitAddonRef = useRef<{ fit: () => void; dispose: () => void } | null>(null);
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

  const { session, isActive } = useSession(isReadOnlyMode ? null : sessionId);
  // Hook also subscribes to session channel for real-time updates
  useSessionAgentctl(isReadOnlyMode ? null : sessionId);
  const taskId = session?.task_id ?? null;
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

    let cancelled = false;
    let resizeObserver: ResizeObserver | null = null;
    let intersectionObserver: IntersectionObserver | null = null;
    let initialFitTimeout: ReturnType<typeof setTimeout> | null = null;

    const initTerminal = async () => {
      const [mod, ghostty] = await Promise.all([
        import('ghostty-web'),
        loadGhostty(),
      ]);
      if (cancelled || !terminalRef.current) return;

      // Note: ghostty option is supported at runtime but not in published types
      const terminal = new mod.Terminal({
        ghostty,
        allowTransparency: true,
        cursorBlink: !isReadOnlyMode,
        disableStdin: isReadOnlyMode,
        convertEol: isReadOnlyMode,
        fontSize: isReadOnlyMode ? 12 : 13,
        fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        theme: {
          background: 'transparent',
          foreground: '#d4d4d4',
          cursor: '#d4d4d4',
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
      } as Parameters<typeof mod.Terminal>[0]);

      const fitAddon = new mod.FitAddon();
      terminal.loadAddon(fitAddon);
      await terminal.open(terminalRef.current);
      if (cancelled) {
        terminal.dispose();
        return;
      }
      fitAddon.fit();

      xtermRef.current = terminal;
      fitAddonRef.current = fitAddon;

      // For read-only mode, write initial output
      if (isReadOnlyMode && outputRef.current) {
        terminal.write(outputRef.current);
        lastOutputLengthRef.current = outputRef.current.length;
      }

      // For interactive mode, write any existing shell output that arrived before terminal was ready
      if (!isReadOnlyMode && sessionId) {
        const existingOutput = storeApi.getState().shell.outputs[sessionId] || '';
        if (existingOutput && existingOutput.length > lastOutputLengthRef.current) {
          const newData = existingOutput.slice(lastOutputLengthRef.current);
          terminal.write(newData);
          lastOutputLengthRef.current = existingOutput.length;
        }

        // Attach onData handler immediately since the effect may have already run
        if (taskId) {
          onDataDisposableRef.current?.dispose();
          onDataDisposableRef.current = terminal.onData((data) => {
            if (/^\x1b\[\d+;\d+R$/.test(data) || /^\x1b\[\d+R$/.test(data)) {
              return;
            }
            send('shell.input', { session_id: sessionId, data });
          });
        }
      }

      // Ensure terminal is properly sized after initial render
      initialFitTimeout = setTimeout(() => {
        fitAddon.fit();
      }, 100);

      // Handle resize
      resizeObserver = new ResizeObserver(() => {
        fitAddon.fit();
      });
      resizeObserver.observe(terminalRef.current);

      // Handle visibility changes (e.g., when tab becomes visible)
      intersectionObserver = new IntersectionObserver(
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

      // Reset output tracking when session changes
      lastOutputLengthRef.current = 0;
    };

    initTerminal();

    return () => {
      cancelled = true;
      if (initialFitTimeout) clearTimeout(initialFitTimeout);
      resizeObserver?.disconnect();
      intersectionObserver?.disconnect();
      xtermRef.current?.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
    };
  }, [taskId, sessionId, send, isReadOnlyMode, storeApi]);

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
  useEffect(() => {
    if (!xtermRef.current || !isReadOnlyMode) return;
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
      <div className="h-full w-full bg-background relative">
        <div ref={terminalRef} className="px-3 py-2 absolute inset-0" />
        {isStopping ? (
          <div className="absolute right-3 top-2 text-xs text-muted-foreground">
            Stopping…
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <div
      ref={terminalRef}
      className="h-full w-full overflow-hidden px-3 py-2 bg-background"
    />
  );
}
