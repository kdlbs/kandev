"use client";

import { useEffect, useRef, type MutableRefObject } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";
import { t as translate } from "@/lib/i18n";
import {
  agentLoginStreamUrl,
  getAgentLoginStatus,
  resizeAgentLogin,
  stopAgentLogin,
  type AgentLoginSession,
  type ApiRequestOptions,
} from "@/lib/api";
import { openExternalLink } from "@/lib/desktop/external-links";
import { clearBufferReader, exposeBufferReader } from "@/components/task/terminal-buffer-reader";
import { pendingPtyStarts, type PendingPtyStart } from "./pty-terminal-lifecycle";
import { FONT } from "@/lib/theme/colors";
import {
  getFixedDarkTerminalTheme,
  TERMINAL_MINIMUM_CONTRAST_RATIO,
} from "@/lib/theme/terminal-theme";

export type PtySessionStatus = "connecting" | "running" | "exited" | "error";

export type PtyTerminalState = {
  status: PtySessionStatus;
  sessionId: string | null;
  exitCode: number | null;
  error: string | null;
};

export type PtyStartOptions = ApiRequestOptions & { clientId?: string };

export type StartPtySession = (
  size: { cols: number; rows: number },
  options?: PtyStartOptions,
) => Promise<AgentLoginSession>;

export type PtyTerminalLifecycle = "stop-on-unmount" | "detach-on-unmount";

type PtyTerminalViewProps = {
  startSession: StartPtySession;
  sessionId?: string | null;
  clientId?: string;
  /** Stable owner key used to cancel a start when a tab is explicitly closed. */
  ownerId?: string;
  lifecycle?: PtyTerminalLifecycle;
  initialInput?: string;
  testIdPrefix?: string;
  className?: string;
  onStateChange?: (state: PtyTerminalState) => void;
};

function createTerminal(container: HTMLDivElement): { term: Terminal; fit: FitAddon } {
  const term = new Terminal({
    convertEol: true,
    cursorBlink: true,
    fontFamily: FONT.mono,
    fontSize: FONT.size,
    minimumContrastRatio: TERMINAL_MINIMUM_CONTRAST_RATIO,
    theme: getFixedDarkTerminalTheme(),
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  term.loadAddon(
    new WebLinksAddon((event, uri) => {
      event.preventDefault();
      void openExternalLink(uri).catch(() => undefined);
    }),
  );
  term.open(container);
  exposeBufferReader(container, term);
  fit.fit();
  return { term, fit };
}

function wireResize(
  container: HTMLDivElement,
  fitRef: MutableRefObject<FitAddon | null>,
  termRef: MutableRefObject<Terminal | null>,
  sessionIdRef: MutableRefObject<string | null>,
  wsRef: MutableRefObject<WebSocket | null>,
): ResizeObserver {
  const observer = new ResizeObserver(() => {
    if (!fitRef.current || !termRef.current) return;
    try {
      fitRef.current.fit();
    } catch {
      return;
    }
    const sessionId = sessionIdRef.current;
    if (!sessionId) return;
    const size = { cols: termRef.current.cols, rows: termRef.current.rows };
    void resizeAgentLogin(sessionId, size).catch(() => undefined);
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "resize", ...size }));
    }
  });
  observer.observe(container);
  return observer;
}

function reportExit(
  sessionId: string,
  exitCode: number | null,
  report: (state: PtyTerminalState) => void,
  disarm: () => void,
) {
  disarm();
  report({ status: "exited", sessionId, exitCode, error: null });
}

