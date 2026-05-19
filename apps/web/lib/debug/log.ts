/**
 * Namespaced debug logger for development.
 *
 * In production (`NODE_ENV === "production"`) the factory returns a no-op
 * function, so debug call sites have zero runtime cost — Next.js inlines the
 * `process.env.NODE_ENV` check at build time and tree-shakes the branch.
 *
 * Output format is logfmt-ish so logs are flat and grep/copy-friendly:
 *
 *   [namespace] message key1=value key2="value with space" key3={"nested":1}
 *
 * Usage:
 *   const debug = createDebugLogger("git-status");
 *   debug("status_update received", { sessionId, fileCount });
 *
 * Logs go through `console.debug`, which the log interceptor mirrors into the
 * ring buffer (see `lib/logger/intercept.ts`), so they also end up in
 * Improve Kandev reports without extra plumbing.
 */

export type DebugLogger = (...args: unknown[]) => void;

const DEV = process.env.NODE_ENV !== "production";

const NOOP: DebugLogger = () => {};

const BARE_VALUE_RE = /^[A-Za-z0-9_\-:./@+]+$/;

function formatValue(value: unknown): string {
  if (value === null) return "null";
  if (value === undefined) return "undefined";
  if (typeof value === "string") {
    return BARE_VALUE_RE.test(value) ? value : JSON.stringify(value);
  }
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
    return String(value);
  }
  if (value instanceof Error) {
    return JSON.stringify({ name: value.name, message: value.message });
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (value === null || typeof value !== "object") return false;
  if (Array.isArray(value)) return false;
  const proto = Object.getPrototypeOf(value);
  return proto === Object.prototype || proto === null;
}

function flattenArgs(args: unknown[]): string {
  const parts: string[] = [];
  for (const arg of args) {
    if (typeof arg === "string") {
      parts.push(arg);
      continue;
    }
    if (isPlainObject(arg)) {
      for (const [key, val] of Object.entries(arg)) {
        parts.push(`${key}=${formatValue(val)}`);
      }
      continue;
    }
    parts.push(formatValue(arg));
  }
  return parts.join(" ");
}

export function createDebugLogger(namespace: string): DebugLogger {
  if (!DEV) return NOOP;
  const prefix = `[${namespace}]`;
  return (...args: unknown[]) => {
    console.debug(`${prefix} ${flattenArgs(args)}`);
  };
}
