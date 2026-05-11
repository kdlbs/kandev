"use client";

import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import {
  agentLoginStreamUrl,
  resizeAgentLogin,
  stopAgentLogin,
  type AgentLoginSession,
} from "@/lib/api";
import type { ApiRequestOptions } from "@/lib/api";

type SessionStatus = "connecting" | "running" | "exited" | "error";

type LoginStateSetters = {
  setStatus: (s: SessionStatus) => void;
  setError: (e: string | null) => void;
  setExitCode: (c: number | null) => void;
};

/**
 * Caller-supplied start function. Returns the same session shape regardless
 * of which underlying endpoint (agent-login vs host-shell) is being driven -
 * once a session exists, stop/resize/stream all share the same session-ID
 * routes on the backend.
 */
export type StartPtySession = (
  size: { cols: number; rows: number },
  options?: ApiRequestOptions,
) => Promise<AgentLoginSession>;

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  /** data-testid prefix for the terminal container element. */
  testIdPrefix?: string;
  startSession: StartPtySession;
  /** Invoked when the user clicks Done. */
  onDone?: () => void;
  /**
   * If set, this text is sent to the shell once the WS connects. Used by
   * auth flows that want to pre-fill a command like `claude auth login`.
   * No trailing newline appended - the user can review and press Enter.
   */
  initialInput?: string;
  /**
   * argv of the actual command running in the PTY (e.g. ["codex", "login",
   * "--device-auth"]). Shown above the terminal so the user can see/copy
   * it - useful after Ctrl+C drops them into a shell and they want to retry.
   */
  command?: string[];
};

function createTerminal(container: HTMLDivElement): { term: Terminal; fit: FitAddon } {
  const term = new Terminal({
    convertEol: true,
    cursorBlink: true,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, "Courier New", monospace',
    fontSize: 13,
    theme: { background: "#0b0b0c" },
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  // Make http(s) URLs clickable - agents like codex print OAuth URLs that
  // span multiple wrapped lines, and the user shouldn't have to copy/paste.
  term.loadAddon(
    new WebLinksAddon((event, uri) => {
      // Open in a new tab. Holding the modifier (Cmd/Ctrl) is xterm's default
      // gesture, but addon-web-links doesn't require it - any click works.
      event.preventDefault();
      window.open(uri, "_blank", "noopener,noreferrer");
    }),
  );
  term.open(container);
  fit.fit();
  return { term, fit };
}

function wireResize(
  container: HTMLDivElement,
  fitRef: React.RefObject<FitAddon | null>,
  termRef: React.RefObject<Terminal | null>,
  sessionIDRef: React.RefObject<string | null>,
  wsRef: React.RefObject<WebSocket | null>,
): ResizeObserver {
  const obs = new ResizeObserver(() => {
    if (!fitRef.current || !termRef.current) return;
    try {
      fitRef.current.fit();
    } catch {
      return;
    }
    const id = sessionIDRef.current;
    if (!id) return;
    const cols = termRef.current.cols;
    const rows = termRef.current.rows;
    void resizeAgentLogin(id, { cols, rows }).catch(() => {});
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "resize", cols, rows }));
    }
  });
  obs.observe(container);
  return obs;
}

function makeWsMessageHandler(term: Terminal, setters: LoginStateSetters) {
  return (ev: MessageEvent) => {
    if (typeof ev.data === "string") {
      try {
        const msg = JSON.parse(ev.data) as { type: string; exit_code?: number };
        if (msg.type === "exit") {
          setters.setStatus("exited");
          setters.setExitCode(msg.exit_code ?? null);
        }
      } catch {
        // ignore non-JSON text frames
      }
      return;
    }
    term.write(new Uint8Array(ev.data as ArrayBuffer));
  };
}

function openSessionWebSocket(
  sessionID: string,
  term: Terminal,
  setters: LoginStateSetters,
  initialInput: string | undefined,
): WebSocket {
  const ws = new WebSocket(agentLoginStreamUrl(sessionID));
  ws.binaryType = "arraybuffer";
  ws.onmessage = makeWsMessageHandler(term, setters);
  ws.onerror = () => {
    setters.setError("Connection error");
    setters.setStatus("error");
  };
  if (initialInput) {
    ws.addEventListener(
      "open",
      () => {
        ws.send(new TextEncoder().encode(initialInput));
      },
      { once: true },
    );
  }
  return ws;
}

type MountArgs = {
  container: HTMLDivElement;
  startSession: StartPtySession;
  setters: LoginStateSetters;
  termRef: React.RefObject<Terminal | null>;
  fitRef: React.RefObject<FitAddon | null>;
  wsRef: React.RefObject<WebSocket | null>;
  sessionIDRef: React.RefObject<string | null>;
  initialInput?: string;
};