function openSessionWebSocket(
  sessionId: string,
  term: Terminal,
  initialInput: string | undefined,
  report: (state: PtyTerminalState) => void,
  disarm: () => void,
): WebSocket {
  const ws = new WebSocket(agentLoginStreamUrl(sessionId));
  ws.binaryType = "arraybuffer";
  ws.onmessage = (event) => {
    if (typeof event.data === "string") {
      try {
        const message = JSON.parse(event.data) as { type: string; exit_code?: number };
        if (message.type === "exit") {
          reportExit(sessionId, message.exit_code ?? null, report, disarm);
        }
      } catch {
        // Ignore non-JSON text frames.
      }
      return;
    }
    term.write(new Uint8Array(event.data as ArrayBuffer));
  };
  ws.onerror = () => {
    disarm();
    report({
      status: "error",
      sessionId,
      exitCode: null,
      error: translate("agents:ptyConnectionError"),
    });
  };
  ws.onclose = () => {
    disarm();
  };
  if (initialInput) {
    ws.addEventListener("open", () => ws.send(new TextEncoder().encode(initialInput)), {
      once: true,
    });
  }
  return ws;
}

type MountArgs = {
  container: HTMLDivElement;
  startSession: StartPtySession;
  initialSessionId: string | null;
  clientId?: string;
  ownerId?: string;
  lifecycle: PtyTerminalLifecycle;
  initialInput?: string;
  report: (state: PtyTerminalState) => void;
  termRef: MutableRefObject<Terminal | null>;
  fitRef: MutableRefObject<FitAddon | null>;
  wsRef: MutableRefObject<WebSocket | null>;
  sessionIdRef: MutableRefObject<string | null>;
  mountGenerationRef: MutableRefObject<number>;
};

type LateStartContext = {
  mountGeneration: number;
  currentGeneration: number;
  hasAttachIdentity: boolean;
};

function shouldStopLateStart(
  lifecycle: PtyTerminalLifecycle,
  pending: PendingPtyStart | null,
  cancelled: boolean,
  context: LateStartContext,
): boolean {
  if (pending?.cancelled) return true;
  if (lifecycle !== "stop-on-unmount" || !cancelled) return false;
  return !context.hasAttachIdentity || context.mountGeneration === context.currentGeneration;
}

function registerPendingStart(ownerId?: string): PendingPtyStart | null {
  if (!ownerId) return null;
  const pending: PendingPtyStart = { cancelled: false };
  pendingPtyStarts.set(ownerId, pending);
  return pending;
}

function clearPendingStart(ownerId: string | undefined, pending: PendingPtyStart | null): void {
  if (ownerId && pendingPtyStarts.get(ownerId) === pending) pendingPtyStarts.delete(ownerId);
}

async function resolveSession(
  args: MountArgs,
  term: Terminal,
  isAttached: boolean,
): Promise<AgentLoginSession> {
  if (isAttached) return getAgentLoginStatus(args.initialSessionId as string);
  return args.startSession(
    { cols: term.cols, rows: term.rows },
    args.clientId ? { clientId: args.clientId } : undefined,
  );
}

async function stopIfLateStart(
  args: MountArgs,
  pending: PendingPtyStart | null,
  cancelled: boolean,
  mountGeneration: number,
  sessionId: string,
): Promise<boolean> {
  const shouldStop = shouldStopLateStart(args.lifecycle, pending, cancelled, {
    mountGeneration,
    currentGeneration: args.mountGenerationRef.current,
    hasAttachIdentity: Boolean(args.ownerId ?? args.initialSessionId),
  });
  if (!shouldStop) return false;
  await stopAgentLogin(sessionId).catch(() => undefined);
  return true;
}

function reportSession(
  args: MountArgs,
  session: AgentLoginSession,
  term: Terminal,
  isAttached: boolean,
): void {
  args.sessionIdRef.current = session.session_id;
  reportSessionState(args, session);
  args.wsRef.current = openSessionWebSocket(
    session.session_id,
    term,
    isAttached ? undefined : args.initialInput,
    args.report,
    () => {
      args.sessionIdRef.current = null;
    },
  );
}

function reportSessionState(args: MountArgs, session: AgentLoginSession): void {
  args.report({
    status: session.running ? "running" : "exited",
    sessionId: session.session_id,
    exitCode: session.exit_code ?? null,
    error: null,
  });
}

function reportSessionError(args: MountArgs, error: unknown, cancelled: boolean): void {
  if (cancelled) return;
  const isMissing = isNotFoundError(error);
  args.report({
    status: isMissing ? "exited" : "error",
    sessionId: args.initialSessionId,
    exitCode: null,
    error: isMissing ? translate("agents:sessionUnavailable") : errorMessage(error),
  });
}

