export type DemoHttpRequest = {
  method: string;
  path: string;
  headers: Record<string, string>;
  body?: string;
};

export type DemoHttpResponse = {
  status: number;
  headers?: Record<string, string>;
  body?: unknown;
  bodyFormat?: "json" | "text";
};

export type DemoWorkerRequest =
  | { kind: "init"; id: string; persistedState?: string }
  | { kind: "http"; id: string; request: DemoHttpRequest }
  | { kind: "ws-open"; socketId: string; url: string }
  | { kind: "ws-send"; socketId: string; data: string | ArrayBuffer }
  | { kind: "ws-close"; socketId: string };

export type DemoWorkerResponse =
  | { kind: "result"; id: string; value: unknown }
  | { kind: "http-result"; id: string; response: DemoHttpResponse }
  | { kind: "ws-event"; socketId: string; event: "open" | "message" | "close"; data?: string }
  | { kind: "persist"; state: string };
