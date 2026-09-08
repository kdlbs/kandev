export type WebSocketRequestErrorDetails = Record<string, unknown>;

/** Error returned when the backend rejects a request with a typed payload. */
export class WebSocketRequestError extends Error {
  readonly code?: string;
  readonly details?: WebSocketRequestErrorDetails;

  constructor(message: string, code?: string, details?: WebSocketRequestErrorDetails) {
    super(message);
    this.name = "WebSocketRequestError";
    this.code = code;
    this.details = details;
  }
}

/** Preserve the structured fields from a rejected WebSocket request. */
export function toWebSocketRequestError(payload: unknown): WebSocketRequestError {
  const record =
    typeof payload === "object" && payload !== null
      ? (payload as { code?: unknown; message?: unknown; details?: unknown })
      : undefined;
  // i18n-exempt: transport fallback used when the backend supplies no message.
  const message =
    typeof record?.message === "string" && record.message
      ? record.message
      : "WebSocket request failed";
  const code = typeof record?.code === "string" ? record.code : undefined;
  const details =
    typeof record?.details === "object" && record.details !== null
      ? (record.details as WebSocketRequestErrorDetails)
      : undefined;
  return new WebSocketRequestError(message, code, details);
}
