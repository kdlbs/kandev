import { getBackendConfig } from '@/lib/config';

export type ApiRequestOptions = {
  baseUrl?: string;
  cache?: RequestCache;
  init?: RequestInit;
};

function resolveUrl(pathOrUrl: string, baseUrl: string) {
  if (pathOrUrl.startsWith('http://') || pathOrUrl.startsWith('https://')) {
    return pathOrUrl;
  }
  return `${baseUrl}${pathOrUrl}`;
}

export async function fetchJson<T>(pathOrUrl: string, options?: ApiRequestOptions): Promise<T> {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = resolveUrl(pathOrUrl, baseUrl);
  const response = await fetch(url, {
    ...options?.init,
    cache: options?.cache,
    headers: {
      'Content-Type': 'application/json',
      ...(options?.init?.headers ?? {}),
    },
  });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status} ${response.statusText} (${url})`);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  const text = await response.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}
