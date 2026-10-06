import type { ReactNode } from "react";
import { act, cleanup, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import targets from "../../backend/internal/editors/service/testdata/embedded-editor-file-targets.json";
import { useOpenSessionInEditor } from "./use-open-session-in-editor";

type Wire = { id: string; type: string; action: string; payload: unknown };
type EditorHook = ReturnType<typeof useOpenSessionInEditor>;
type EditorOptions = Parameters<EditorHook["open"]>[0];
type HookResult = { current: EditorHook };
type HttpRequest = { url: string; init: RequestInit | undefined };

// Only the socket wire is substituted; the real client serializes and settles RPCs.
class Socket {
  static readonly OPEN = 1;
  static readonly CLOSED = 3;
  static current: Socket;
  static failOpenFile = false;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  requests: Wire[] = [];

  constructor() {
    Socket.current = this;
  }

  send(data: string) {
    const frame = JSON.parse(data) as Wire;
    this.requests.push(frame);
    const failed = Socket.failOpenFile && frame.action === "vscode.openFile";
    queueMicrotask(() => {
      this.onmessage?.({
        data: JSON.stringify({
          id: frame.id,
          type: failed ? "error" : "response",
          payload: failed
            ? { code: "internal_error", message: "remote CLI unavailable" }
            : { success: true },
        }),
      });
    });
  }

  close() {
    this.readyState = Socket.CLOSED;
  }
}

let client: WebSocketClient;
let previousClient: WebSocketClient | null;
let httpRequests: HttpRequest[];
let resolveEditorHTTP: ((response: Response) => void) | null;
const sessionId = "embedded-editor-session";
const editorId = "embedded-vscode";
const bareSentinel = "internal://vscode";
const wrapper = ({ children }: { children: ReactNode }) => (
  <ToastProvider>{children}</ToastProvider>
);

beforeEach(() => {
  vi.useFakeTimers();
  previousClient = getWebSocketClient();
  httpRequests = [];
  resolveEditorHTTP = null;
  Socket.failOpenFile = false;
  vi.stubGlobal("WebSocket", Socket);
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      httpRequests.push({ url, init });
      if (url.endsWith(`/task-sessions/${sessionId}/open-editor`)) {
        return new Promise<Response>((resolve) => {
          resolveEditorHTTP = resolve;
        });
      }
      if (url.endsWith("/api/v1/system/logs/frontend-errors")) {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.reject(new Error(`Unexpected HTTP request: ${url}`));
    }),
  );
  client = new WebSocketClient("ws://embedded-editor-target-wire", undefined, { enabled: false });
  client.connect();
  Socket.current.readyState = Socket.OPEN;
  Socket.current.onopen?.();
  setWebSocketClient(client);
});

afterEach(async () => {
  try {
    await act(async () => {
      client.disconnect();
      await vi.runOnlyPendingTimersAsync();
    });
    expect(vi.getTimerCount()).toBe(0);
  } finally {
    cleanup();
    setWebSocketClient(previousClient);
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  }
});

function mountEditor(currentSession: string | null = sessionId) {
  // Dockview's default null API permits dispatch assertions, not panel-rendering claims.
  return renderHook(() => useOpenSessionInEditor(currentSession), { wrapper }).result;
}

async function settleEditor(
  result: HookResult,
  options: EditorOptions,
  body: unknown,
  status = 200,
) {
  let opening!: ReturnType<EditorHook["open"]>;
  act(() => {
    opening = result.current.open(options);
  });
  expect(result.current.isLoading).toBe(true);
  const resolve = resolveEditorHTTP;
  if (!resolve) throw new Error("Expected the real API to reach the deferred HTTP wire");
  let returned: Awaited<ReturnType<EditorHook["open"]>> = null;
  await act(async () => {
    resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
    returned = await opening;
    await vi.advanceTimersByTimeAsync(0);
  });
  return returned;
}

function expectOpenEditorRequest(body: unknown) {
  const requests = httpRequests.filter(({ url }) => url.endsWith("/open-editor"));
  expect(requests).toHaveLength(1);
  const { url, init } = requests[0];
  expect(url).toContain(`/api/v1/task-sessions/${sessionId}/open-editor`);
  expect(init).toMatchObject({ method: "POST", cache: "no-store", credentials: "include" });
  expect(JSON.parse(init?.body as string)).toEqual(body);
}

