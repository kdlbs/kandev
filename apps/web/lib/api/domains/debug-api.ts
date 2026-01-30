import { fetchJson } from '@/lib/api/client';
import type { Message } from '@/lib/types/http';

export type DiscoveredFile = {
  path: string;
  protocol: string;
  agent?: string;
  message_count: number;
  file_type?: string;
};

export type NormalizedFixture = {
  protocol: string;
  tool_name: string;
  tool_type: string;
  input: Record<string, unknown>;
  payload: Record<string, unknown>;
};

/**
 * Fetches list of discovered fixture files.
 */
export async function fetchFixtureFiles(): Promise<DiscoveredFile[]> {
  const response = await fetchJson<{ files: DiscoveredFile[] }>('/api/v1/debug/fixture-files');
  return response.files ?? [];
}

/**
 * Fetches normalized message fixtures for a specific file.
 * @param filePath - Relative path to the fixture file
 */
export async function fetchNormalizedMessages(filePath: string): Promise<NormalizedFixture[]> {
  return fetchJson<NormalizedFixture[]>(`/api/v1/debug/normalize-messages?file=${encodeURIComponent(filePath)}`);
}

/**
 * Fetches list of discovered normalized event files.
 */
export async function fetchNormalizedFiles(): Promise<DiscoveredFile[]> {
  const response = await fetchJson<{ files: DiscoveredFile[] }>('/api/v1/debug/normalized-files');
  return response.files ?? [];
}

/**
 * Fetches normalized events as Messages from a specific file.
 * @param filePath - Relative path to the normalized file
 */
export async function fetchNormalizedEventsAsMessages(filePath: string): Promise<Message[]> {
  const response = await fetchJson<{ messages: Message[] }>(`/api/v1/debug/normalized-events?file=${encodeURIComponent(filePath)}`);
  return response.messages ?? [];
}
