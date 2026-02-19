"use client";

import { useEffect, useRef, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useSession } from "@/hooks/domains/session/use-session";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { getTerminalTheme } from "@/lib/theme/terminal-theme";

type ShellTerminalProps = {
  sessionId?: string;
  processOutput?: string;
  processId?: string | null;
  isStopping?: boolean;
};

type TerminalRefs = {
  terminalRef: React.RefObject<HTMLDivElement | null>;
  xtermRef: React.RefObject<Terminal | null>;
  fitAddonRef: React.RefObject<FitAddon | null>;
  lastOutputLengthRef: React.RefObject<number>;
  outputRef: React.RefObject<string>;
};

function useTerminalInit(
  refs: TerminalRefs,
  isReadOnlyMode: boolean,
  taskId: string | null,
  sessionId: string | null | undefined,
) {
  const { terminalRef, xtermRef, fitAddonRef, lastOutputLengthRef, outputRef } = refs;

  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return;
    const terminal = new Terminal({
      cursorBlink: !isReadOnlyMode,
      disableStdin: isReadOnlyMode,
      convertEol: isReadOnlyMode,
      fontSize: isReadOnlyMode ? 12 : 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: getTerminalTheme(terminalRef.current),
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(terminalRef.current);
    fitAddon.fit();
    xtermRef.current = terminal;
    fitAddonRef.current = fitAddon;

    if (isReadOnlyMode && outputRef.current) {
      terminal.write(outputRef.current);
      lastOutputLengthRef.current = outputRef.current.length;
    }

    const initialFitTimeout = setTimeout(() => {
      fitAddon.fit();
    }, 100);
    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
    });
    resizeObserver.observe(terminalRef.current);
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
      { threshold: 0.1 },
    );
    intersectionObserver.observe(terminalRef.current);
    if (!isReadOnlyMode) lastOutputLengthRef.current = 0;

    return () => {
      clearTimeout(initialFitTimeout);
      resizeObserver.disconnect();
      intersectionObserver.disconnect();
      terminal.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
    };
  }, [
    taskId,
    sessionId,
    isReadOnlyMode,
    terminalRef,
    xtermRef,
    fitAddonRef,
    lastOutputLengthRef,
    outputRef,
  ]);
}

type ShellSubscriptionOptions = {
  refs: Pick<TerminalRefs, "xtermRef" | "lastOutputLengthRef">;
  taskId: string | null;
  sessionId: string | null | undefined;
  canSubscribe: boolean;
  send: (action: string, payload: Record<string, unknown>) => void;
  storeApi: ReturnType<typeof useAppStoreApi>;
};

function useShellSubscription({
  refs,
  taskId,
  sessionId,
  canSubscribe,
  send,
  storeApi,
}: ShellSubscriptionOptions) {
  const { xtermRef, lastOutputLengthRef } = refs;
  const subscriptionIdRef = useRef(0);
  const retryTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!taskId || !sessionId || !canSubscribe) return;
    const currentSubscriptionId = ++subscriptionIdRef.current;
    storeApi.getState().clearShellOutput(sessionId);
    lastOutputLengthRef.current = 0;
    if (xtermRef.current) xtermRef.current.clear();

    const client = getWebSocketClient();
    if (!client) return;
    if (retryTimeoutRef.current) {
      clearTimeout(retryTimeoutRef.current);
      retryTimeoutRef.current = null;
    }

    let cancelled = false;
    const attemptSubscribe = () => {
      client
        .request<{ success: boolean; buffer?: string }>("shell.subscribe", {
          session_id: sessionId,
        })
        .then((response) => {
          if (cancelled || subscriptionIdRef.current !== currentSubscriptionId) return;
          if (response.buffer) storeApi.getState().appendShellOutput(sessionId, response.buffer);
          setTimeout(() => {
            if (!cancelled && subscriptionIdRef.current === currentSubscriptionId) {
              send("shell.input", { session_id: sessionId, data: "\x0c" });
            }
          }, 100);
        })
        .catch((err) => {
          if (cancelled || subscriptionIdRef.current !== currentSubscriptionId) return;
          const message = err instanceof Error ? err.message : String(err);
          if (message.includes("no agent running")) {
            retryTimeoutRef.current = setTimeout(() => {
              if (!cancelled && subscriptionIdRef.current === currentSubscriptionId)
                attemptSubscribe();
            }, 1000);
            return;
          }
          console.error("Failed to subscribe to shell:", err);
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
  }, [taskId, sessionId, storeApi, canSubscribe, send, xtermRef, lastOutputLengthRef]);
}

type ReadOnlySyncParams = {
  xtermRef: React.RefObject<Terminal | null>;
  isReadOnlyMode: boolean;
  processOutput: string | undefined;
  outputRef: React.RefObject<string>;
  processId: string | null | undefined;
  processIdRef: React.RefObject<string | null>;
  lastOutputLengthRef: React.RefObject<number>;
};

