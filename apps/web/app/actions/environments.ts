'use server';

import { getBackendConfig } from '@/lib/config';
import type { Environment, ListEnvironmentsResponse } from '@/lib/types/http';

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

export async function listEnvironmentsAction(): Promise<ListEnvironmentsResponse> {
  return fetchJson<ListEnvironmentsResponse>(`${apiBaseUrl}/api/v1/environments`);
}

export async function getEnvironmentAction(id: string): Promise<Environment> {
  return fetchJson<Environment>(`${apiBaseUrl}/api/v1/environments/${id}`);
}

export async function createEnvironmentAction(payload: {
  name: string;
  kind: string;
  worktree_root?: string;
  image_tag?: string;
  dockerfile?: string;
  build_config?: Record<string, string>;
}): Promise<Environment> {
  return fetchJson<Environment>(`${apiBaseUrl}/api/v1/environments`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateEnvironmentAction(
  id: string,
  payload: Partial<Pick<Environment, 'name' | 'kind' | 'worktree_root' | 'image_tag' | 'dockerfile' | 'build_config'>>
): Promise<Environment> {
  return fetchJson<Environment>(`${apiBaseUrl}/api/v1/environments/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteEnvironmentAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/environments/${id}`, { method: 'DELETE' });
}
