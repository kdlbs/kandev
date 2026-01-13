'use server';

import { getBackendConfig } from '@/lib/config';
import type { Executor, ListExecutorsResponse } from '@/lib/types/http';

const { apiBaseUrl } = getBackendConfig();

async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...options,
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      ...(options?.headers ?? {}),
    },
  });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status} ${response.statusText}`);
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

export async function listExecutorsAction(): Promise<ListExecutorsResponse> {
  return fetchJson<ListExecutorsResponse>(`${apiBaseUrl}/api/v1/executors`);
}

export async function getExecutorAction(id: string): Promise<Executor> {
  return fetchJson<Executor>(`${apiBaseUrl}/api/v1/executors/${id}`);
}

export async function createExecutorAction(payload: {
  name: string;
  type: string;
  status?: string;
  config?: Record<string, string>;
}): Promise<Executor> {
  return fetchJson<Executor>(`${apiBaseUrl}/api/v1/executors`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateExecutorAction(
  id: string,
  payload: Partial<Pick<Executor, 'name' | 'type' | 'status' | 'config'>>
): Promise<Executor> {
  return fetchJson<Executor>(`${apiBaseUrl}/api/v1/executors/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteExecutorAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/executors/${id}`, { method: 'DELETE' });
}
