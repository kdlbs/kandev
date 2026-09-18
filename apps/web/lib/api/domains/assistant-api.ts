import { ApiError, fetchJson } from "../client";
import type {
  AssistantAnswer,
  AssistantBinding,
  AssistantControl,
  AssistantControlReceipt,
  AssistantCredential,
  AssistantInputSnapshot,
  AssistantMemory,
  AssistantMemoryEdit,
  AssistantMode,
  AssistantPage,
  AssistantPages,
} from "./assistant-types";
export type * from "./assistant-types";
const base = "/api/v1/orchestration/assistant";
const json = (method: string, body: unknown) => ({ init: { method, body: JSON.stringify(body) } });
export async function getAssistant(signal?: AbortSignal): Promise<AssistantBinding | null> {
  try {
    return await fetchJson<AssistantBinding>(base, { init: { signal } });
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null;
    throw error;
  }
}
export const selectAssistant = (
  orchestrator_id: string,
  expected_version: number,
  execution_mode: AssistantMode,
) =>
  fetchJson<AssistantBinding>(
    base,
    json("PUT", { orchestrator_id, expected_version, execution_mode }),
  );
export async function getAssistantPage<K extends keyof AssistantPages>(
  kind: K,
  after = "",
  signal?: AbortSignal,
): Promise<AssistantPage<AssistantPages[K]>> {
  const query = new URLSearchParams({ after, limit: "50" });
  const page = await fetchJson<{
    entries?: AssistantPages[K][];
    objectives?: AssistantPages[K][];
    memory?: AssistantPages[K][];
    next_cursor?: string;
  }>(`${base}/${kind}?${query}`, { init: { signal } });
  return {
    entries: page.entries ?? page.objectives ?? page.memory ?? [],
    next_cursor: page.next_cursor ?? "",
  };
}
export const getAssistantCredentials = (signal?: AbortSignal) =>
  fetchJson<{ credentials: AssistantCredential[] }>(`${base}/credentials`, { init: { signal } });
export const getAssistantInput = (id: string, signal?: AbortSignal) =>
  fetchJson<AssistantInputSnapshot>(`${base}/attention/${encodeURIComponent(id)}/input`, {
    init: { signal },
  });
export const resolveAssistantInput = (id: string, body: AssistantAnswer) =>
  fetchJson(`${base}/attention/${encodeURIComponent(id)}/resolve`, json("POST", body));
export const controlAssistant = (body: AssistantControl) =>
  fetchJson<AssistantControlReceipt>(`${base}/control`, json("POST", body));
export const editAssistantMemory = (id: string, body: AssistantMemoryEdit) =>
  fetchJson<AssistantMemory>(`${base}/memory/${encodeURIComponent(id)}`, json("PUT", body));
export const forgetAssistantMemory = (id: string, expected_revision: number) =>
  fetchJson(`${base}/memory/${encodeURIComponent(id)}`, json("DELETE", { expected_revision }));

export const getAssistantMemorySource = (id: string, signal?: AbortSignal) =>
  fetchJson<{ id: string; body: string; created_at: string; truncated: boolean }>(
    `${base}/memory/${encodeURIComponent(id)}/source`,
    { init: { signal } },
  );
