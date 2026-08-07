import { deriveTaskID } from "@/lib/api/domains/frontend-error-log-api";
import { encodedBytes, MAX_ENTRY_BYTES, type LogEntry, type LogLevel } from "./buffer";
import { stageLogEntry } from "./runtime";

const LEVELS: LogLevel[] = ["debug", "info", "warn", "error"];
const MAX_ARGUMENTS = 20;
const STRING_LIMIT = 4 * 1024;
const ERROR_MESSAGE_LIMIT = 8 * 1024;
const STACK_LIMIT = 16 * 1024;

let installed = false;
let removeWindowListeners: (() => void) | null = null;
let restoreConsole: (() => void) | null = null;

function preview(value: unknown): unknown {
  switch (typeof value) {
    case "string":
      return bounded(value, STRING_LIMIT);
    case "number":
    case "boolean":
      return value;
    case "undefined":
      return { type: "undefined" };
    case "bigint":
      return bounded(value.toString(), STRING_LIMIT);
    case "symbol":
      return { type: "symbol" };
    case "function":
      return { type: "function" };
    case "object":
      if (value === null) return null;
      return errorPreview(value) ?? { type: "object" };
    default:
      return { type: "unknown" };
  }
}

function errorPreview(value: object): { name: string; message: string; stack?: string } | null {
  try {
    if (!(value instanceof Error)) return null;
    return {
      name: bounded(value.name || "Error", STRING_LIMIT),
      message: bounded(value.message, ERROR_MESSAGE_LIMIT),
      stack: value.stack ? bounded(value.stack, STACK_LIMIT) : undefined,
    };
  } catch {
    return null;
  }
}

function extractMessage(first: unknown): string {
  if (typeof first === "string") return bounded(first, ERROR_MESSAGE_LIMIT);
  if (typeof first === "number" || typeof first === "boolean" || typeof first === "bigint") {
    return bounded(String(first), ERROR_MESSAGE_LIMIT);
  }
  if (first && typeof first === "object") {
    const error = errorPreview(first);
    if (error) return error.message;
  }
  return first == null ? "" : `[${typeof first}]`;
}

export function buildLogEntry(level: LogLevel, source: string, args: unknown[]): LogEntry {
  const firstError =
    args[0] && typeof args[0] === "object" ? errorPreview(args[0] as object) : null;
  const entry: LogEntry = {
    timestamp: new Date().toISOString(),
    level,
    source,
    message: extractMessage(args[0]),
    args: args.length > 1 ? args.slice(1, MAX_ARGUMENTS).map(preview) : undefined,
    stack: firstError?.stack,
  };
  addLocation(entry);
  while (encodedBytes(entry) > MAX_ENTRY_BYTES && entry.args?.length) entry.args.pop();
  if (encodedBytes(entry) > MAX_ENTRY_BYTES) entry.stack = undefined;
  if (encodedBytes(entry) > MAX_ENTRY_BYTES) entry.message = bounded(entry.message, STRING_LIMIT);
  return entry;
}

export function installConsoleInterceptor(): void {
  if (installed || typeof window === "undefined") return;
  installed = true;
  const originals = new Map<LogLevel, (...args: unknown[]) => void>();
  for (const level of LEVELS) {
    const original = console[level]?.bind(console);
    if (!original) continue;
    originals.set(level, original);
    console[level] = (...args: unknown[]) => {
      try {
        stageLogEntry(buildLogEntry(level, "console", args));
      } catch {
        // Diagnostics cannot alter console behavior.
      }
      original(...args);
    };
  }
  restoreConsole = () => {
    for (const [level, original] of originals) console[level] = original;
  };

  const onError = (event: ErrorEvent) => {
    try {
      const error =
        event.error && typeof event.error === "object" ? errorPreview(event.error) : null;
      stageLogEntry(
        buildWindowEntry("window.onerror", event.message || "Uncaught error", error?.stack, {
          filename: bounded(event.filename || "", STRING_LIMIT),
          line: event.lineno,
          column: event.colno,
          error: error ?? undefined,
        }),
      );
    } catch {
      // Best effort only.
    }
  };
  const onRejection = (event: PromiseRejectionEvent) => {
    try {
      const error =
        event.reason && typeof event.reason === "object" ? errorPreview(event.reason) : null;
      stageLogEntry(
        buildWindowEntry(
          "unhandledrejection",
          error?.message ?? extractMessage(event.reason),
          error?.stack,
          error ?? preview(event.reason),
        ),
      );
    } catch {
      // Best effort only.
    }
  };
  window.addEventListener("error", onError);
  window.addEventListener("unhandledrejection", onRejection);
  removeWindowListeners = () => {
    window.removeEventListener("error", onError);
    window.removeEventListener("unhandledrejection", onRejection);
  };
}

function buildWindowEntry(
  source: string,
  message: string,
  stack: string | undefined,
  arg: unknown,
) {
  const entry: LogEntry = {
    timestamp: new Date().toISOString(),
    level: "error",
    source,
    message: bounded(message, ERROR_MESSAGE_LIMIT),
    args: [arg],
    stack: stack ? bounded(stack, STACK_LIMIT) : undefined,
  };
  addLocation(entry);
  return entry;
}

function addLocation(entry: LogEntry): void {
  try {
    const url = new URL(window.location.href);
    entry.url = bounded(url.href, ERROR_MESSAGE_LIMIT);
    entry.task_id = deriveTaskID(url);
  } catch {
    // Location is optional.
  }
}

function bounded(value: string, limit: number): string {
  return value.length <= limit ? value : value.slice(0, limit);
}

/** Exposed for tests. */
export function _resetInstalledForTesting(): void {
  removeWindowListeners?.();
  restoreConsole?.();
  removeWindowListeners = null;
  restoreConsole = null;
  installed = false;
}
