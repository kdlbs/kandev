"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useSession } from "@/hooks/domains/session/use-session";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { getTerminalTheme } from "@/lib/theme/terminal-theme";
import { useTerminalLinkHandler } from "@/hooks/use-terminal-link-handler";
import { buildTerminalFontFamily } from "@/lib/terminal/terminal-font";
import { exposeBufferReader } from "./terminal-buffer-reader";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { matchesShortcut } from "@/lib/keyboard/utils";
import { useTerminalSearch } from "./use-terminal-search";
import { TerminalSearchBar } from "./terminal-search-bar";
import { usePanelSearch } from "@/hooks/use-panel-search";
import { suppressIOSKeyboardAssists } from "@/lib/terminal/suppress-ios-keyboard-assists";
import { sendShellInput } from "@/lib/terminal/send-shell-input";

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

type ShellTerminalInitOptions = {
  refs: TerminalRefs;
  isReadOnlyMode: boolean;
  taskId: string | null;
  sessionId: string | null | undefined;
  linkHandler?: (event: MouseEvent, uri: string) => void;
  fontFamily?: string;
  fontSize?: number;
  onReady?: () => void;
};

function useTerminalInit({
  refs,
  isReadOnlyMode,
  taskId,
  sessionId,
  linkHandler,
  fontFamily,
  fontSize,
  onReady,
}: ShellTerminalInitOptions) {
  const { terminalRef, xtermRef, fitAddonRef, lastOutputLengthRef, outputRef } = refs;

  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return;
    const terminal = new Terminal({
      cursorBlink: !isReadOnlyMode,
      disableStdin: isReadOnlyMode,
      convertEol: isReadOnlyMode,
      fontSize: fontSize ?? (isReadOnlyMode ? 12 : 13),
      fontFamily: fontFamily || 'Menlo, Monaco, "Courier New", monospace',
      macOptionIsMeta: true,
      theme: getTerminalTheme(terminalRef.current),
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    const webLinksAddon = new WebLinksAddon(linkHandler);
    terminal.loadAddon(webLinksAddon);
    terminal.open(terminalRef.current);
    suppressIOSKeyboardAssists(terminalRef.current);
    exposeBufferReader(terminalRef.current, terminal);
    fitAddon.fit();
    xtermRef.current = terminal;
    fitAddonRef.current = fitAddon;
    onReady?.();

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
    linkHandler,
    fontFamily,
    fontSize,
    terminalRef,
    xtermRef,
    fitAddonRef,
    lastOutputLengthRef,
    outputRef,
    onReady,
  ]);
}

type ShellSubscriptionOptions = {
  refs: Pick<TerminalRefs, "xtermRef" | "lastOutputLengthRef">;
  taskId: string | null;
  sessionId: string | null | undefined;
  canSubscribe: boolean;
  agentctlStatusKey: string;
  send: (action: string, payload: Record<string, unknown>) => void;
  storeApi: ReturnType<typeof useAppStoreApi>;
};

function useShellSubscription({
  refs,
  taskId,
  sessionId,
  canSubscribe,
  agentctlStatusKey: _agentctlStatusKey,
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
  }, [
    taskId,
    sessionId,
    storeApi,
    canSubscribe,
    send,
    xtermRef,
    lastOutputLengthRef,
    _agentctlStatusKey,
  ]);
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

type ShellKeyHandlerOptions = {
  xtermRef: React.RefObject<Terminal | null>;
  isReadOnlyMode: boolean;
  sessionId: string | null | undefined;
  send: (action: string, payload: Record<string, unknown>) => void;
  onFindInPanel: () => void;
  isTerminalReady: boolean;
};

/** Custom xterm key handler: Ctrl/Cmd+F → open search; Cmd+Arrow → Home/End on macOS. */
function useShellTerminalKeyHandler({
  xtermRef,
  isReadOnlyMode,
  sessionId,
  send,
  onFindInPanel,
  isTerminalReady,
}: ShellKeyHandlerOptions) {
  // Use a ref so the handler (attached once) always reads the latest values
  // without needing cleanup (attachCustomKeyEventHandler has no dispose).
  const stateRef = useRef({ sessionId, send, onFindInPanel, isReadOnlyMode });
  useEffect(() => {
    stateRef.current = { sessionId, send, onFindInPanel, isReadOnlyMode };
  });

  const attachedRef = useRef(false);
  useEffect(() => {
    if (!xtermRef.current || !isTerminalReady || attachedRef.current) return;
    attachedRef.current = true;
    xtermRef.current.attachCustomKeyEventHandler((event) => {
      const {
        sessionId: sid,
        send: sendFn,
        onFindInPanel: findFn,
        isReadOnlyMode: ro,
      } = stateRef.current;
      if (matchesShortcut(event, SHORTCUTS.FIND_IN_PANEL)) {
        event.preventDefault();
        if (event.type === "keydown") findFn();
        return false;
      }
      if (
        !ro &&
        event.type === "keydown" &&
        event.metaKey &&
        !event.ctrlKey &&
        !event.altKey &&
        sid &&
        (event.key === "ArrowLeft" || event.key === "ArrowRight")
      ) {
        event.preventDefault();
        const seq = event.key === "ArrowLeft" ? "\x01" : "\x05";
        sendFn("shell.input", { session_id: sid, data: seq });
        return false;
      }
      return true;
    });
  }, [xtermRef, isTerminalReady]);
}

