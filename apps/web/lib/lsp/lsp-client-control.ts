export type LspBrokerControl = {
  action: string;
  requestId?: string;
  reason?: string;
  documentVersions?: Record<string, number>;
};

let requestCounter = 0;

export function nextLspControlRequestId(): string {
  requestCounter += 1;
  return `lsp-control-${Date.now()}-${requestCounter}`;
}

export function waitForLspControlAck(
  ws: WebSocket,
  requestId: string,
  action: string,
  timeoutMs: number,
  send: () => void,
): Promise<LspBrokerControl | null> {
  return new Promise((resolve) => {
    let settled = false;
    const finish = (control: LspBrokerControl | null) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      ws.removeEventListener("message", onMessage);
      resolve(control);
    };
    const onMessage = (event: MessageEvent) => {
      let message: unknown;
      try {
        message = JSON.parse(event.data as string);
      } catch {
        return;
      }
      if (!isLspBrokerControl(message)) return;
      if (message.action !== action || message.requestId !== requestId) return;
      finish(message);
    };
    const timer = window.setTimeout(() => finish(null), timeoutMs);
    ws.addEventListener("message", onMessage);
    try {
      send();
    } catch {
      finish(null);
    }
  });
}

function isLspBrokerControl(value: unknown): value is LspBrokerControl & { kandev: "lsp" } {
  return (
    typeof value === "object" &&
    value !== null &&
    "kandev" in value &&
    value.kandev === "lsp" &&
    "action" in value &&
    typeof value.action === "string"
  );
}
