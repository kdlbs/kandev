import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  TurnChangeContent,
  TurnChangeContentVariant,
  TurnChangeFilesPage,
  TurnChangeHistoryPage,
  TurnChangeSetSummary,
} from "@/lib/types/turn-changes";

function pageQuery(offset: number, limit: number): string {
  const query = new URLSearchParams({ offset: String(offset), limit: String(limit) });
  return `?${query.toString()}`;
}

export function listTurnChangeHistory(
  sessionId: string,
  offset = 0,
  limit = 50,
  options?: ApiRequestOptions,
) {
  return fetchJson<TurnChangeHistoryPage>(
    `/api/v1/task-sessions/${sessionId}/turn-changes${pageQuery(offset, limit)}`,
    options,
  );
}

export function getTurnChangeHistoryItem(
  sessionId: string,
  changeSetId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<TurnChangeSetSummary>(
    `/api/v1/task-sessions/${sessionId}/turn-changes/${changeSetId}`,
    options,
  );
}

export function listTurnChangeFiles(
  sessionId: string,
  changeSetId: string,
  repositoryChangeId: string,
  pagination: { offset?: number; limit?: number } = {},
  options?: ApiRequestOptions,
) {
  const { offset = 0, limit = 100 } = pagination;
  return fetchJson<TurnChangeFilesPage>(
    `/api/v1/task-sessions/${sessionId}/turn-changes/${changeSetId}/repositories/${repositoryChangeId}/files${pageQuery(offset, limit)}`,
    options,
  );
}

export function readTurnChangeContent(
  sessionId: string,
  changeSetId: string,
  fileChangeId: string,
  variant: TurnChangeContentVariant,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ variant });
  return fetchJson<TurnChangeContent>(
    `/api/v1/task-sessions/${sessionId}/turn-changes/${changeSetId}/files/${fileChangeId}/content?${query.toString()}`,
    options,
  );
}

export function decodeTurnChangeContent(content: string): string {
  return new TextDecoder().decode(Uint8Array.from(atob(content), (char) => char.charCodeAt(0)));
}
