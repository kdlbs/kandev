export type NativeMCPDiagnosticOperation = "enable" | "list_tools";
export type NativeMCPDiagnosticStage = "resolve" | "start" | "wait" | "cleanup" | "output";
export type NativeMCPDiagnosticKind =
  | "executable_unavailable"
  | "start_failed"
  | "wait_failed"
  | "output_wait_timeout"
  | "timeout"
  | "canceled"
  | "cleanup_failed"
  | "exit_status"
  | "output_truncated"
  | "unrecognized_output";

export type NativeMCPDiagnostic = {
  operation: NativeMCPDiagnosticOperation;
  stage: NativeMCPDiagnosticStage;
  kind: NativeMCPDiagnosticKind;
  message: string;
  exitCode?: number;
  cleanupMessage?: string;
};

export type NativeMCPDiagnosticPayload = Omit<
  NativeMCPDiagnostic,
  "exitCode" | "cleanupMessage"
> & {
  exit_code?: number;
  cleanup_message?: string;
};

export const NATIVE_MCP_DIAGNOSTIC_MESSAGE_LIMIT = 1024;

const OPERATIONS: readonly NativeMCPDiagnosticOperation[] = ["enable", "list_tools"];
const STAGES: readonly NativeMCPDiagnosticStage[] = [
  "resolve",
  "start",
  "wait",
  "cleanup",
  "output",
];
const KINDS: readonly NativeMCPDiagnosticKind[] = [
  "executable_unavailable",
  "start_failed",
  "wait_failed",
  "output_wait_timeout",
  "timeout",
  "canceled",
  "cleanup_failed",
  "exit_status",
  "output_truncated",
  "unrecognized_output",
];
const URL_SPAN = /(?:\b[a-z][a-z\d+.-]*:\/\/|\bwww\.)[^\s<>"']+/i;
const ANSI_ESCAPE = /\u001b(?:\[[0-?]*[ -/]*[@-~]|\][^\u0007]*(?:\u0007|\u001b\\)|[@-_])/;
const CONTROL_CHARACTER = /[\u0000-\u001f\u007f-\u009f]/;
type RequiredDiagnosticFields = Record<string, unknown> & {
  operation: NativeMCPDiagnosticOperation;
  stage: NativeMCPDiagnosticStage;
  kind: NativeMCPDiagnosticKind;
  message: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function utf8Length(value: string): number {
  return new TextEncoder().encode(value).byteLength;
}

function isSafeDiagnosticMessage(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    utf8Length(value) <= NATIVE_MCP_DIAGNOSTIC_MESSAGE_LIMIT &&
    !URL_SPAN.test(value) &&
    !ANSI_ESCAPE.test(value) &&
    !CONTROL_CHARACTER.test(value)
  );
}

function hasValidRequiredFields(value: Record<string, unknown>): value is RequiredDiagnosticFields {
  return (
    typeof value.operation === "string" &&
    OPERATIONS.includes(value.operation as NativeMCPDiagnosticOperation) &&
    typeof value.stage === "string" &&
    STAGES.includes(value.stage as NativeMCPDiagnosticStage) &&
    typeof value.kind === "string" &&
    KINDS.includes(value.kind as NativeMCPDiagnosticKind) &&
    isSafeDiagnosticMessage(value.message)
  );
}

function readExitCode(value: unknown): number | null | undefined {
  if (value === undefined) return undefined;
  return Number.isSafeInteger(value) && (value as number) >= 0 ? (value as number) : null;
}

function readCleanupMessage(value: unknown): string | null | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== "string") return null;
  if (value === "") return undefined;
  return isSafeDiagnosticMessage(value) ? value : null;
}

/** Validates the server-owned closed schema before diagnostic data reaches a UI. */
export function normalizeNativeMcpDiagnostic(value: unknown): NativeMCPDiagnostic | undefined {
  if (!isRecord(value) || !hasValidRequiredFields(value)) return undefined;
  const exitCode = readExitCode(value.exit_code);
  const cleanupMessage = readCleanupMessage(value.cleanup_message);
  if (exitCode === null || cleanupMessage === null) return undefined;

  return {
    operation: value.operation as NativeMCPDiagnosticOperation,
    stage: value.stage as NativeMCPDiagnosticStage,
    kind: value.kind as NativeMCPDiagnosticKind,
    message: value.message,
    ...(exitCode !== undefined ? { exitCode } : {}),
    ...(cleanupMessage ? { cleanupMessage } : {}),
  };
}
