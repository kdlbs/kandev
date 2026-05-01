/**
 * Console + window-error interceptor that mirrors entries into the in-memory
 * log ring buffer (see `./buffer.ts`). Idempotent — safe to call multiple
 * times. Designed to be installed once from the top-level client boot.
 */

import { getLogBuffer, type LogEntry, type LogLevel } from "./buffer";

let installed = false;

const LEVELS: LogLevel[] = ["debug", "info", "warn", "error"];

function safeStringify(arg: unknown): unknown {
  if (
    typeof arg === "string" ||
    typeof arg === "number" ||
    typeof arg === "boolean" ||
    arg === null
  ) {
    return arg;
  }
  if (arg instanceof Error) {
    return { name: arg.name, message: arg.message, stack: arg.stack };
  }
  try {
    JSON.stringify(arg);
    return arg;
  } catch {
    return String(arg);
  }
}

function extractMessage(first: unknown): string {
  if (typeof first === "string") return first;
  if (first instanceof Error) return first.message;
  return String(first ?? "");
}

function buildEntry(level: LogLevel, source: string, args: unknown[]): LogEntry {
  const message = extractMessage(args[0]);
  // Preserve the originating Error's stack on the entry so the bundle keeps it
  // even though we serialize args by value below.
  const stack = args[0] instanceof Error ? args[0].stack : undefined;
  return {
    timestamp: new Date().toISOString(),
    level,
    source,
    message,
    args: args.length > 1 ? args.slice(1).map(safeStringify) : undefined,
    stack,
  };
}

export function installConsoleInterceptor(): void {
  if (installed) return;
  if (typeof window === "undefined") return;
  installed = true;

  const buffer = getLogBuffer();

  for (const level of LEVELS) {
    const original = console[level]?.bind(console);
    if (!original) continue;
    console[level] = (...args: unknown[]) => {
      try {
        buffer.push(buildEntry(level, "console", args));
      } catch {
        // never let interceptor break the host console call
      }
      original(...args);
    };
  }

  window.addEventListener("error", (event) => {
    buffer.push({
      timestamp: new Date().toISOString(),
      level: "error",
      source: "window.onerror",
      message: event.message ?? "Uncaught error",
      args: [
        {
          filename: event.filename,
          lineno: event.lineno,
          colno: event.colno,
          error:
            event.error instanceof Error
              ? { name: event.error.name, stack: event.error.stack }
              : undefined,
        },
      ],
      stack: event.error instanceof Error ? event.error.stack : undefined,
    });
  });

  window.addEventListener("unhandledrejection", (event) => {
    const reason = event.reason;
    const message =
      reason instanceof Error ? reason.message : String(reason ?? "Unhandled rejection");
    buffer.push({
      timestamp: new Date().toISOString(),
      level: "error",
      source: "unhandledrejection",
      message,
      args:
        reason instanceof Error
          ? [{ name: reason.name, stack: reason.stack }]
          : [safeStringify(reason)],
      stack: reason instanceof Error ? reason.stack : undefined,
    });
  });
}

/** Exposed for tests. */
export function _resetInstalledForTesting(): void {
  installed = false;
}
