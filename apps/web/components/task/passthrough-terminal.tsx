'use client';

import { useEffect, useRef, useCallback, useMemo } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { AttachAddon } from '@xterm/addon-attach';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { WebglAddon } from '@xterm/addon-webgl';
import '@xterm/xterm/css/xterm.css';
import { useAppStore } from '@/components/state-provider';
import { useSession } from '@/hooks/domains/session/use-session';
import { useSessionAgentctl } from '@/hooks/domains/session/use-session-agentctl';
import { getBackendConfig } from '@/lib/config';
import { terminalTheme, applyTransparentBackground } from '@/lib/terminal-theme';

type PassthroughTerminalProps = {
  sessionId?: string | null;
  terminalId?: string; // Terminal tab identifier (e.g., "shell-1", "shell-2")
  label?: string;      // Label for plain shell terminals (sent to backend for persistence)
};

// Debug flag - set to true to see detailed logs
const DEBUG = false;
const log = (...args: unknown[]) => {
  if (DEBUG) console.log('[PassthroughTerminal]', ...args);
};

// Minimum dimensions to prevent zero-size issues
const MIN_WIDTH = 100;
const MIN_HEIGHT = 100;

/**
 * PassthroughTerminal provides direct terminal interaction with an agent CLI.
 *
 * Design: Dedicated Binary WebSocket + AttachAddon
 * - Uses a dedicated WebSocket connection to /xterm.js/:sessionId
 * - Raw binary frames bypass JSON encoding/decoding latency
 * - AttachAddon (official xterm.js addon) handles the bridging
 * - Unicode11Addon enables proper unicode character support
 * - Resize commands sent via binary protocol: [0x01][JSON {cols, rows}]
 */