function useReadOnlyOutputSync({
  xtermRef,
  isReadOnlyMode,
  processOutput,
  outputRef,
  processId,
  processIdRef,
  lastOutputLengthRef,
}: ReadOnlySyncParams) {
  useEffect(() => {
    if (isReadOnlyMode) outputRef.current = processOutput ?? "";
  }, [processOutput, isReadOnlyMode, outputRef]);

  useEffect(() => {
    if (!xtermRef.current || !isReadOnlyMode) return;
    if (processIdRef.current === null) {
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
  }, [processId, isReadOnlyMode, xtermRef, processIdRef, outputRef, lastOutputLengthRef]);
}

function useTerminalOutputWrite(
  xtermRef: React.RefObject<Terminal | null>,
  isReadOnlyMode: boolean,
  processOutput: string | undefined,
  shellOutput: string,
  lastOutputLengthRef: React.RefObject<number>,
) {
  useEffect(() => {
    if (!xtermRef.current) return;
    const output = isReadOnlyMode ? (processOutput ?? "") : shellOutput;
    const newData = output.slice(lastOutputLengthRef.current);
    if (newData) {
      xtermRef.current.write(newData);
      lastOutputLengthRef.current = output.length;
    }
  }, [xtermRef, shellOutput, processOutput, isReadOnlyMode, lastOutputLengthRef]);
}

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
  const onDataDisposableRef = useRef<{ dispose: () => void } | null>(null);
  const processIdRef = useRef<string | null>(null);
  const outputRef = useRef(processOutput ?? "");
  const storeApi = useAppStoreApi();

  const isReadOnlyMode = processOutput !== undefined;
  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = propSessionId ?? storeSessionId;
  const { session, isActive, isFailed, errorMessage } = useSession(
    isReadOnlyMode ? null : sessionId,
  );
  useSessionAgentctl(isReadOnlyMode ? null : sessionId);
  const taskId = session?.task_id ?? null;
  const isSessionFailed = !isReadOnlyMode && isFailed;
  const shellOutput = useAppStore((state) =>
    sessionId && !isReadOnlyMode ? state.shell.outputs[sessionId] || "" : "",
  );
  const canSubscribe = Boolean(sessionId && isActive && !isReadOnlyMode);
  useReadOnlyOutputSync({
    xtermRef,
    isReadOnlyMode,
    processOutput,
    outputRef,
    processId,
    processIdRef,
    lastOutputLengthRef,
  });

  const send = useCallback((action: string, payload: Record<string, unknown>) => {
    const client = getWebSocketClient();
    if (client) client.send({ type: "request", action, payload });
  }, []);

  const refs: TerminalRefs = { terminalRef, xtermRef, fitAddonRef, lastOutputLengthRef, outputRef };
  useTerminalInit(refs, isReadOnlyMode, taskId, sessionId);

  // Handle user input (interactive mode only)
  useEffect(() => {
    if (!xtermRef.current || isReadOnlyMode) return;
    onDataDisposableRef.current?.dispose();
    onDataDisposableRef.current = null;
    if (!taskId || !sessionId) return;
    onDataDisposableRef.current = xtermRef.current.onData((data) => {
      if (/^\x1b\[\d+;\d+R$/.test(data) || /^\x1b\[\d+R$/.test(data)) return;
      send("shell.input", { session_id: sessionId, data });
    });
    return () => {
      onDataDisposableRef.current?.dispose();
      onDataDisposableRef.current = null;
    };
  }, [taskId, sessionId, send, isReadOnlyMode]);
  useTerminalOutputWrite(xtermRef, isReadOnlyMode, processOutput, shellOutput, lastOutputLengthRef);

  useShellSubscription({
    refs: { xtermRef, lastOutputLengthRef },
    taskId,
    sessionId,
    canSubscribe,
    send,
    storeApi,
  });

  if (isReadOnlyMode) {
    return (
      <div className="h-full w-full bg-transparent relative">
        <div ref={terminalRef} className="p-1 absolute inset-0" />
        {isStopping && (
          <div className="absolute right-3 top-2 text-xs text-muted-foreground">Stopping…</div>
        )}
      </div>
    );
  }
  if (isSessionFailed) {
    return (
      <div className="h-full p-4 w-full bg-transparent flex flex-col gap-2">
        <div className="text-sm text-destructive/80">Session failed</div>
        {errorMessage && <div className="text-xs text-muted-foreground">{errorMessage}</div>}
      </div>
    );
  }
  return <div ref={terminalRef} className="h-full p-1 w-full overflow-hidden bg-transparent" />;
}
