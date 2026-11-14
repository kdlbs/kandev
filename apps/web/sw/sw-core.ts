/**
 * Pure decision logic for the service worker. Nothing here touches the
 * ServiceWorkerGlobalScope, so it is unit-testable under vitest and the
 * worker entry stays a thin event adapter.
 */

import { linkToTask } from "@/lib/links";

export const SW_MESSAGE_NAVIGATE = "kandev:navigate";

/** Path prefixes the worker must never answer from a cache or intercept. */
const NEVER_INTERCEPT_PREFIXES = ["/api/", "/ws", "/debug/", "/mcp"];
const HASHED_ASSET_PREFIX = "/assets/";

export type PushPayload = {
  title: string;
  body: string;
  tag?: string;
  url: string;
  taskId?: string;
  sessionId?: string;
};

export type WindowClientLike = {
  url: string;
  focused: boolean;
  visibilityState: string;
};

type RequestLike = {
  method: string;
  mode: string;
  url: string;
};

export function isSameOrigin(url: string, origin: string): boolean {
  try {
    return new URL(url, origin).origin === origin;
  } catch {
    return false;
  }
}

export function isNeverIntercepted(pathname: string): boolean {
  return NEVER_INTERCEPT_PREFIXES.some(
    (prefix) => pathname === prefix.replace(/\/$/, "") || pathname.startsWith(prefix),
  );
}

export type RequestRoute = "navigate" | "asset" | "precache" | "passthrough";

/**
 * Decides how the worker treats a request. Anything not explicitly listed is
 * passed through untouched, which is what keeps API and WebSocket traffic out
 * of every cache.
 */
export function routeRequest(
  request: RequestLike,
  origin: string,
  precachePaths: ReadonlySet<string>,
): RequestRoute {
  if (request.method !== "GET" || !isSameOrigin(request.url, origin)) return "passthrough";
  const { pathname } = new URL(request.url, origin);
  if (isNeverIntercepted(pathname)) return "passthrough";
  if (request.mode === "navigate") return "navigate";
  if (pathname.startsWith(HASHED_ASSET_PREFIX)) return "asset";
  if (precachePaths.has(pathname)) return "precache";
  return "passthrough";
}

type ResponseLike = {
  ok: boolean;
  status: number;
  type: string;
  redirected: boolean;
  headers: { get(name: string): string | null };
};

/**
 * A response may be stored only when it is a plain successful same-origin
 * answer for a static file. Redirects, opaque responses and HTML are refused:
 * behind an access proxy an expired session turns every request into a 200 or
 * 302 login page, and caching that as a script or stylesheet would brick the
 * app until the cache is cleared.
 */
export function isCacheableAsset(response: ResponseLike): boolean {
  if (!response.ok || response.status !== 200) return false;
  if (response.type !== "basic" || response.redirected) return false;
  const contentType = (response.headers.get("content-type") ?? "").toLowerCase();
  return contentType !== "" && !contentType.includes("text/html");
}

/** Only the offline shell may contain HTML; other precache entries are assets. */
export function isCacheablePrecache(response: ResponseLike, path: string): boolean {
  if (!response.ok || response.status !== 200 || response.type !== "basic" || response.redirected) {
    return false;
  }
  const contentType = (response.headers.get("content-type") ?? "").toLowerCase();
  if (path === "/offline.html") return contentType.includes("text/html");
  return contentType !== "" && !contentType.includes("text/html");
}

export function cacheNames(buildId: string) {
  return { shell: `kandev-shell-${buildId}`, assets: `kandev-assets-${buildId}` };
}

/** Names of caches this build does not own; deleted on activate. */
export function staleCacheNames(existing: readonly string[], buildId: string): string[] {
  const current = new Set(Object.values(cacheNames(buildId)));
  return existing.filter((name) => name.startsWith("kandev-") && !current.has(name));
}

/**
 * Parses a push message. A malformed or empty message still yields a generic
 * notification: Chrome requires every push to show one, and dropping it would
 * cost the site its push permission.
 */
export function parsePushPayload(raw: string | null, fallbackTitle: string): PushPayload {
  const fallback: PushPayload = { title: fallbackTitle, body: "", url: "/" };
  if (!raw) return fallback;
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return fallback;
  }
  if (typeof data !== "object" || data === null) return fallback;
  const record = data as Record<string, unknown>;
  const str = (key: string) => (typeof record[key] === "string" ? (record[key] as string) : "");
  return {
    title: str("title") || fallbackTitle,
    body: str("body"),
    tag: str("tag") || undefined,
    url: sanitizeTargetUrl(str("url")),
    taskId: str("taskId") || undefined,
    sessionId: str("sessionId") || undefined,
  };
}

/** Only origin-relative paths are honoured; anything else falls back to home. */
export function sanitizeTargetUrl(url: string): string {
  if (
    !url.startsWith("/") ||
    url.startsWith("//") ||
    url.includes("\\") ||
    /[\u0000-\u001f\u007f]/.test(url)
  ) {
    return "/";
  }
  return url;
}

/** True when a visible focused window is already showing the notified task. */
export function shouldSuppressNotification(
  clients: readonly WindowClientLike[],
  taskId?: string,
): boolean {
  if (!taskId) return false;
  const expectedPath = linkToTask(taskId);
  return clients.some((client) => {
    if (!client.focused || client.visibilityState !== "visible") return false;
    try {
      return new URL(client.url).pathname.replace(/\/$/, "") === expectedPath;
    } catch {
      return false;
    }
  });
}

export function notificationOptions(payload: PushPayload, iconUrl: string) {
  return {
    body: payload.body,
    tag: payload.tag,
    icon: iconUrl,
    // Re-alert when a notification with the same tag is replaced; otherwise a
    // second "needs input" for the same session would update silently.
    renotify: Boolean(payload.tag),
    data: { url: payload.url, taskId: payload.taskId, sessionId: payload.sessionId },
  };
}

/** Prefers a client already on the target path, then any open client. */
export function pickClientForUrl<T extends WindowClientLike>(
  clients: readonly T[],
  targetUrl: string,
  origin: string,
): T | undefined {
  const target = new URL(targetUrl, origin);
  const onTarget = clients.find((client) => {
    const url = new URL(client.url);
    return url.pathname === target.pathname && url.search === target.search;
  });
  return onTarget ?? clients.find((client) => client.focused) ?? clients[0];
}
