import { getBackendConfig } from "@/lib/config";
import { readInterimSettingsInterlockToken } from "@/src/boot-payload";

const interimSettingsInterlockHeader = "X-Kandev-Interim-Settings-Interlock";

export type ApiRequestOptions = {
  baseUrl?: string;
  cache?: RequestCache;
  init?: RequestInit;
};

/**
 * Error thrown by fetchJson on non-2xx responses. `body` carries the parsed
 * JSON response body (if any) so callers can react to structured fields like
 * the dirty_files list returned with HTTP 409.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;
  readonly retryAfterSeconds?: number;
  constructor(message: string, status: number, body: unknown, retryAfterSeconds?: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

function resolveUrl(pathOrUrl: string, baseUrl: string) {
  if (pathOrUrl.startsWith("http://") || pathOrUrl.startsWith("https://")) {
    return pathOrUrl;
  }
  return `${baseUrl}${pathOrUrl}`;
}

// Notified when a request carries a Kandev session challenge. The auth gate
// (src/main.tsx) registers a callback that clears the store's auth slice and
// redirects to the login page, so a session that expires mid-use doesn't leave
// the SPA stuck making requests against a stale authenticated identity.
let onUnauthorized: (() => void) | null = null;

export function setOnUnauthorized(cb: (() => void) | null): void {
  onUnauthorized = cb;
}

function isKandevAuthChallenge(response: Response): boolean {
  const challenge = response.headers.get("WWW-Authenticate");
  return challenge?.split(",").some((value) => /^Bearer(?:\s|$)/i.test(value.trim())) ?? false;
}

async function throwFromResponse(response: Response): Promise<never> {
  let body: unknown = null;
  let message = `Request failed: ${response.status} ${response.statusText}`;
  try {
    body = await response.json();
  } catch {
    // body remains null
  }
  if (body && typeof body === "object" && "error" in body) {
    const errVal = (body as { error?: unknown }).error;
    if (typeof errVal === "string") message = errVal;
  }
  const retryAfter = Number(response.headers.get("Retry-After"));
  throw new ApiError(
    message,
    response.status,
    body,
    Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter : undefined,
  );
}

export async function fetchJson<T>(pathOrUrl: string, options?: ApiRequestOptions): Promise<T> {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = resolveUrl(pathOrUrl, baseUrl);
  const response = await fetch(url, {
    ...options?.init,
    cache: options?.cache,
    // Send the session cookie for opt-in authentication.
    credentials: "include",
    headers: buildRequestHeaders(options),
  });
  if (!response.ok) {
    if (response.status === 401 && isKandevAuthChallenge(response)) onUnauthorized?.();
    await throwFromResponse(response);
  }
  if (response.status === 204) return undefined as T;
  const text = await response.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

// buildRequestHeaders assembles the JSON content-type header plus the
// interim-settings interlock token on mutating requests.
function buildRequestHeaders(options?: ApiRequestOptions): Headers {
  const headers = requestHeaders(options?.init?.headers);
  headers.set("Content-Type", "application/json");
  if (isMutation(options?.init?.method)) {
    const token = readInterimSettingsInterlockToken();
    if (token) headers.set(interimSettingsInterlockHeader, token);
  }
  return headers;
}

function isMutation(method: string | undefined): boolean {
  return ["POST", "PUT", "PATCH", "DELETE"].includes(method?.toUpperCase() ?? "GET");
}

function requestHeaders(headers: HeadersInit | undefined): Headers {
  return new Headers(headers);
}

/**
 * Wraps fetchJson with up to `maxAttempts` retries (default 3) and
 * exponential backoff starting at `baseDelayMs` (default 100 ms).
 *
 * Only transient network errors (TypeError: fetch failed, ECONNRESET,
 * ECONNREFUSED) are retried. ApiError (non-2xx HTTP status) propagates
 * immediately — the backend response is authoritative.
 *
 * Use for idempotent SSR and client-side reads that can race against a
 * backend restart or a saturated connection pool.
 */
export async function fetchJsonWithRetry<T>(
  pathOrUrl: string,
  options?: ApiRequestOptions,
  maxAttempts = 3,
  baseDelayMs = 100,
): Promise<T> {
  let lastError: unknown;
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    try {
      return await fetchJson<T>(pathOrUrl, options);
    } catch (err) {
      if (err instanceof ApiError) throw err; // do not retry HTTP errors
      lastError = err;
      if (attempt < maxAttempts - 1) {
        await new Promise((r) => setTimeout(r, baseDelayMs * 2 ** attempt));
      }
    }
  }
  throw lastError;
}
