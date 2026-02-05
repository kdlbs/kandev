import { fetchJson, type ApiRequestOptions } from '../client';
import type { StatsResponse } from '@/lib/types/http';

export async function fetchStats(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<StatsResponse>(`/api/v1/workspaces/${workspaceId}/stats`, options);
}

