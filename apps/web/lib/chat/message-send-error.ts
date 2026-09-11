export type MessageSendErrorCode =
  | "connection-unavailable"
  | "no-active-session"
  | "session-unavailable"
  | "plan-comment-migration-pending"
  | "plan-comments-changed"
  | "primary-session-changed";

export class MessageSendError extends Error {
  readonly code: MessageSendErrorCode;

  constructor(code: MessageSendErrorCode, message: string) {
    super(message);
    this.name = "MessageSendError";
    this.code = code;
  }
}

export function isMessageSendError(error: unknown): error is MessageSendError {
  if (!(error instanceof Error) || error.name !== "MessageSendError") return false;
  const code = (error as Error & { code?: unknown }).code;
  return (
    code === "connection-unavailable" ||
    code === "no-active-session" ||
    code === "session-unavailable" ||
    code === "plan-comment-migration-pending" ||
    code === "plan-comments-changed" ||
    code === "primary-session-changed"
  );
}
