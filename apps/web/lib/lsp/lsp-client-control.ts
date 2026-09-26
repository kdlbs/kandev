export type LspBrokerControl = {
  action: string;
  requestId?: string;
  reason?: string;
  documentVersions?: Record<string, number>;
};

export type LspControlWaitOptions = {
  timeoutMs: number;
  send: () => void;
  cancelOnClose?: boolean;
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
  options: LspControlWaitOptions,
): Promise<LspBrokerControl | null> {
  const { timeoutMs, send, cancelOnClose = false } = options;
  return new Promise((resolve) => {
    let settled = false;
    let onClose: (() => void) | null = null;
    const finish = (control: LspBrokerControl | null) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      ws.removeEventListener("message", onMessage);
      if (onClose) ws.removeEventListener("close", onClose);
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
    onClose = () => finish(null);
    const timer = window.setTimeout(() => finish(null), timeoutMs);
    ws.addEventListener("message", onMessage);
    if (cancelOnClose) ws.addEventListener("close", onClose, { once: true });
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