async function attachOrStart(args: MountArgs, cancelledRef: { value: boolean }): Promise<void> {
  const term = args.termRef.current;
  if (!term) return;

  const pending = registerPendingStart(args.ownerId);
  const mountGeneration = ++args.mountGenerationRef.current;
  const isAttached = Boolean(args.initialSessionId);

  try {
    const session = await resolveSession(args, term, isAttached);
    if (
      await stopIfLateStart(args, pending, cancelledRef.value, mountGeneration, session.session_id)
    ) {
      clearPendingStart(args.ownerId, pending);
      return;
    }
    clearPendingStart(args.ownerId, pending);
    // A detached mount that finished after cleanup leaves the session alive
    // for its tab owner, but must not attach a WebSocket or xterm instance to
    // the disposed StrictMode generation. A later mount will reattach through
    // the stable client/session identity.
    if (cancelledRef.value) {
      // A Quick Chat terminal may detach while its start request is in
      // flight. Keep the resolved identity in the tab store so an explicit
      // close can still stop the PTY. Do not attach a socket to the disposed
      // terminal; a later mount will reattach through the stored identity.
      if (args.lifecycle === "detach-on-unmount") reportSessionState(args, session);
      return;
    }

    reportSession(args, session, term, isAttached);
  } catch (error) {
    clearPendingStart(args.ownerId, pending);
    const detachedQuickTab =
      args.lifecycle === "detach-on-unmount" && cancelledRef.value && !pending?.cancelled;
    reportSessionError(args, error, cancelledRef.value && !detachedQuickTab);
  }
}

function isNotFoundError(error: unknown): boolean {
  return typeof error === "object" && error !== null && "status" in error
    ? (error as { status?: unknown }).status === 404
    : false;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function mountSession(args: MountArgs): () => void {
  const terminal = createTerminal(args.container);
  args.termRef.current = terminal.term;
  args.fitRef.current = terminal.fit;
  const cancelledRef = { value: false };

  void attachOrStart(args, cancelledRef);
  const resizeObserver = wireResize(
    args.container,
    args.fitRef,
    args.termRef,
    args.sessionIdRef,
    args.wsRef,
  );
  const dataDisposable = terminal.term.onData((data) => {
    if (args.wsRef.current?.readyState === WebSocket.OPEN) {
      args.wsRef.current.send(new TextEncoder().encode(data));
    }
  });

  return () => {
    cancelledRef.value = true;
    resizeObserver.disconnect();
    dataDisposable.dispose();
    if (args.lifecycle === "stop-on-unmount" && args.sessionIdRef.current) {
      void stopAgentLogin(args.sessionIdRef.current).catch(() => undefined);
    }
    args.wsRef.current?.close();
    args.wsRef.current = null;
    clearBufferReader(args.container);
    terminal.term.dispose();
    args.termRef.current = null;
    args.fitRef.current = null;
  };
}

export function PtyTerminalView({
  startSession,
  sessionId,
  clientId,
  ownerId,
  lifecycle = "stop-on-unmount",
  initialInput,
  testIdPrefix = "pty",
  className = "h-[420px] rounded-md bg-[#0b0b0c] p-2 overflow-hidden",
  onStateChange,
}: PtyTerminalViewProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const initialSessionIdRef = useRef(sessionId ?? null);
  const mountGenerationRef = useRef(0);
  const reportRef = useRef(onStateChange);
  reportRef.current = onStateChange;

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return undefined;
    return mountSession({
      container,
      startSession,
      initialSessionId: initialSessionIdRef.current,
      clientId,
      ownerId,
      lifecycle,
      initialInput,
      report: (state) => reportRef.current?.(state),
      termRef,
      fitRef,
      wsRef,
      sessionIdRef,
      mountGenerationRef,
    });
  }, [clientId, initialInput, lifecycle, ownerId, startSession]);

  return <div ref={containerRef} data-testid={`${testIdPrefix}-terminal`} className={className} />;
}