function mountSession(args: MountArgs): () => void {
  const term = createTerminal(args.container);
  args.termRef.current = term.term;
  args.fitRef.current = term.fit;
  let cancelled = false;
  // AbortController cancels an in-flight start POST so React StrictMode's
  // double-mount can't leave a half-spawned session behind.
  const startAbort = new AbortController();

  void (async () => {
    try {
      const sess = await args.startSession(
        { cols: term.term.cols, rows: term.term.rows },
        { init: { signal: startAbort.signal } },
      );
      if (cancelled) {
        await stopAgentLogin(sess.session_id);
        return;
      }
      args.sessionIDRef.current = sess.session_id;
      args.wsRef.current = openSessionWebSocket(
        sess.session_id,
        term.term,
        args.setters,
        args.initialInput,
      );
      args.setters.setStatus("running");
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      if (!cancelled) {
        args.setters.setError(err instanceof Error ? err.message : String(err));
        args.setters.setStatus("error");
      }
    }
  })();

  const resizeObs = wireResize(
    args.container,
    args.fitRef,
    args.termRef,
    args.sessionIDRef,
    args.wsRef,
  );
  const dataDisp = term.term.onData((data) => {
    if (args.wsRef.current && args.wsRef.current.readyState === WebSocket.OPEN) {
      args.wsRef.current.send(new TextEncoder().encode(data));
    }
  });

  return () => {
    cancelled = true;
    startAbort.abort();
    resizeObs.disconnect();
    dataDisp.dispose();
    if (args.sessionIDRef.current) {
      void stopAgentLogin(args.sessionIDRef.current).catch(() => {});
      args.sessionIDRef.current = null;
    }
    if (args.wsRef.current) {
      args.wsRef.current.close();
      args.wsRef.current = null;
    }
    term.term.dispose();
    args.termRef.current = null;
    args.fitRef.current = null;
  };
}

function PtySessionView({
  startSession,
  testIdPrefix,
  initialInput,
  onDone,
}: {
  startSession: StartPtySession;
  testIdPrefix?: string;
  initialInput?: string;
  onDone: () => void;
}) {
  const termContainerRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const sessionIDRef = useRef<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<SessionStatus>("connecting");
  const [exitCode, setExitCode] = useState<number | null>(null);

  useEffect(() => {
    const container = termContainerRef.current;
    if (!container) return;
    return mountSession({
      container,
      startSession,
      setters: { setStatus, setError, setExitCode },
      termRef,
      fitRef,
      wsRef,
      sessionIDRef,
      initialInput,
    });
  }, [startSession, initialInput]);

  return (
    <>
      <div
        ref={termContainerRef}
        data-testid={`${testIdPrefix ?? "pty"}-terminal`}
        className="h-[420px] rounded-md bg-[#0b0b0c] p-2 overflow-hidden"
      />
      {status === "connecting" && (
        <p className="text-xs text-muted-foreground">Starting session…</p>
      )}
      {status === "exited" && (
        <p className="text-xs text-muted-foreground">
          Session ended{exitCode != null ? ` (exit ${exitCode})` : ""}.
        </p>
      )}
      {status === "error" && error && <p className="text-xs text-destructive">{error}</p>}
      <DialogFooter>
        <Button
          type="button"
          onClick={onDone}
          className="cursor-pointer"
          data-testid={`${testIdPrefix ?? "pty"}-done`}
        >
          Done
        </Button>
      </DialogFooter>
    </>
  );
}

/**
 * Generic dialog that hosts a PTY-backed terminal. Mounts the inner
 * `<PtySessionView>` only while `open` is true - closing the dialog unmounts
 * it, which fires cleanup (stop session, dispose terminal).
 */
export function PtyTerminalDialog({
  open,
  onOpenChange,
  title,
  description,
  testIdPrefix,
  startSession,
  onDone,
  initialInput,
  command,
}: Props) {
  const handleDone = () => {
    onDone?.();
    onOpenChange(false);
  };

  const cmdLine = command && command.length > 0 ? command.join(" ") : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[820px]">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {cmdLine && (
          <div
            data-testid={`${testIdPrefix ?? "pty"}-command`}
            className="flex items-center gap-1 rounded-md bg-muted px-2 py-1.5 font-mono text-xs"
          >
            <span className="text-muted-foreground">$</span>
            <code className="flex-1 truncate" title={cmdLine}>
              {cmdLine}
            </code>
          </div>
        )}
        {open && (
          <PtySessionView
            startSession={startSession}
            testIdPrefix={testIdPrefix}
            initialInput={initialInput}
            onDone={handleDone}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