export function PassthroughTerminal({ sessionId: propSessionId, terminalId, label }: PassthroughTerminalProps) {
  // Debug: log props on every render
  log('Render - props:', { propSessionId, terminalId, label });

  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const attachAddonRef = useRef<AttachAddon | null>(null);
  const isInitializedRef = useRef(false);
  const lastDimensionsRef = useRef({ cols: 0, rows: 0 });
  const resizeTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const webglAddonRef = useRef<WebglAddon | null>(null);

  // Get sessionId from store if not provided as prop
  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = propSessionId ?? storeSessionId;

  const { session, isActive } = useSession(sessionId);
  useSessionAgentctl(sessionId);
  const taskId = session?.task_id ?? null;

  const canConnect = Boolean(sessionId && isActive);

  // Compute WebSocket base URL from backend config
  const wsBaseUrl = useMemo(() => {
    try {
      const backendUrl = getBackendConfig().apiBaseUrl;
      const url = new URL(backendUrl);
      const protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
      return `${protocol}//${url.host}`;
    } catch {
      return 'ws://localhost:8080';
    }
  }, []);

  // Send resize command via binary protocol
  const sendResize = useCallback((cols: number, rows: number) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      log('sendResize: WebSocket not ready', ws?.readyState);
      return;
    }
    if (cols <= 0 || rows <= 0) {
      log('sendResize: invalid dimensions', cols, rows);
      return;
    }

    log('sendResize:', cols, 'x', rows);

    // Binary protocol: first byte 0x01 = resize command, followed by JSON
    const json = JSON.stringify({ cols, rows });
    const encoder = new TextEncoder();
    const jsonBytes = encoder.encode(json);
    const buffer = new Uint8Array(1 + jsonBytes.length);
    buffer[0] = 0x01; // Resize command byte
    buffer.set(jsonBytes, 1);
    ws.send(buffer);
  }, []);

  // Fit terminal and send resize if dimensions changed
  const fitAndResize = useCallback((force = false) => {
    const terminal = xtermRef.current;
    const fitAddon = fitAddonRef.current;
    const container = terminalRef.current;

    if (!terminal || !fitAddon || !container) {
      log('fitAndResize: missing refs');
      return;
    }

    const rect = container.getBoundingClientRect();
    log('fitAndResize: container', rect.width, 'x', rect.height);

    // Skip if container is too small
    if (rect.width < MIN_WIDTH || rect.height < MIN_HEIGHT) {
      log('fitAndResize: container too small, skipping');
      return;
    }

    try {
      fitAddon.fit();
      log('fitAndResize: fit done', terminal.cols, 'x', terminal.rows);
    } catch (e) {
      log('fitAndResize: fit failed', e);
      return;
    }

    const { cols, rows } = terminal;
    const last = lastDimensionsRef.current;

    // Only send resize if dimensions changed or force is true
    if (force || cols !== last.cols || rows !== last.rows) {
      lastDimensionsRef.current = { cols, rows };
      sendResize(cols, rows);
    }
  }, [sendResize]);

  // Initialize terminal - runs once per mount
  useEffect(() => {
    log('Terminal init effect');

    const container = terminalRef.current;
    if (!container) {
      log('No container ref');
      return;
    }

    if (isInitializedRef.current) {
      log('Already initialized');
      return;
    }

    // Wait for container to have valid dimensions
    const initWhenReady = () => {
      const rect = container.getBoundingClientRect();
      log('Init check: dimensions', rect.width, 'x', rect.height);

      if (rect.width >= MIN_WIDTH && rect.height >= MIN_HEIGHT) {
        initializeTerminal();
        return true;
      }
      return false;
    };

    // Try immediately, then retry with RAF
    if (!initWhenReady()) {
      let retryCount = 0;
      const maxRetries = 30;

      const retry = () => {
        if (isInitializedRef.current) return;
        retryCount++;
        if (initWhenReady() || retryCount >= maxRetries) {
          if (retryCount >= maxRetries) {
            log('Max retries, forcing init');
            initializeTerminal();
          }
          return;
        }
        requestAnimationFrame(retry);
      };

      requestAnimationFrame(retry);
    }

    function initializeTerminal() {
      if (isInitializedRef.current || xtermRef.current || !container) {
        return;
      }
      isInitializedRef.current = true;

      // Capture container reference for closures
      const termContainer = container;

      log('Creating terminal');

      const terminal = new Terminal({
        allowProposedApi: true,
        cursorBlink: true,
        disableStdin: false,
        convertEol: false,
        scrollOnUserInput: true,
        scrollback: 5000,
        fontSize: 13,
        fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        theme: terminalTheme,
      });

      const fitAddon = new FitAddon();
      terminal.loadAddon(fitAddon);

      // Load Unicode11 addon
      const unicode11Addon = new Unicode11Addon();
      terminal.loadAddon(unicode11Addon);
      terminal.unicode.activeVersion = '11';

      log('Opening terminal in container');
      terminal.open(termContainer);

      // Initial fit
      try {
        fitAddon.fit();
        lastDimensionsRef.current = { cols: terminal.cols, rows: terminal.rows };
        log('Initial fit:', terminal.cols, 'x', terminal.rows);
      } catch (e) {
        log('Initial fit failed:', e);
      }

      // Load WebGL addon
      try {
        const webglAddon = new WebglAddon();
        webglAddon.onContextLoss(() => {
          log('WebGL context lost');
          webglAddon.dispose();
          webglAddonRef.current = null;
        });
        terminal.loadAddon(webglAddon);
        webglAddonRef.current = webglAddon;
        log('WebGL addon loaded');
      } catch (e) {
        log('WebGL failed, using canvas:', e);
      }

      xtermRef.current = terminal;
      fitAddonRef.current = fitAddon;

      // Apply transparent background styling
      const applyStyles = () => applyTransparentBackground(termContainer);
      requestAnimationFrame(applyStyles);

      // Debounced resize handler
      const handleResize = () => {
        const rect = termContainer.getBoundingClientRect();
        log('ResizeObserver:', rect.width, 'x', rect.height);

        // Skip zero/tiny dimensions
        if (rect.width < MIN_WIDTH || rect.height < MIN_HEIGHT) {
          log('Skipping resize - too small');
          return;
        }

        // Clear pending resize
        if (resizeTimeoutRef.current) {
          clearTimeout(resizeTimeoutRef.current);
        }

        // Debounce the actual fit/resize
        resizeTimeoutRef.current = setTimeout(() => {
          fitAndResize();
          // Re-apply styles after resize
          applyStyles();
        }, 100);
      };

      const resizeObserver = new ResizeObserver(handleResize);
      resizeObserver.observe(termContainer);

      // Store cleanup
      return () => {
        log('Terminal cleanup');
        if (resizeTimeoutRef.current) {
          clearTimeout(resizeTimeoutRef.current);
        }
        resizeObserver.disconnect();
        if (webglAddonRef.current) {
          webglAddonRef.current.dispose();
          webglAddonRef.current = null;
        }
        terminal.dispose();
        xtermRef.current = null;
        fitAddonRef.current = null;
        isInitializedRef.current = false;
        lastDimensionsRef.current = { cols: 0, rows: 0 };
      };
    }

    return () => {
      log('Effect cleanup');
      if (resizeTimeoutRef.current) {
        clearTimeout(resizeTimeoutRef.current);
      }
      if (webglAddonRef.current) {
        webglAddonRef.current.dispose();
        webglAddonRef.current = null;
      }
      if (xtermRef.current) {
        xtermRef.current.dispose();
        xtermRef.current = null;
      }
      fitAddonRef.current = null;
      isInitializedRef.current = false;
      lastDimensionsRef.current = { cols: 0, rows: 0 };
    };
  }, [fitAndResize]);

  // Connect WebSocket when ready
  useEffect(() => {
    log('WebSocket effect:', { taskId, sessionId, terminalId, canConnect, hasTerminal: !!xtermRef.current });

    if (!taskId || !sessionId || !canConnect) {
      log('WebSocket effect: early return - missing requirements', { taskId, sessionId, canConnect });
      return;
    }
    if (!xtermRef.current || !fitAddonRef.current) {
      log('Terminal not ready for WebSocket');
      return;
    }

    const terminal = xtermRef.current;
    let isMounted = true;
    let connectTimeout: ReturnType<typeof setTimeout> | null = null;
    let refreshInterval: ReturnType<typeof setInterval> | null = null;

    // Delay connection to handle React Strict Mode
    connectTimeout = setTimeout(() => {
      if (!isMounted) return;

      // Close existing connection
      if (attachAddonRef.current) {
        attachAddonRef.current.dispose();
        attachAddonRef.current = null;
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }

      // Build WebSocket URL - terminalId is required for user shells
      // If terminalId is not provided, this is an error for user shell terminals
      if (!terminalId) {
        log('ERROR: terminalId is required but was not provided!', { sessionId, terminalId });
        return;
      }
      let wsUrl = `${wsBaseUrl}/xterm.js/${sessionId}?terminalId=${encodeURIComponent(terminalId)}`;
      if (label) {
        // For plain shell terminals, send the label for persistence
        wsUrl += `&label=${encodeURIComponent(label)}`;
      }
      log('Connecting to', wsUrl, { terminalId, label });

      const ws = new WebSocket(wsUrl);
      ws.binaryType = 'arraybuffer';
      wsRef.current = ws;

      ws.onopen = () => {
        if (!isMounted) {
          ws.close();
          return;
        }
        log('WebSocket connected');

        // Attach to terminal
        const attachAddon = new AttachAddon(ws, { bidirectional: true });
        terminal.loadAddon(attachAddon);
        attachAddonRef.current = attachAddon;

        // Send initial resize to trigger PTY and get content
        // Send multiple times with delay to ensure PTY redraws
        const triggerRefresh = () => {
          if (isMounted && ws.readyState === WebSocket.OPEN) {
            fitAndResize(true);
          }
        };

        // Immediate refresh
        requestAnimationFrame(triggerRefresh);

        // Follow-up refreshes to ensure content appears
        setTimeout(triggerRefresh, 100);
        setTimeout(triggerRefresh, 300);
        setTimeout(triggerRefresh, 500);

        // Also set up periodic refresh check for the first few seconds
        let refreshCount = 0;
        refreshInterval = setInterval(() => {
          refreshCount++;
          if (refreshCount > 5 || !isMounted) {
            if (refreshInterval) clearInterval(refreshInterval);
            return;
          }
          triggerRefresh();
        }, 1000);
      };

      ws.onclose = (event) => {
        log('WebSocket closed:', event.code, event.reason);
        if (attachAddonRef.current) {
          attachAddonRef.current.dispose();
          attachAddonRef.current = null;
        }
      };

      ws.onerror = (error) => {
        log('WebSocket error:', error);
      };
    }, 150);

    return () => {
      log('WebSocket cleanup');
      isMounted = false;
      if (connectTimeout) clearTimeout(connectTimeout);
      if (refreshInterval) clearInterval(refreshInterval);
      if (attachAddonRef.current) {
        attachAddonRef.current.dispose();
        attachAddonRef.current = null;
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [taskId, sessionId, canConnect, fitAndResize, wsBaseUrl, terminalId, label]);

  return (
    <div
      ref={terminalRef}
      className="h-full w-full overflow-hidden rounded-md bg-background p-1"
      style={{ minWidth: MIN_WIDTH, minHeight: MIN_HEIGHT }}
    />
  );
}
