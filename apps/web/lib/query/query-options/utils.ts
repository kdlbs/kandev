import type { ApiRequestOptions } from "@/lib/api/client";

export function withSignal(signal: AbortSignal): ApiRequestOptions {
  return { init: { signal } };
}