function registerTargetCases() {
  // @covers AC-UI-EMBEDDED-EDITOR-TARGET-001.1, AC-UI-EMBEDDED-EDITOR-TARGET-001.2,
  // AC-UI-EMBEDDED-EDITOR-TARGET-001.3, AC-UI-EMBEDDED-EDITOR-TARGET-001.5,
  // AC-UI-EMBEDDED-EDITOR-TARGET-001.6
  it.each(targets)("$name", async ({ input, target, url }) => {
    const result = mountEditor();
    const returned = await settleEditor(
      result,
      {
        editorId,
        filePath: input.path,
        line: input.line,
        column: input.column,
        worktreeId: "wt-2",
      },
      { url },
    );
    expectOpenEditorRequest({
      editor_id: editorId,
      file_path: input.path,
      line: input.line,
      column: input.column,
      worktree_id: "wt-2",
    });
    expect(Socket.current.requests).toHaveLength(1);
    expect(Socket.current.requests[0]).toMatchObject({
      type: "request",
      action: "vscode.openFile",
    });
    expect(Socket.current.requests[0].payload).toEqual({ session_id: sessionId, ...target });
    expect(returned).toEqual({ url });
    expect(result.current.status).toBe("success");
    expect(result.current.isLoading).toBe(false);
  });
}

function registerWorkspaceControls() {
  // @covers AC-UI-EMBEDDED-EDITOR-TARGET-001.4
  it("opens the workspace without dispatching a file", async () => {
    const result = mountEditor();
    expect(await settleEditor(result, { editorId }, { url: bareSentinel })).toEqual({
      url: bareSentinel,
    });
    expectOpenEditorRequest({ editor_id: editorId });
    expect(Socket.current.requests).toEqual([]);
    expect(result.current.status).toBe("success");
    expect(result.current.isLoading).toBe(false);
  });

  it("does not dispatch a marked directory even with a returned target", async () => {
    const result = mountEditor();
    const response = { url: "internal://vscode?goto=src" };
    expect(
      await settleEditor(result, { editorId, filePath: "src", isDirectory: true }, response),
    ).toEqual(response);
    expectOpenEditorRequest({ editor_id: editorId, file_path: "src" });
    expect(Socket.current.requests).toEqual([]);
    expect(result.current.isLoading).toBe(false);
  });

  it("returns an empty response without a file dispatch", async () => {
    const result = mountEditor();
    expect(await settleEditor(result, { editorId }, {})).toEqual({});
    expect(Socket.current.requests).toEqual([]);
    expect(result.current.status).toBe("success");
  });
}

function registerErrorAndExternalControls() {
  // @covers AC-UI-EMBEDDED-EDITOR-TARGET-001.5
  it("returns null without HTTP or RPC when the session is absent", async () => {
    const result = mountEditor(null);
    await act(async () => {
      expect(await result.current.open({ editorId })).toBeNull();
    });
    expect(httpRequests).toEqual([]);
    expect(Socket.current.requests).toEqual([]);
    expect(result.current.status).toBe("success");
    expect(result.current.isLoading).toBe(false);
  });

  it("retains the real localized toast and error state on rejected HTTP", async () => {
    const result = mountEditor();
    expect(
      await settleEditor(result, { editorId }, { error: "workspace path not found" }, 404),
    ).toBeNull();
    expect(screen.getByText("Failed to open editor")).toBeTruthy();
    expect(screen.getByText("workspace path not found")).toBeTruthy();
    expect(screen.getAllByTestId("toast-message")).toHaveLength(1);
    expectOpenEditorRequest({ editor_id: editorId });
    expect(Socket.current.requests).toEqual([]);
    expect(result.current.status).toBe("error");
    expect(result.current.isLoading).toBe(false);
  });

  it("opens an external custom scheme without an embedded RPC", async () => {
    const opened = vi.spyOn(window, "open");
    const result = mountEditor();
    const response = { url: "vscode://file/workspace/main.go" };
    expect(await settleEditor(result, { editorId: "external-vscode" }, response)).toEqual(response);
    expect(opened).toHaveBeenCalledTimes(1);
    expect(opened).toHaveBeenCalledWith(response.url, "_blank", "noopener,noreferrer");
    expectOpenEditorRequest({ editor_id: "external-vscode" });
    expect(Socket.current.requests).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    opened.mock.results[0]?.value?.close();
  });

  it("logs a socket rejection while retaining the successful HTTP result", async () => {
    Socket.failOpenFile = true;
    const warned = vi.spyOn(console, "warn");
    const result = mountEditor();
    const response = { url: "internal://vscode?goto=main.go" };
    expect(await settleEditor(result, { editorId, filePath: "main.go" }, response)).toEqual(
      response,
    );
    expect(Socket.current.requests).toHaveLength(1);
    expect(Socket.current.requests[0].payload).toEqual({
      session_id: sessionId,
      path: "main.go",
      line: 0,
      col: 0,
    });
    expect(warned).toHaveBeenCalledWith(
      "Failed to open file in VS Code:",
      expect.objectContaining({ message: "remote CLI unavailable" }),
    );
    expect(screen.queryByTestId("toast-message")).toBeNull();
    expect(result.current.status).toBe("success");
    expect(result.current.isLoading).toBe(false);
  });
}

describe("embedded editor target wire", () => {
  describe("targets", registerTargetCases);
  describe("workspace controls", registerWorkspaceControls);
  describe("error and external controls", registerErrorAndExternalControls);
});
