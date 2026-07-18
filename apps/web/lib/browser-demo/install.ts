/* eslint-disable complexity, max-lines-per-function */

import type { BootPayload } from "@/src/boot-payload";
import type { DemoHttpResponse, DemoWorkerRequest, DemoWorkerResponse } from "./protocol";
import { DEMO_STORAGE_KEY } from "./scenario";

type PendingRequest = {
  resolve(value: unknown): void;
  reject(error: Error): void;
};

export async function installBrowserDemo(): Promise<void> {
  history.replaceState({}, "", "/");
  localStorage.setItem("kandev.onboarding.completed", "true");
  const worker = new Worker(new URL("./worker.ts", import.meta.url), { type: "module" });
  const pending = new Map<string, PendingRequest>();
  const sockets = new Map<string, DemoWebSocket>();
  let sequence = 0;

  worker.addEventListener("message", (event: MessageEvent<DemoWorkerResponse>) => {
    const message = event.data;
    if (message.kind === "persist") {
      localStorage.setItem(DEMO_STORAGE_KEY, message.state);
      return;
    }
    if (message.kind === "ws-event") {
      const targets = message.socketId === "*" ? sockets.values() : [sockets.get(message.socketId)];
      for (const socket of targets) socket?.receive(message);
      return;
    }
    const request = pending.get(message.id);
    if (!request) return;
    pending.delete(message.id);
    request.resolve(message.kind === "http-result" ? message.response : message.value);
  });

  function call(
    message:
      | { kind: "init"; id?: string; persistedState?: string }
      | {
          kind: "http";
          id?: string;
          request: Extract<DemoWorkerRequest, { kind: "http" }>["request"];
        },
  ) {
    const id = message.id ?? `demo-${++sequence}`;
    return new Promise<unknown>((resolve, reject) => {
      pending.set(id, { resolve, reject });
      worker.postMessage({ ...message, id });
    });
  }

  const nativeFetch = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request?.url ?? String(input), window.location.href);
    if (!url.pathname.startsWith("/api/") && url.pathname !== "/health") {
      return nativeFetch(input, init);
    }
    const headers = Object.fromEntries(new Headers(init?.headers ?? request?.headers).entries());
    let body = init?.body;
    if (body == null && request && request.method !== "GET" && request.method !== "HEAD") {
      body = await request.clone().text();
    }
    const response = (await call({
      kind: "http",
      request: {
        method: init?.method ?? request?.method ?? "GET",
        path: `${url.pathname}${url.search}`,
        headers,
        body: serializeBody(body),
      },
    })) as DemoHttpResponse;
    const responseBody = response.body === undefined ? null : JSON.stringify(response.body);
    return new Response(responseBody, { status: response.status, headers: response.headers });
  };

  class DemoWebSocket extends EventTarget {
    static readonly CONNECTING = 0;
    static readonly OPEN = 1;
    static readonly CLOSING = 2;
    static readonly CLOSED = 3;
    readonly url: string;
    readonly protocol = "";
    readonly extensions = "";
    readonly bufferedAmount = 0;
    readonly socketId: string;
    binaryType: BinaryType = "blob";
    readyState = DemoWebSocket.CONNECTING;
    onopen: ((event: Event) => void) | null = null;
    onmessage: ((event: MessageEvent) => void) | null = null;
    onerror: ((event: Event) => void) | null = null;
    onclose: ((event: CloseEvent) => void) | null = null;

    constructor(url: string | URL) {
      super();
      this.url = String(url);
      this.socketId = `socket-${++sequence}`;
      sockets.set(this.socketId, this);
      worker.postMessage({ kind: "ws-open", socketId: this.socketId } satisfies DemoWorkerRequest);
    }

    send(data: string | ArrayBufferLike | Blob | ArrayBufferView) {
      if (this.readyState !== DemoWebSocket.OPEN)
        throw new DOMException("WebSocket is not open", "InvalidStateError");
      if (typeof data !== "string")
        throw new TypeError("The browser demo supports text WebSocket messages only");
      worker.postMessage({
        kind: "ws-send",
        socketId: this.socketId,
        data,
      } satisfies DemoWorkerRequest);
    }

    close() {
      if (this.readyState >= DemoWebSocket.CLOSING) return;
      this.readyState = DemoWebSocket.CLOSING;
      worker.postMessage({ kind: "ws-close", socketId: this.socketId } satisfies DemoWorkerRequest);
    }

    receive(message: Extract<DemoWorkerResponse, { kind: "ws-event" }>) {
      if (message.event === "open") {
        this.readyState = DemoWebSocket.OPEN;
        this.dispatch("open", new Event("open"));
      } else if (message.event === "message") {
        this.dispatch("message", new MessageEvent("message", { data: message.data ?? "" }));
      } else {
        this.readyState = DemoWebSocket.CLOSED;
        sockets.delete(this.socketId);
        this.dispatch("close", new CloseEvent("close", { code: 1000 }));
      }
    }

    private dispatch(name: "open" | "message" | "close", event: Event) {
      this.dispatchEvent(event);
      const handler = this[`on${name}`] as ((value: never) => void) | null;
      handler?.call(this, event as never);
    }
  }

  window.WebSocket = DemoWebSocket as unknown as typeof WebSocket;
  const payload = (await call({
    kind: "init",
    persistedState: localStorage.getItem(DEMO_STORAGE_KEY) ?? undefined,
  })) as BootPayload;
  (window as Window & { __KANDEV_BOOT_PAYLOAD__?: BootPayload }).__KANDEV_BOOT_PAYLOAD__ = payload;
  window.parent?.postMessage(
    { source: "kandev-browser-demo", kind: "ready" },
    window.location.origin,
  );
}

function serializeBody(body: BodyInit | null | undefined): string | undefined {
  if (body == null) return undefined;
  return typeof body === "string" ? body : String(body);
}
