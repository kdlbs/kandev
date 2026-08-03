import { fetchJson } from "@/lib/api/client";

const REPORT_PATH = "/api/v1/system/logs/frontend-errors";
const REQUEST_TIMEOUT_MS = 2_000;
const TEXT_LIMIT = 8 * 1024;
const TASK_ID_LIMIT = 128;
const BROWSER_FIELD_LIMIT = 2 * 1024;
const STACK_LIMIT = 16 * 1024;
const MAX_REACT_TEXT_NODES = 32;

export type FrontendErrorSource = "sonner" | "toast-provider";

export type FrontendErrorReportInput = {
  source: FrontendErrorSource;
  title?: unknown;
  description?: unknown;
  error?: unknown;
};

export type FrontendErrorReport = {
  client_timestamp: string;
  source: FrontendErrorSource;
  task_id?: string;
  title?: string;
  description?: string;
  url?: string;
  user_agent?: string;
  language?: string;
  platform?: string;
  viewport?: { width: number; height: number };
  stack?: string;
  error?: { name: string; message: string; stack?: string };
};

export function scheduleFrontendErrorReport(input: FrontendErrorReportInput): void {
  queueMicrotask(() => {
    try {
      const report = buildFrontendErrorReport(input);
      void sendFrontendErrorReport(report).catch(() => {});
    } catch {
      // Diagnostics must never alter or recursively log the original toast.
    }
  });
}

export function buildFrontendErrorReport(input: FrontendErrorReportInput): FrontendErrorReport {
  const currentURL = readCurrentURL();
  const report: FrontendErrorReport = {
    client_timestamp: new Date().toISOString(),
    source: input.source,
    title: visibleText(input.title),
    description: visibleText(input.description),
    stack: bounded(new Error("error toast emitted").stack, STACK_LIMIT),
  };
  if (currentURL) {
    report.task_id = deriveTaskID(currentURL);
    report.url = bounded(`${currentURL.origin}${currentURL.pathname}`, TEXT_LIMIT);
  }
  addBrowserContext(report);
  const error = errorDetails(input.error);
  if (error) report.error = error;
  return report;
}

export async function sendFrontendErrorReport(report: FrontendErrorReport): Promise<void> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
  try {
    await fetchJson<void>(REPORT_PATH, {
      init: {
        method: "POST",
        body: JSON.stringify(report),
        signal: controller.signal,
      },
    });
  } finally {
    clearTimeout(timeout);
  }
}

export function deriveTaskID(url: URL): string | undefined {
  const segments = url.pathname.split("/").filter(Boolean);
  let candidate: string | null = null;
  if ((segments[0] === "t" || segments[0] === "tasks") && segments.length >= 2) {
    candidate = segments[1];
  } else if (segments[0] === "office" && segments[1] === "tasks" && segments.length >= 3) {
    candidate = segments[2];
  } else {
    candidate = url.searchParams.get("taskId");
  }
  if (!candidate) return undefined;
  try {
    return bounded(decodeURIComponent(candidate), TASK_ID_LIMIT);
  } catch {
    return bounded(candidate, TASK_ID_LIMIT);
  }
}

function addBrowserContext(report: FrontendErrorReport): void {
  if (typeof navigator !== "undefined") {
    report.user_agent = bounded(navigator.userAgent, BROWSER_FIELD_LIMIT);
    report.language = bounded(navigator.language, BROWSER_FIELD_LIMIT);
    report.platform = bounded(navigator.platform, BROWSER_FIELD_LIMIT);
  }
  if (typeof window !== "undefined") {
    report.viewport = { width: window.innerWidth, height: window.innerHeight };
  }
}

function readCurrentURL(): URL | undefined {
  if (typeof window === "undefined") return undefined;
  try {
    return new URL(window.location.href);
  } catch {
    return undefined;
  }
}

function visibleText(value: unknown): string | undefined {
  const state = { nodes: 0 };
  return bounded(extractVisibleText(value, state, 0), TEXT_LIMIT);
}

function extractVisibleText(
  value: unknown,
  state: { nodes: number },
  depth: number,
): string | undefined {
  if (state.nodes++ >= MAX_REACT_TEXT_NODES || depth > 4 || value == null) return undefined;
  switch (typeof value) {
    case "string":
      return value;
    case "number":
    case "boolean":
    case "bigint":
      return String(value);
    case "function":
      return "[function]";
    case "symbol":
      return "[symbol]";
    case "object":
      return extractObjectText(value, state, depth);
    default:
      return undefined;
  }
}

function extractObjectText(
  value: object,
  state: { nodes: number },
  depth: number,
): string | undefined {
  try {
    if (Array.isArray(value)) {
      const parts: string[] = [];
      const count = Math.min(value.length, MAX_REACT_TEXT_NODES);
      for (let index = 0; index < count; index++) {
        const item = Object.getOwnPropertyDescriptor(value, String(index))?.value;
        const text = extractVisibleText(item, state, depth + 1);
        if (text) parts.push(text);
      }
      return parts.join(" ") || "[object]";
    }
    const props = Object.getOwnPropertyDescriptor(value, "props")?.value;
    if (props && typeof props === "object") {
      const children = Object.getOwnPropertyDescriptor(props, "children")?.value;
      const text = extractVisibleText(children, state, depth + 1);
      if (text) return text;
    }
  } catch {
    return "[object]";
  }
  return value instanceof Error ? bounded(value.message, TEXT_LIMIT) : "[object]";
}

function errorDetails(value: unknown): FrontendErrorReport["error"] | undefined {
  if (!(value instanceof Error)) return undefined;
  return {
    name: bounded(value.name, BROWSER_FIELD_LIMIT) ?? "Error",
    message: bounded(value.message, TEXT_LIMIT) ?? "",
    stack: bounded(value.stack, STACK_LIMIT),
  };
}

function bounded(value: string | undefined, limit: number): string | undefined {
  if (!value) return undefined;
  if (value.length <= limit) return value;
  return value.slice(0, limit);
}
