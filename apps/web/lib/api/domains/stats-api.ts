import { fetchJson, type ApiRequestOptions } from '../client';
import type { StatsResponse } from '@/lib/types/http';

export type StatsRange = 'week' | 'month' | 'all';

export async function fetchStats(
  workspaceId: string,
  options?: ApiRequestOptions,
  range?: StatsRange
) {
  const query = range ? `?range=${encodeURIComponent(range)}` : '';
  return fetchJson<StatsResponse>(`/api/v1/workspaces/${workspaceId}/stats${query}`, options);
}
