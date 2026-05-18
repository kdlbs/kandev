import { getBackendConfig } from "@/lib/config";

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
  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

function resolveUrl(pathOrUrl: string, baseUrl: string) {
  if (pathOrUrl.startsWith("http://") || pathOrUrl.startsWith("https://")) {
    return pathOrUrl;
  }
  return `${baseUrl}${pathOrUrl}`;
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
  throw new ApiError(message, response.status, body);
}

export async function fetchJson<T>(pathOrUrl: string, options?: ApiRequestOptions): Promise<T> {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = resolveUrl(pathOrUrl, baseUrl);
  const response = await fetch(url, {
    ...options?.init,
    cache: options?.cache,
    headers: {
      "Content-Type": "application/json",
      ...(options?.init?.headers ?? {}),
    },
  });
  if (!response.ok) await throwFromResponse(response);
  if (response.status === 204) return undefined as T;
  const text = await response.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
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
