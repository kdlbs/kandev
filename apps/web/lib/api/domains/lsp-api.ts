import { fetchJson, type ApiRequestOptions } from "../client";
import type { TaskLspLanguageSnapshot, TaskLspPolicy, TaskLspSnapshot } from "@/lib/types/http-lsp";

function languagePath(taskId: string, language: string): string {
  return `/api/v1/tasks/${encodeURIComponent(taskId)}/lsp/${encodeURIComponent(language)}`;
}

export function getTaskLsp(taskId: string, options?: ApiRequestOptions): Promise<TaskLspSnapshot> {
  return fetchJson<TaskLspSnapshot>(`/api/v1/tasks/${encodeURIComponent(taskId)}/lsp`, options);
}

export function setTaskLspPolicy(
  taskId: string,
  language: string,
  policy: TaskLspPolicy,
  options?: ApiRequestOptions,
): Promise<TaskLspLanguageSnapshot> {
  return fetchJson<TaskLspLanguageSnapshot>(`${languagePath(taskId, language)}/policy`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PUT",
      body: JSON.stringify({ policy }),
    },
  });
}

function controlTaskLsp(
  taskId: string,
  language: string,
  action: "start" | "stop" | "restart",
  options?: ApiRequestOptions,
): Promise<TaskLspLanguageSnapshot> {
  return fetchJson<TaskLspLanguageSnapshot>(`${languagePath(taskId, language)}/${action}`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: undefined },
  });
}

export const startTaskLsp = (taskId: string, language: string, options?: ApiRequestOptions) =>
  controlTaskLsp(taskId, language, "start", options);
export const stopTaskLsp = (taskId: string, language: string, options?: ApiRequestOptions) =>
  controlTaskLsp(taskId, language, "stop", options);
export const restartTaskLsp = (taskId: string, language: string, options?: ApiRequestOptions) =>
  controlTaskLsp(taskId, language, "restart", options);