/** Handles user input in interactive mode, filtering out cursor position responses. */
function useShellInputHandler(opts: {
  xtermRef: React.RefObject<Terminal | null>;
  onDataDisposableRef: React.MutableRefObject<{ dispose: () => void } | null>;
  isReadOnlyMode: boolean;
  taskId: string | null;
  sessionId: string | null | undefined;
}) {
  const { xtermRef, onDataDisposableRef, isReadOnlyMode, taskId, sessionId } = opts;
  useEffect(() => {
    if (!xtermRef.current || isReadOnlyMode) return;
    onDataDisposableRef.current?.dispose();
    onDataDisposableRef.current = null;
    if (!taskId || !sessionId) return;
    onDataDisposableRef.current = xtermRef.current.onData((data) => {
      if (/^\x1b\[\d+;\d+R$/.test(data) || /^\x1b\[\d+R$/.test(data)) return;
      sendShellInput(sessionId, data);
    });
    return () => {
      onDataDisposableRef.current?.dispose();
      onDataDisposableRef.current = null;
    };
  }, [taskId, sessionId, isReadOnlyMode, xtermRef, onDataDisposableRef]);
}

function useShellSessionState(propSessionId: string | undefined, isReadOnlyMode: boolean) {
  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = propSessionId ?? storeSessionId;
  const { session, isActive, isFailed, errorMessage } = useSession(
    isReadOnlyMode ? null : sessionId,
  );
  const agentctlStatus = useSessionAgentctl(isReadOnlyMode ? null : sessionId);
  const taskId = session?.task_id ?? null;
  const isSessionFailed = !isReadOnlyMode && isFailed;
  const shellOutput = useAppStore((state) => {
    if (!sessionId || isReadOnlyMode) return "";
    const envKey = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return state.shell.outputs[envKey] || "";
  });
  const canSubscribe = Boolean(sessionId && isActive && !isReadOnlyMode && !agentctlStatus.isError);
  return {
    sessionId,
    taskId,
    isSessionFailed,
    errorMessage,
    shellOutput,
    canSubscribe,
    agentctlStatusKey: agentctlStatus.status,
  };
}

// eslint-disable-next-line max-lines-per-function -- wires many hooks + refs; each block is already its own hook
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
  const {
    sessionId,
    taskId,
    isSessionFailed,
    errorMessage,
    shellOutput,
    canSubscribe,
    agentctlStatusKey,
  } = useShellSessionState(propSessionId, isReadOnlyMode);
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

  const linkHandler = useTerminalLinkHandler();
  const terminalFontFamily = useAppStore((s) => s.userSettings.terminalFontFamily);
  const terminalFontSize = useAppStore((s) => s.userSettings.terminalFontSize);
  const refs: TerminalRefs = { terminalRef, xtermRef, fitAddonRef, lastOutputLengthRef, outputRef };
  const [isTerminalReady, setIsTerminalReady] = useState(false);
  const onTerminalReady = useCallback(() => setIsTerminalReady(true), []);
  useTerminalInit({
    refs,
    isReadOnlyMode,
    taskId,
    sessionId,
    linkHandler,
    fontFamily: buildTerminalFontFamily(terminalFontFamily),
    fontSize: terminalFontSize ?? undefined,
    onReady: onTerminalReady,
  });

  const search = useTerminalSearch({ xtermRef, isTerminalReady });
  const containerRef = useRef<HTMLDivElement>(null);
  usePanelSearch({
    containerRef,
    isOpen: search.isOpen,
    onOpen: search.open,
    onClose: search.close,
  });

  useShellInputHandler({ xtermRef, onDataDisposableRef, isReadOnlyMode, taskId, sessionId });
  useShellTerminalKeyHandler({
    xtermRef,
    isReadOnlyMode,
    sessionId,
    send,
    onFindInPanel: search.open,
    isTerminalReady,
  });
  useTerminalOutputWrite(xtermRef, isReadOnlyMode, processOutput, shellOutput, lastOutputLengthRef);

  useShellSubscription({
    refs: { xtermRef, lastOutputLengthRef },
    taskId,
    sessionId,
    canSubscribe,
    agentctlStatusKey,
    send,
    storeApi,
  });

  const searchBar = <TerminalSearchBar search={search} />;
  if (isReadOnlyMode) {
    return (
      <div
        ref={containerRef}
        tabIndex={-1}
        data-panel-kind="terminal"
        className="h-full w-full bg-transparent relative outline-none"
      >
        <div className="p-1 pb-2 absolute inset-0">
          <div ref={terminalRef} className="h-full w-full" />
        </div>
        {isStopping && (
          <div className="absolute right-3 top-2 text-xs text-muted-foreground">Stopping…</div>
        )}
        {searchBar}
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
  return (
    <div
      ref={containerRef}
      tabIndex={-1}
      data-panel-kind="terminal"
      className="h-full p-1 pb-2 w-full overflow-hidden bg-transparent relative outline-none"
    >
      <div ref={terminalRef} className="h-full w-full" />
      {searchBar}
    </div>
  );
}
